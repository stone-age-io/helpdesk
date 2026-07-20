package tickets

import (
	"time"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"

	"github.com/stone-age-io/helpdesk/internal/notifications"
)

// AutoCloseResolved promotes tickets that have sat `resolved` untouched for at
// least `days` to `closed`, and returns how many it closed. It is the second
// stage of the lifecycle: `resolved` is a grace window (a requester reply
// reopens it), and this sweep finalizes the ones nobody came back to.
//
// The mail is suppressed — auto-close is administrative, not a "we closed your
// ticket" message — while the status change still writes a `ticket_events` row
// (no actor = system) via the after-update hook, so the timeline records it.
// Exported and broker-free so it can be driven directly in tests (the sibling
// convention); cmd/helpdesk wires it to a daily cron. days <= 0 is a no-op, so
// a disabled config can't accidentally close anything.
func AutoCloseResolved(app core.App, days int) (int, error) {
	if days <= 0 {
		return 0, nil
	}
	cutoff := time.Now().UTC().AddDate(0, 0, -days).Format("2006-01-02 15:04:05.000Z")
	// resolved_at != '' skips pre-migration resolved tickets (null age); they
	// close once re-resolved. Reading the field, not `updated`, means an
	// unrelated edit never resets the clock.
	stale, err := app.FindRecordsByFilter(
		"tickets",
		"status = 'resolved' && resolved_at != '' && resolved_at < {:cutoff}",
		"", 0, 0,
		dbx.Params{"cutoff": cutoff},
	)
	if err != nil {
		return 0, err
	}
	closed := 0
	for _, t := range stale {
		t.Set("status", "closed")
		notifications.Suppress(t)
		if err := app.Save(t); err != nil {
			// Best-effort per ticket: one failure shouldn't abort the sweep.
			continue
		}
		closed++
	}
	return closed, nil
}
