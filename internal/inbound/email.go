package inbound

// Email ingestion core — provider-agnostic. An email-parsing provider (Postmark
// to start, see postmark.go) turns inbound mail into a NormalizedInbound and
// hands it to IngestEmail, which either threads a reply onto an existing ticket
// (as a public comment) or creates a new ticket. All logic lives here; adapters
// only translate their wire format into NormalizedInbound. See
// docs/email-ingestion.md.

import (
	"fmt"
	"log/slog"
	"regexp"
	"strconv"
	"strings"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
)

const (
	maxTitleLen = 300   // tickets.title schema max
	maxBodyLen  = 10000 // tickets.body / ticket_comments.body schema max
)

// Outcome values returned in Result; the adapter maps them to HTTP.
const (
	OutcomeCreated   = "created"
	OutcomeCommented = "commented"
	OutcomeDuplicate = "duplicate"
	OutcomeIgnored   = "ignored"
)

// Addr is a parsed email address.
type Addr struct {
	Email string
	Name  string
}

// NormalizedInbound is the one shape every provider adapter produces. It is
// deliberately post-parse and TEXT-ONLY: we take the plain-text body and never
// touch MIME or attachments. Email attachments are a non-goal — inbound mail is
// dominated by inline signature images, which would bury real files under the
// per-record attachment cap; files belong on the portal. See docs/email-ingestion.md.
type NormalizedInbound struct {
	MessageID  string
	From       Addr
	Subject    string
	Body       string            // best plain text: provider's stripped reply, else full body
	ReplyToken string            // optional threading override; empty ⇒ core derives it from the [#N] subject tag
	Headers    map[string]string // lower-cased keys
	DKIMPass   bool              // provider verdict — LOGGED, not enforced (v1)
	SpamFlag   bool              // provider spam verdict
}

// Result is what IngestEmail decided; the adapter turns it into a response.
type Result struct {
	Outcome string       // one of the Outcome* constants
	Reason  string       // set when Outcome == OutcomeIgnored
	Ticket  *core.Record // set when a ticket was created/commented/deduped
}

// IngestEmail projects one normalized inbound email into a ticket or comment.
// It never returns an error for a message it deliberately drops (spam, loops,
// unresolved tenant) — those come back as OutcomeIgnored so the provider stops
// retrying. A returned error is a genuine server fault (let the provider retry).
func IngestEmail(app core.App, msg NormalizedInbound) (Result, error) {
	if reason, drop := shouldDrop(msg); drop {
		slog.Info("inbound email dropped", "reason", reason,
			"from", msg.From.Email, "message_id", msg.MessageID)
		return Result{Outcome: OutcomeIgnored, Reason: reason}, nil
	}
	if !msg.DKIMPass {
		// v1 is log-only: reject-unmatched is the primary abuse control, and a
		// spoofed KNOWN sender is the named residual risk in the design doc.
		slog.Warn("inbound email failed DKIM (processing anyway)",
			"from", msg.From.Email, "message_id", msg.MessageID)
	}

	// Threading: a reply naming an existing, non-closed ticket becomes a public
	// comment on it. A closed ticket is final (mirrors migration 1822000000), so
	// it falls through to a fresh ticket with a breadcrumb. The token comes from
	// the [#N] subject tag (derived here so adapters stay dumb); a provider with
	// a stronger signal may pre-fill ReplyToken to override.
	token := msg.ReplyToken
	if token == "" {
		token = ParseTicketToken(msg.Subject)
	}
	var bodyPrefix string
	if n, ok := parseTicketNumber(token); ok {
		if ticket := findTicketByNumber(app, n); ticket != nil {
			if ticket.GetString("status") == "closed" {
				bodyPrefix = fmt.Sprintf("Reply to closed ticket #%d:\n\n", n)
			} else {
				if msg.MessageID != "" && commentExistsForMessage(app, msg.MessageID) {
					return Result{Outcome: OutcomeDuplicate, Ticket: ticket}, nil
				}
				if _, err := createEmailComment(app, ticket, msg); err != nil {
					return Result{}, fmt.Errorf("create email comment: %w", err)
				}
				return Result{Outcome: OutcomeCommented, Ticket: ticket}, nil
			}
		}
	}

	// New ticket. Resolve the tenant or reject — there is no catch-all customer.
	customer, err := resolveCustomer(app, msg.From)
	if err != nil {
		return Result{}, fmt.Errorf("resolve customer: %w", err)
	}
	if customer == nil {
		slog.Info("inbound email rejected: no customer for sender",
			"from", msg.From.Email, "message_id", msg.MessageID)
		return Result{Outcome: OutcomeIgnored, Reason: "unresolved customer"}, nil
	}

	// Reuse the one webhook projection; it also matches the requester by email
	// within the customer and dedupes on the Message-ID (dedupe_key).
	ticket, created, err := CreateTicket(app, customer, Payload{
		Title:          subjectOrFallback(msg.Subject),
		Body:           truncate(bodyPrefix+msg.Body, maxBodyLen),
		RequesterEmail: msg.From.Email,
		DedupeKey:      msg.MessageID,
		Source:         "email",
	})
	if err != nil {
		return Result{}, fmt.Errorf("create ticket: %w", err)
	}
	outcome := OutcomeCreated
	if !created {
		outcome = OutcomeDuplicate
	}
	return Result{Outcome: outcome, Ticket: ticket}, nil
}

// shouldDrop screens loops and junk before any DB work. The helpdesk emails from
// a neighboring address, so bounces and out-of-office replies WILL arrive; this
// is what stops a notification→reply loop.
func shouldDrop(msg NormalizedInbound) (string, bool) {
	if msg.SpamFlag {
		return "spam", true
	}
	from := strings.ToLower(strings.TrimSpace(msg.From.Email))
	if from == "" {
		return "empty from", true
	}
	local := from
	if at := strings.Index(from, "@"); at >= 0 {
		local = from[:at]
	}
	if local == "mailer-daemon" || local == "postmaster" {
		return "system sender", true
	}
	if v := header(msg, "auto-submitted"); v != "" && v != "no" {
		return "auto-submitted", true
	}
	switch header(msg, "precedence") {
	case "bulk", "list", "junk":
		return "bulk precedence", true
	}
	return "", false
}

// resolveCustomer maps a sender to a customer: an exact registered user's
// customer, else the sender domain (never a public provider), else nil (reject).
func resolveCustomer(app core.App, from Addr) (*core.Record, error) {
	email := strings.TrimSpace(from.Email)
	if email == "" {
		return nil, nil
	}
	// Rung 1: an exact registered user. CreateTicket re-matches the requester
	// within the customer, so we only need the customer here.
	if user, err := app.FindFirstRecordByFilter("users",
		"email = {:e}", dbx.Params{"e": email}); err == nil && user != nil {
		return app.FindRecordById("customers", user.GetString("customer"))
	}
	// Rung 2: the sender's own domain maps to a customer. Public providers are
	// excluded (a shared domain can't identify a tenant).
	domain := domainOf(email)
	if domain == "" || IsPublicEmailDomain(domain) {
		return nil, nil
	}
	cust, err := app.FindFirstRecordByFilter("customers",
		"email_domain = {:d} && active = true", dbx.Params{"d": domain})
	if err != nil || cust == nil {
		return nil, nil
	}
	return cust, nil
}

// createEmailComment records a reply as a PUBLIC comment. Attribution is only to
// a registered user OF THIS CUSTOMER — an unmatched sender stays unattributed,
// so the tickets hook won't auto-reopen a resolved ticket on their say-so. That
// hook (internal/tickets) does the rest: a public, user-authored comment reopens
// a resolved ticket and clears awaiting_requester.
func createEmailComment(app core.App, ticket *core.Record, msg NormalizedInbound) (*core.Record, error) {
	col, err := app.FindCollectionByNameOrId("ticket_comments")
	if err != nil {
		return nil, err
	}
	rec := core.NewRecord(col)
	rec.Set("ticket", ticket.Id)
	if user, err := app.FindFirstRecordByFilter("users",
		"email = {:e} && customer = {:c}",
		dbx.Params{"e": msg.From.Email, "c": ticket.GetString("customer")},
	); err == nil && user != nil {
		rec.Set("author_user", user.Id)
	}
	rec.Set("body", provenanceBody(msg))
	rec.Set("internal", false)
	rec.Set("source_message_id", msg.MessageID)
	if err := app.Save(rec); err != nil {
		return nil, err
	}
	return rec, nil
}

// provenanceBody prefixes the sender so staff see who wrote a comment even when
// it isn't attributed to a portal account.
func provenanceBody(msg NormalizedInbound) string {
	from := msg.From.Email
	if msg.From.Name != "" {
		from = fmt.Sprintf("%s <%s>", msg.From.Name, msg.From.Email)
	}
	return truncate(fmt.Sprintf("From: %s\n\n%s", from, msg.Body), maxBodyLen)
}

func findTicketByNumber(app core.App, n int) *core.Record {
	t, err := app.FindFirstRecordByFilter("tickets", "number = {:n}", dbx.Params{"n": n})
	if err != nil {
		return nil
	}
	return t
}

func commentExistsForMessage(app core.App, msgID string) bool {
	c, err := app.FindFirstRecordByFilter("ticket_comments",
		"source_message_id = {:m}", dbx.Params{"m": msgID})
	return err == nil && c != nil
}

var ticketTokenRe = regexp.MustCompile(`\[#(\d+)\]`)

// ParseTicketToken extracts a ticket number from a subject like
// "Re: [#42] printer on fire". Returns "" when absent. Exported so adapters can
// fill NormalizedInbound.ReplyToken without duplicating the pattern.
func ParseTicketToken(subject string) string {
	if m := ticketTokenRe.FindStringSubmatch(subject); m != nil {
		return m[1]
	}
	return ""
}

func parseTicketNumber(token string) (int, bool) {
	if token == "" {
		return 0, false
	}
	n, err := strconv.Atoi(token)
	if err != nil || n <= 0 {
		return 0, false
	}
	return n, true
}

// publicEmailDomains are shared/free providers that must never be mapped to one
// customer — doing so would route every sender on that provider into a single
// tenant. Used by the resolution ladder here and enforced at write time in
// internal/customers via IsPublicEmailDomain.
var publicEmailDomains = map[string]bool{
	"gmail.com": true, "googlemail.com": true,
	"outlook.com": true, "hotmail.com": true, "live.com": true, "msn.com": true,
	"yahoo.com": true, "yahoo.co.uk": true, "ymail.com": true,
	"icloud.com": true, "me.com": true, "mac.com": true,
	"aol.com": true, "proton.me": true, "protonmail.com": true,
	"gmx.com": true, "zoho.com": true, "mail.com": true, "fastmail.com": true,
}

// IsPublicEmailDomain reports whether a domain is a shared/free email provider.
func IsPublicEmailDomain(domain string) bool {
	return publicEmailDomains[strings.ToLower(strings.TrimSpace(domain))]
}

func domainOf(email string) string {
	at := strings.LastIndex(email, "@")
	if at < 0 || at == len(email)-1 {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(email[at+1:]))
}

func header(msg NormalizedInbound, key string) string {
	return strings.ToLower(strings.TrimSpace(msg.Headers[key]))
}

func subjectOrFallback(subject string) string {
	s := strings.TrimSpace(subject)
	if s == "" {
		return "(no subject)"
	}
	return truncate(s, maxTitleLen)
}

// truncate caps a string to max runes (PocketBase text-field limits count
// runes, and a rune-boundary cut keeps the value valid UTF-8).
func truncate(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max])
}
