package migrations

import (
	"fmt"

	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"

	"github.com/stone-age-io/helpdesk/internal/authz"
)

// Two read-rule widenings so the requester portal can show more ticket
// context (status timeline + category), without breaching the identity
// boundary:
//
//   - ticket_events: requesters may read the STATUS-change events on their own
//     company's tickets — enough for a progress timeline (Opened → In progress
//     → Resolved). Scoped to `field = 'status'`, so priority/assignee events
//     (whose values are staff names — the roster we hide) never match; status
//     old/new are plain enum strings. The actor relations stay gated by their
//     own view rules, so an actor expand is dropped for a requester (same
//     mechanism that hides the visit technician), and the portal never asks
//     for it.
//   - ticket_categories: requesters may read the category labels so a ticket's
//     category badge resolves. Categories are a global, non-sensitive taxonomy
//     ("Hardware", "Network"); reads only, all writes remain admin-only.
//
// Idempotent: re-applies the same rules. Writes are untouched.

func init() {
	m.Register(portalTicketContextUp, portalTicketContextDown)
}

func portalTicketContextUp(app core.App) error {
	events, err := app.FindCollectionByNameOrId("ticket_events")
	if err != nil {
		return fmt.Errorf("find ticket_events: %w", err)
	}
	statusRead := authz.StaffRule +
		" || (" + authz.RequesterRule + " && field = 'status' && ticket.customer = @request.auth.customer)"
	events.ListRule = &statusRead
	events.ViewRule = &statusRead
	if err := app.Save(events); err != nil {
		return fmt.Errorf("save ticket_events: %w", err)
	}

	categories, err := app.FindCollectionByNameOrId("ticket_categories")
	if err != nil {
		return fmt.Errorf("find ticket_categories: %w", err)
	}
	catRead := authz.StaffRule + " || " + authz.RequesterRule
	categories.ListRule = &catRead
	categories.ViewRule = &catRead
	if err := app.Save(categories); err != nil {
		return fmt.Errorf("save ticket_categories: %w", err)
	}
	return nil
}

// portalTicketContextDown restores the staff-only reads.
func portalTicketContextDown(app core.App) error {
	staffRule := authz.StaffRule
	for _, name := range []string{"ticket_events", "ticket_categories"} {
		col, err := app.FindCollectionByNameOrId(name)
		if err != nil {
			continue
		}
		col.ListRule = &staffRule
		col.ViewRule = &staffRule
		if err := app.Save(col); err != nil {
			return fmt.Errorf("save %s: %w", name, err)
		}
	}
	return nil
}
