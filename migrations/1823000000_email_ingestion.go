package migrations

import (
	"fmt"

	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

// Email ingestion (inbound). Three additive schema changes that let an
// email-parsing provider (Postmark to start) turn mail into tickets and
// comments via an authenticated webhook. No collection RULE changes — the
// ingest path writes server-side with app.Save, which bypasses rules. See
// docs/email-ingestion.md for the full design.
//
//   - tickets.source gains "email" — a distinct provenance alongside
//     portal/agent/nats/webhook, so an emailed ticket is queryable as such.
//   - customers.email_domain — optional, UNIQUE when set. The domain rung of
//     the customer-resolution ladder (a sender at acme.com maps to Acme when no
//     exact user match exists). Optional because a customer may be a single
//     contact on a shared provider (gmail.com) with no domain of their own; a
//     public-provider domain is rejected at write time by internal/customers.
//   - ticket_comments.source_message_id — hidden, UNIQUE when set. The email
//     Message-ID, the idempotency backstop for the reply→comment path, same
//     idiom as tickets.dedupe_key (unique partial index is the real guard).
//
// Idempotent: each change is guarded by field presence, so a re-run is a no-op.

func init() {
	m.Register(emailIngestionUp, emailIngestionDown)
}

func emailIngestionUp(app core.App) error {
	tickets, err := app.FindCollectionByNameOrId("tickets")
	if err != nil {
		return fmt.Errorf("find tickets: %w", err)
	}
	source, ok := tickets.Fields.GetByName("source").(*core.SelectField)
	if !ok {
		return fmt.Errorf("tickets.source is not a select field")
	}
	if !hasValue(source.Values, "email") {
		source.Values = append(source.Values, "email")
		if err := app.Save(tickets); err != nil {
			return fmt.Errorf("save tickets: %w", err)
		}
	}

	customers, err := app.FindCollectionByNameOrId("customers")
	if err != nil {
		return fmt.Errorf("find customers: %w", err)
	}
	if customers.Fields.GetByName("email_domain") == nil {
		customers.Fields.Add(&core.TextField{Name: "email_domain", Max: 253})
		// Partial unique: a domain maps to exactly one tenant; blank customers
		// (shared-provider contacts) are exempt. Mirrors idx_customers_platform_org.
		customers.AddIndex("idx_customers_email_domain", true, "email_domain", "email_domain != ''")
		if err := app.Save(customers); err != nil {
			return fmt.Errorf("save customers: %w", err)
		}
	}

	comments, err := app.FindCollectionByNameOrId("ticket_comments")
	if err != nil {
		return fmt.Errorf("find ticket_comments: %w", err)
	}
	if comments.Fields.GetByName("source_message_id") == nil {
		comments.Fields.Add(&core.TextField{Name: "source_message_id", Hidden: true, Max: 512})
		comments.AddIndex("idx_ticket_comments_source_msgid", true, "source_message_id", "source_message_id != ''")
		if err := app.Save(comments); err != nil {
			return fmt.Errorf("save ticket_comments: %w", err)
		}
	}
	return nil
}

// emailIngestionDown is dev-loop only, like the other down paths: drop the two
// added fields (their indexes go with them) and the source value.
func emailIngestionDown(app core.App) error {
	if err := removeField(app, "customers", "email_domain"); err != nil {
		return err
	}
	if err := removeField(app, "ticket_comments", "source_message_id"); err != nil {
		return err
	}
	tickets, err := app.FindCollectionByNameOrId("tickets")
	if err != nil {
		return nil
	}
	source, ok := tickets.Fields.GetByName("source").(*core.SelectField)
	if !ok {
		return nil
	}
	out := source.Values[:0]
	for _, v := range source.Values {
		if v != "email" {
			out = append(out, v)
		}
	}
	source.Values = out
	return app.Save(tickets)
}
