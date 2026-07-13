// Package ingest owns the NATS→ticket pipeline: a durable JetStream
// consumer on the hub-side HELPDESK_EVENTS stream that projects
// machine-published events into ticket records (kiosk controller pattern).
//
// Provenance: the customer org id is parsed from subject token 2 — injected
// by the platform's operator-signed export/import — and NEVER from the
// payload. An event whose org has no mapped customer is logged and acked;
// once the operator sets the customer's platform_org_id, later events flow.
package ingest

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"

	"github.com/stone-age-io/helpdesk/internal/subjects"
)

// CreatePayload is the wire shape customer-side apps publish to
// helpdesk.tickets.create (documented in docs/protocol.md). Unknown fields
// are ignored; only title is required.
type CreatePayload struct {
	Title    string `json:"title"`
	Body     string `json:"body,omitempty"`
	Priority string `json:"priority,omitempty"` // low|normal|high|urgent; anything else → normal
	// DedupeKey makes ingestion idempotent per customer event: a publisher
	// that retries (or a flapping sensor stamping a stable key) creates one
	// ticket, not many.
	DedupeKey string `json:"dedupe_key,omitempty"`
	// Thing is an optional provenance hint stored as the ticket's asset.
	// Location is free text stored as location_note. LocationCode is the
	// platform Location join key: it resolves to a locations row for this
	// customer and only falls back to location_note when it can't be matched.
	Thing        string `json:"thing,omitempty"`
	Location     string `json:"location,omitempty"`
	LocationCode string `json:"location_code,omitempty"`
	// Category is an optional ticket_categories key. An unknown or inactive
	// key is ignored (the ticket is still created, unclassified) — same
	// graceful-degradation stance as an unmapped org.
	Category string `json:"category,omitempty"`
}

// Outcome tells the dispatcher how to ack the underlying JetStream message.
// Pulled out so projection logic is testable without a real jetstream.Msg.
type Outcome int

const (
	// Ack — success or terminal skip (unknown org, duplicate, bad payload):
	// redelivery cannot change the result.
	Ack Outcome = iota
	// Retry — transient failure (DB hiccup): let JetStream redeliver.
	Retry
)

// Consumer owns the durable-consumer lifecycle. Start launches the consume
// loop; Stop drains it.
type Consumer struct {
	app     core.App
	js      jetstream.JetStream
	stream  string
	durable string
	subj    subjects.Subjects

	consumeCC jetstream.ConsumeContext
}

// New wires the consumer; call Start to begin consuming. The stream must
// already exist (natsx.EnsureStream in main's OnServe).
func New(app core.App, js jetstream.JetStream, stream, durable string, subj subjects.Subjects) *Consumer {
	return &Consumer{app: app, js: js, stream: stream, durable: durable, subj: subj}
}

// Start provisions the durable consumer (idempotent) and begins consuming.
func (c *Consumer) Start(ctx context.Context) error {
	stream, err := c.js.Stream(ctx, c.stream)
	if err != nil {
		return fmt.Errorf("open stream %q: %w", c.stream, err)
	}
	cons, err := stream.CreateOrUpdateConsumer(ctx, jetstream.ConsumerConfig{
		Durable:       c.durable,
		Description:   "helpdesk ingest: projects machine events into tickets",
		DeliverPolicy: jetstream.DeliverAllPolicy,
		AckPolicy:     jetstream.AckExplicitPolicy,
		// Generous headroom over worst-case projection latency (SQLite is
		// single-writer); a handler that exceeds AckWait would be redelivered
		// while still in flight. Redelivery of a completed create is absorbed
		// by the dedupe_key unique index anyway.
		AckWait:        60 * time.Second,
		MaxAckPending:  64,
		FilterSubjects: c.subj.StreamWildcards(),
	})
	if err != nil {
		return fmt.Errorf("ensure consumer %q: %w", c.durable, err)
	}
	cc, err := cons.Consume(func(msg jetstream.Msg) {
		switch c.Project(msg.Subject(), msg.Data()) {
		case Ack:
			_ = msg.Ack()
		case Retry:
			_ = msg.Nak()
		}
	})
	if err != nil {
		return fmt.Errorf("start consume: %w", err)
	}
	c.consumeCC = cc
	slog.Info("ingest consumer started", "stream", c.stream, "durable", c.durable)
	return nil
}

// Stop tears down the consume loop. Safe to call multiple times or on nil.
func (c *Consumer) Stop() {
	if c == nil {
		return
	}
	if c.consumeCC != nil {
		c.consumeCC.Stop()
		c.consumeCC = nil
	}
}

var validPriorities = []string{"low", "normal", "high", "urgent"}

// Project is the pure-state effect of one inbound message: parse, map the
// org to a customer, dedupe, create the ticket. Exposed for tests, which
// drive it without a broker.
func (c *Consumer) Project(subject string, data []byte) Outcome {
	orgID, verb, ok := c.subj.ParseTicketEvent(subject)
	if !ok {
		slog.Warn("ingest: unparseable subject", "subject", subject)
		return Ack
	}
	if verb != subjects.VerbCreate {
		// Grammar reserves comment/resolve for later; ack so the stream
		// drains, log so the operator sees a publisher running ahead of us.
		slog.Info("ingest: unsupported verb", "subject", subject, "verb", verb)
		return Ack
	}

	var payload CreatePayload
	if err := json.Unmarshal(data, &payload); err != nil {
		slog.Warn("ingest: bad payload", "subject", subject, "err", err)
		return Ack
	}
	if strings.TrimSpace(payload.Title) == "" {
		slog.Warn("ingest: missing title", "subject", subject)
		return Ack
	}

	customer, err := c.app.FindFirstRecordByFilter(
		"customers", "platform_org_id = {:org}", dbx.Params{"org": orgID})
	if err != nil {
		if !isNotFound(err) {
			slog.Warn("ingest: customer lookup failed", "org", orgID, "err", err)
			return Retry
		}
		// Unmapped org: the operator hasn't linked this platform org to a
		// customer yet. Ack — the event is gone, but the mapping gap is loud
		// in the logs and later events flow once it's fixed.
		slog.Warn("ingest: no customer mapped for platform org — set customers.platform_org_id", "org", orgID, "subject", subject)
		return Ack
	}

	if payload.DedupeKey != "" {
		existing, err := c.app.FindFirstRecordByFilter(
			"tickets", "dedupe_key = {:k}", dbx.Params{"k": payload.DedupeKey})
		if err != nil && !isNotFound(err) {
			slog.Warn("ingest: dedupe lookup failed", "err", err)
			return Retry
		}
		if existing != nil {
			return Ack // already projected (redelivery or publisher retry)
		}
	}

	col, err := c.app.FindCollectionByNameOrId("tickets")
	if err != nil {
		slog.Warn("ingest: tickets collection missing", "err", err)
		return Retry
	}

	priority := payload.Priority
	if !slices.Contains(validPriorities, priority) {
		priority = "normal"
	}

	rec := core.NewRecord(col)
	rec.Set("customer", customer.Id)
	rec.Set("title", strings.TrimSpace(payload.Title))
	rec.Set("body", strings.TrimSpace(payload.Body))
	rec.Set("priority", priority)
	rec.Set("source", "nats")
	rec.Set("origin_subject", subject)
	// Provenance hints as structured, filterable/reportable fields (they used
	// to be folded into the body as a trailing [thing · location] line).
	rec.Set("asset", strings.TrimSpace(payload.Thing))
	// A location code resolves to the structured relation (the reporting axis);
	// free-text Location is the note. An unresolved code is logged and kept as a
	// breadcrumb in the note so the operator can create the missing locations row.
	locNote := strings.TrimSpace(payload.Location)
	if code := strings.TrimSpace(payload.LocationCode); code != "" {
		if locID, ok := resolveLocation(c.app, customer.Id, code); ok {
			rec.Set("location", locID)
		} else {
			slog.Warn("ingest: unresolved location code — create a locations row with this code",
				"code", code, "customer", customer.GetString("name"), "subject", subject)
			if locNote == "" {
				locNote = code
			}
		}
	}
	rec.Set("location_note", locNote)
	if catID := c.resolveCategory(payload.Category); catID != "" {
		rec.Set("category", catID)
	}
	if payload.DedupeKey != "" {
		rec.Set("dedupe_key", payload.DedupeKey)
	}
	if err := c.app.Save(rec); err != nil {
		if isUniqueViolation(err) {
			return Ack // dedupe race: another delivery won; the ticket exists
		}
		slog.Warn("ingest: save ticket failed", "err", err)
		return Retry
	}
	slog.Info("ingest: ticket created",
		"number", rec.GetInt("number"), "customer", customer.GetString("name"), "subject", subject)
	return Ack
}

// resolveCategory maps an optional category key to a ticket_categories id.
// Empty/unknown/inactive → "" (leave the ticket unclassified) rather than an
// error: a machine publisher naming a category the operator hasn't created
// shouldn't drop the ticket.
func (c *Consumer) resolveCategory(key string) string {
	key = strings.TrimSpace(key)
	if key == "" {
		return ""
	}
	cat, err := c.app.FindFirstRecordByFilter(
		"ticket_categories", "key = {:k} && active = true", dbx.Params{"k": key})
	if err != nil || cat == nil {
		return ""
	}
	return cat.Id
}

// resolveLocation maps a location code to a locations id within the customer.
// Empty/unknown → ("", false); callers keep a free-text fallback rather than
// auto-creating an addressless stub (worse than nothing for a dispatched tech).
// Scoped by customer so a code can never resolve across tenants.
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

func isNotFound(err error) bool {
	return err != nil && strings.Contains(strings.ToLower(err.Error()), "no rows")
}

func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "unique") || strings.Contains(msg, "constraint")
}
