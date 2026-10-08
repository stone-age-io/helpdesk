package migrations

import (
	"fmt"

	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

// tickets.dedupe_key becomes unique per CUSTOMER rather than across the whole
// install.
//
// The key is publisher-chosen — a machine names its own retry token, the
// webhook caller names theirs, an email carries its Message-ID — and publishers
// in different tenants know nothing of each other. A single global index made
// their key spaces one: "pump-7-overcurrent" filed by one customer silently
// swallowed the same key from another (NATS acked it as a redelivery), and the
// webhook answered `duplicate: true` with the OTHER tenant's ticket id and
// number. Every intake already knows its customer before it dedupes, so the
// fix is to key on the pair, which is what the lookups now do as well.
//
// Safe on existing data: rows that were unique globally are unique per
// customer. Maintenance's `pm:{planId}:{date}` keys are unaffected — a plan id
// is already unique, and a plan belongs to one customer.
func init() {
	m.Register(ticketDedupePerCustomerUp, ticketDedupePerCustomerDown)
}

func ticketDedupePerCustomerUp(app core.App) error {
	tickets, err := app.FindCollectionByNameOrId("tickets")
	if err != nil {
		return fmt.Errorf("find tickets: %w", err)
	}
	if tickets.GetIndex("idx_tickets_customer_dedupe") != "" {
		return nil // idempotent
	}
	tickets.RemoveIndex("idx_tickets_dedupe")
	tickets.AddIndex("idx_tickets_customer_dedupe", true, "customer, dedupe_key", "dedupe_key != ''")
	if err := app.Save(tickets); err != nil {
		return fmt.Errorf("save tickets: %w", err)
	}
	return nil
}

func ticketDedupePerCustomerDown(app core.App) error {
	tickets, err := app.FindCollectionByNameOrId("tickets")
	if err != nil {
		return fmt.Errorf("find tickets: %w", err)
	}
	tickets.RemoveIndex("idx_tickets_customer_dedupe")
	tickets.AddIndex("idx_tickets_dedupe", true, "dedupe_key", "dedupe_key != ''")
	return app.Save(tickets)
}
