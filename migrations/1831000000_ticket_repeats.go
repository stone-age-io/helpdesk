package migrations

import (
	"fmt"

	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

// tickets.repeat_count + last_repeat_at: how often a machine source re-sent a
// ticket that was still open.
//
// A NATS or webhook dedupe_key used to suppress forever — the lookups carry no
// status filter, so a flapping alarm with a stable key was silenced for good
// by its first ticket, even after that ticket was closed. Now a nats/webhook
// ticket gives its key back when it is resolved or closed (internal/tickets),
// so the next occurrence opens a fresh ticket, and a repeat that lands while
// the ticket is still open is COUNTED here instead of vanishing. The count is
// of deliveries, so a webhook caller's own retries are included.
//
// Email (Message-ID) and maintenance (pm:{plan}:{date}) keys are unchanged:
// they are idempotency tokens for one message / one occurrence, not "the same
// problem again", and both rely on the key surviving closure — a Postmark retry
// after the ticket closed, and the PM crash-recovery path. They never count.
//
// Server-only. The fields ride the tickets read rules like any other column,
// and a request hook in internal/tickets pins them on every record-API write,
// so neither a requester nor a staff PATCH can set or roll them back. No index
// change: the partial unique index on (customer, dedupe_key) from 1830000000
// stays the race backstop, and a released (empty) key falls outside it.
func init() {
	m.Register(ticketRepeatsUp, ticketRepeatsDown)
}

func ticketRepeatsUp(app core.App) error {
	tickets, err := app.FindCollectionByNameOrId("tickets")
	if err != nil {
		return fmt.Errorf("find tickets: %w", err)
	}
	if tickets.Fields.GetByName("repeat_count") != nil {
		return nil // idempotent
	}
	min := 0.0
	tickets.Fields.Add(&core.NumberField{Name: "repeat_count", OnlyInt: true, Min: &min})
	tickets.Fields.Add(&core.DateField{Name: "last_repeat_at"})
	if err := app.Save(tickets); err != nil {
		return fmt.Errorf("save tickets: %w", err)
	}
	return nil
}

// ticketRepeatsDown is dev-loop only, like the other down paths.
func ticketRepeatsDown(app core.App) error {
	if tickets, err := app.FindCollectionByNameOrId("tickets"); err == nil {
		tickets.Fields.RemoveByName("repeat_count")
		tickets.Fields.RemoveByName("last_repeat_at")
		if err := app.Save(tickets); err != nil {
			return fmt.Errorf("save tickets: %w", err)
		}
	}
	return nil
}
