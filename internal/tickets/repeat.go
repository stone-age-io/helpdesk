package tickets

import (
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/types"

	"github.com/stone-age-io/helpdesk/internal/notifications"
)

// CountRepeat records that a machine source (NATS or webhook) re-sent the
// dedupe_key of a ticket that is still open: repeat_count + 1, last_repeat_at =
// now (migration 1831000000). The intakes call it in place of creating a
// ticket; email and maintenance never do — their keys are idempotency tokens,
// not "the same problem again".
//
// Silent. The ticket update hook only mails on a status or assignee change, so
// this save would send nothing anyway; Suppress keeps it that way if that hook
// ever grows. A plain read-modify-write: two deliveries racing on one ticket
// can lose an increment, which is fine for a count nobody bills on.
func CountRepeat(app core.App, ticket *core.Record) error {
	ticket.Set("repeat_count", ticket.GetInt("repeat_count")+1)
	ticket.Set("last_repeat_at", types.NowDateTime())
	notifications.Suppress(ticket)
	return app.Save(ticket)
}
