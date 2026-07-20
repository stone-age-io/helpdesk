package migrations

import (
	"fmt"

	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

// resolved_at: a nullable datetime stamped when a ticket enters `resolved` and
// cleared when it leaves. It gives the auto-close cron a trustworthy "how long
// has this been resolved" signal (unlike `updated`, which bumps on any edit) —
// the same reason visits carry `completed_at`. One nullable column, stamped by
// the `internal/tickets` guard hook; no re-resolve history.
//
// This is the storage half of the two-stage lifecycle: `resolved` is a grace
// window (requester reply reopens it), and after `auto_close_resolved_days` of
// silence the cron promotes it to `closed` (final; requesters can't reply — see
// migration `1822000000`). Existing resolved tickets have a null `resolved_at`
// and simply won't auto-close until they're re-resolved — acceptable, no
// backfill.

func init() {
	m.Register(ticketResolvedAtUp, ticketResolvedAtDown)
}

func ticketResolvedAtUp(app core.App) error {
	tickets, err := app.FindCollectionByNameOrId("tickets")
	if err != nil {
		return fmt.Errorf("find tickets: %w", err)
	}
	if tickets.Fields.GetByName("resolved_at") == nil {
		tickets.Fields.Add(&core.DateField{Name: "resolved_at"})
		if err := app.Save(tickets); err != nil {
			return fmt.Errorf("save tickets: %w", err)
		}
	}
	return nil
}

// ticketResolvedAtDown is dev-loop only, like the other down paths.
func ticketResolvedAtDown(app core.App) error {
	if tickets, err := app.FindCollectionByNameOrId("tickets"); err == nil {
		tickets.Fields.RemoveByName("resolved_at")
		if err := app.Save(tickets); err != nil {
			return fmt.Errorf("save tickets: %w", err)
		}
	}
	return nil
}
