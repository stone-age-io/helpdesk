// Package inbound is the authenticated HTTP ticket intake:
//
//	POST /api/helpdesk/inbound/{token}
//
// The token is a per-customer shared secret (customers.webhook_token,
// hidden from the record API); possessing it both authenticates the caller
// and selects the customer. This route is also the future email-provider
// (Postmark/Mailgun) integration point — a provider webhook adapter would
// normalize into the same payload.
//
// The package also owns the staff-side token lifecycle route the SPA's
// customer detail view calls:
//
//	POST /api/helpdesk/customers/{id}/webhook-token           reveal (mint on first use)
//	POST /api/helpdesk/customers/{id}/webhook-token?rotate=1  regenerate
package inbound

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"net/http"
	"strings"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"

	"github.com/stone-age-io/helpdesk/internal/authz"
)

// tokenEntropy sizes the webhook token: 18 bytes → 24-char URL-safe base64.
const tokenEntropy = 18

// Payload is the inbound JSON shape. Only title is required.
type Payload struct {
	Title    string `json:"title"`
	Body     string `json:"body,omitempty"`
	Priority string `json:"priority,omitempty"` // low|normal|high|urgent; anything else → normal
	// RequesterEmail links the ticket to an existing portal account of this
	// customer when it matches; silently ignored otherwise.
	RequesterEmail string `json:"requester_email,omitempty"`
	DedupeKey      string `json:"dedupe_key,omitempty"`
	// Category is an optional ticket_categories key; unknown/inactive keys are
	// ignored (ticket still created, unclassified). Asset/Location are optional
	// free-text provenance fields.
	Category string `json:"category,omitempty"`
	Asset    string `json:"asset,omitempty"`
	// Location is free-text (location_note); LocationCode resolves to a
	// locations row for this customer, falling back to location_note on a miss.
	Location     string `json:"location,omitempty"`
	LocationCode string `json:"location_code,omitempty"`
	// Source is the ticket provenance to stamp. Not wire-bound (json:"-") — it's
	// set by the server-side caller: the HTTP webhook leaves it empty (defaults
	// to "webhook"), the email adapter sets "email". Keeps CreateTicket the one
	// projection for every non-portal intake.
	Source string `json:"-"`
}

// Register binds both routes.
func Register(e *core.ServeEvent) {
	e.Router.POST("/api/helpdesk/inbound/{token}", handleInbound)
	e.Router.POST("/api/helpdesk/customers/{id}/webhook-token", handleTokenReveal)
}

func handleInbound(re *core.RequestEvent) error {
	token := re.Request.PathValue("token")
	if token == "" {
		return re.NotFoundError("unknown token", nil)
	}
	customer, err := re.App.FindFirstRecordByFilter(
		"customers", "webhook_token = {:t} && active = true", dbx.Params{"t": token})
	if err != nil || customer == nil {
		// One shape for wrong token and inactive customer: no oracle.
		return re.NotFoundError("unknown token", nil)
	}

	var payload Payload
	if err := re.BindBody(&payload); err != nil {
		return re.BadRequestError("invalid JSON body", err)
	}

	ticket, created, err := CreateTicket(re.App, customer, payload)
	if err != nil {
		if bad, ok := err.(*badPayloadError); ok {
			return re.BadRequestError(bad.msg, nil)
		}
		return re.InternalServerError("create ticket failed", err)
	}
	status := http.StatusCreated
	if !created {
		status = http.StatusOK // duplicate dedupe_key: report the existing ticket
	}
	return re.JSON(status, map[string]any{
		"id":        ticket.Id,
		"number":    ticket.GetInt("number"),
		"duplicate": !created,
	})
}

type badPayloadError struct{ msg string }

func (e *badPayloadError) Error() string { return e.msg }

// CreateTicket projects one webhook payload into a ticket for the resolved
// customer. Returns created=false when the dedupe key already has a ticket
// (idempotent retries). Exposed for tests, which drive it without HTTP.
func CreateTicket(app core.App, customer *core.Record, p Payload) (*core.Record, bool, error) {
	title := strings.TrimSpace(p.Title)
	if title == "" {
		return nil, false, &badPayloadError{"title is required"}
	}

	if p.DedupeKey != "" {
		existing, err := app.FindFirstRecordByFilter(
			"tickets", "dedupe_key = {:k}", dbx.Params{"k": p.DedupeKey})
		if err == nil && existing != nil {
			return existing, false, nil
		}
	}

	col, err := app.FindCollectionByNameOrId("tickets")
	if err != nil {
		return nil, false, err
	}

	priority := p.Priority
	switch priority {
	case "low", "normal", "high", "urgent":
	default:
		priority = "normal"
	}

	source := p.Source
	if source == "" {
		source = "webhook"
	}

	rec := core.NewRecord(col)
	rec.Set("customer", customer.Id)
	rec.Set("title", title)
	rec.Set("body", p.Body)
	rec.Set("priority", priority)
	rec.Set("source", source)
	rec.Set("asset", strings.TrimSpace(p.Asset))
	// A location code resolves to the structured relation; free-text Location is
	// the note. An unresolved code is kept as a breadcrumb in the note.
	locNote := strings.TrimSpace(p.Location)
	if code := strings.TrimSpace(p.LocationCode); code != "" {
		if locID, ok := resolveLocation(app, customer.Id, code); ok {
			rec.Set("location", locID)
		} else if locNote == "" {
			locNote = code
		}
	}
	rec.Set("location_note", locNote)
	if key := strings.TrimSpace(p.Category); key != "" {
		if cat, err := app.FindFirstRecordByFilter(
			"ticket_categories", "key = {:k} && active = true",
			dbx.Params{"k": key}); err == nil && cat != nil {
			rec.Set("category", cat.Id)
		}
	}
	if p.DedupeKey != "" {
		rec.Set("dedupe_key", p.DedupeKey)
	}
	// Best-effort requester match, scoped to this customer so a stray email
	// can never link a ticket across tenants.
	if email := strings.TrimSpace(p.RequesterEmail); email != "" {
		if user, err := app.FindFirstRecordByFilter(
			"users", "email = {:e} && customer = {:c}",
			dbx.Params{"e": email, "c": customer.Id}); err == nil && user != nil {
			rec.Set("requester", user.Id)
		}
	}
	if err := app.Save(rec); err != nil {
		return nil, false, err
	}
	return rec, true, nil
}

// handleTokenReveal returns the customer's webhook token to admin staff,
// minting one on first use, or rotating on ?rotate=1. Route-level (not a
// collection rule) because the field is Hidden — it never rides the record
// API.
func handleTokenReveal(re *core.RequestEvent) error {
	if re.Auth == nil ||
		re.Auth.Collection().Name != authz.StaffCollection ||
		re.Auth.GetString("role") != "admin" {
		return re.ForbiddenError("admin staff only", nil)
	}
	customer, err := re.App.FindRecordById("customers", re.Request.PathValue("id"))
	if err != nil {
		return re.NotFoundError("customer not found", nil)
	}

	token := customer.GetString("webhook_token")
	if token == "" || re.Request.URL.Query().Get("rotate") != "" {
		token, err = mintToken()
		if err != nil {
			return re.InternalServerError("mint token failed", err)
		}
		customer.Set("webhook_token", token)
		if err := re.App.Save(customer); err != nil {
			return re.InternalServerError("save token failed", err)
		}
	}
	return re.JSON(http.StatusOK, map[string]any{"token": token})
}

// resolveLocation maps a location code to a locations id within the customer
// (empty/unknown → ("", false)); no auto-stub, matching the ingest path.
// Customer-scoped so a code can never resolve across tenants.
func resolveLocation(app core.App, customerID, code string) (string, bool) {
	code = strings.TrimSpace(code)
	if code == "" {
		return "", false
	}
	loc, err := app.FindFirstRecordByFilter(
		"locations", "customer = {:c} && code = {:code}",
		dbx.Params{"c": customerID, "code": code})
	if err != nil || loc == nil {
		return "", false
	}
	return loc.Id, true
}

func mintToken() (string, error) {
	raw := make([]byte, tokenEntropy)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("read entropy: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}
