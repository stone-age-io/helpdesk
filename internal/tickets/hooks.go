// Package tickets owns ticket lifecycle glue that can't live in collection
// rules: sequential number assignment and field defaults.
package tickets

import (
	"log/slog"

	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/core"

	"github.com/stone-age-io/helpdesk/internal/activity"
	"github.com/stone-age-io/helpdesk/internal/notifications"
)

// Register binds the ticket create hook (sequential number + field defaults)
// and the comment-driven auto-reopen. PocketBase serializes writes on one
// SQLite connection, and the unique index on `number` is the collision
// backstop.
func Register(app *pocketbase.PocketBase) {
	app.OnRecordCreate("tickets").BindFunc(func(e *core.RecordEvent) error {
		if e.Record.GetInt("number") == 0 {
			e.Record.Set("number", nextNumber(e.App))
		}
		if e.Record.GetString("status") == "" {
			e.Record.Set("status", "open")
		}
		if e.Record.GetString("priority") == "" {
			e.Record.Set("priority", "normal")
		}
		if e.Record.GetString("source") == "" {
			e.Record.Set("source", "agent")
		}
		return e.Next()
	})

	// A requester replying on a resolved or closed ticket reopens it — if they
	// still have a problem, it isn't done. Internal notes and staff comments
	// never reopen. The reopen is silent (Suppress): the comment itself
	// already emailed staff, so a second status-change mail would be noise.
	app.OnRecordAfterCreateSuccess("ticket_comments").BindFunc(func(e *core.RecordEvent) error {
		if e.Record.GetBool("internal") || e.Record.GetString("author_user") == "" {
			return e.Next()
		}
		ticket, err := e.App.FindRecordById("tickets", e.Record.GetString("ticket"))
		if err != nil {
			return e.Next()
		}
		switch ticket.GetString("status") {
		case "resolved", "closed":
			ticket.Set("status", "open")
			notifications.Suppress(ticket)
			// Attribute the reopen to the requester whose comment triggered it
			// so the audit timeline names them, not "system".
			activity.SetActor(ticket, "users", e.Record.GetString("author_user"))
			if err := e.App.Save(ticket); err != nil {
				// Best-effort: a failed reopen must not fail the comment write.
				slog.Warn("auto-reopen failed", "ticket", ticket.Id, "err", err)
			}
		}
		return e.Next()
	})
}

func nextNumber(app core.App) int {
	var max int
	_ = app.DB().
		Select("COALESCE(MAX(number), 0)").
		From("tickets").
		Row(&max)
	return max + 1
}
