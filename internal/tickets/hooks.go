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
		// Reactive issue unless a caller (staff UI, project setup) says install.
		if e.Record.GetString("type") == "" {
			e.Record.Set("type", "issue")
		}
		return e.Next()
	})

	// Resolving or closing a ticket clears the "waiting on requester" flag — the
	// ball is no longer in anyone's court. Pre-save so it rides the same write
	// as the status change (no extra round-trip); setting it true is driven by
	// staff comments below.
	app.OnRecordUpdate("tickets").BindFunc(func(e *core.RecordEvent) error {
		if now := e.Record.GetString("status"); now == "resolved" || now == "closed" {
			if e.Record.Original().GetString("status") != now {
				e.Record.Set("awaiting_requester", false)
			}
		}
		return e.Next()
	})

	// Public comments drive the conversation state. A requester replying on a
	// resolved/closed ticket reopens it (if they still have a problem, it isn't
	// done) and, either way, puts the ball back with staff. A public staff reply
	// puts it in the requester's court (awaiting_requester = true) ONLY when the
	// author explicitly asked for a reply (requests_reply) — a plain status
	// update shouldn't nag the customer. Internal notes change nothing. The
	// reopen is silent (Suppress): the comment itself already emailed staff, so a
	// second status-change mail would be noise.
	app.OnRecordAfterCreateSuccess("ticket_comments").BindFunc(func(e *core.RecordEvent) error {
		if e.Record.GetBool("internal") {
			return e.Next()
		}
		ticket, err := e.App.FindRecordById("tickets", e.Record.GetString("ticket"))
		if err != nil {
			return e.Next()
		}
		if userID := e.Record.GetString("author_user"); userID != "" {
			handleRequesterReply(e.App, ticket, userID)
		} else if e.Record.GetString("author_staff") != "" && e.Record.GetBool("requests_reply") {
			markAwaitingRequester(e.App, ticket)
		}
		return e.Next()
	})
}

// handleRequesterReply reopens a done ticket and clears awaiting_requester in a
// single save. Best-effort: a failed follow-up must not fail the comment write.
func handleRequesterReply(app core.App, ticket *core.Record, userID string) {
	changed := false
	switch ticket.GetString("status") {
	case "resolved", "closed":
		ticket.Set("status", "open")
		notifications.Suppress(ticket)
		// Attribute the reopen to the requester whose comment triggered it so
		// the audit timeline names them, not "system".
		activity.SetActor(ticket, "users", userID)
		changed = true
	}
	if ticket.GetBool("awaiting_requester") {
		ticket.Set("awaiting_requester", false)
		changed = true
	}
	if changed {
		if err := app.Save(ticket); err != nil {
			slog.Warn("requester-reply follow-up failed", "ticket", ticket.Id, "err", err)
		}
	}
}

// markAwaitingRequester flags that staff are waiting on the requester. Install
// tickets are excluded — proactive field work isn't a reply-driven conversation
// (its status is tracked by visits/project, not a customer answer). A public
// note on an already-resolved/closed ticket doesn't ask for anything, and a
// no-op when the flag is already set avoids a redundant write.
func markAwaitingRequester(app core.App, ticket *core.Record) {
	if ticket.GetString("type") == "install" {
		return
	}
	if st := ticket.GetString("status"); st == "resolved" || st == "closed" {
		return
	}
	if ticket.GetBool("awaiting_requester") {
		return
	}
	ticket.Set("awaiting_requester", true)
	if err := app.Save(ticket); err != nil {
		slog.Warn("set awaiting_requester failed", "ticket", ticket.Id, "err", err)
	}
}

func nextNumber(app core.App) int {
	var max int
	_ = app.DB().
		Select("COALESCE(MAX(number), 0)").
		From("tickets").
		Row(&max)
	return max + 1
}
