// Package activity records a ticket audit trail: one ticket_events row per
// workflow-field change (status, priority, assignee), attributed to whoever
// made it. It reuses the transient-record-flag trick from notifications —
// the request hook stamps the actor onto the record, and the after-success
// model hook (which sees old vs new but not the HTTP request) reads it back.
package activity

import (
	"log/slog"

	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/core"
)

// Transient (non-persisted) keys carrying the acting identity from the
// request hook to the after-success hook. Custom keys ride the shared record
// instance and are never written to a column.
const (
	actorColField = "_actorCollection"
	actorIDField  = "_actorID"
)

// auditedFields are the workflow fields whose changes are worth a timeline
// entry. Title/body edits are deliberately excluded — too noisy, no workflow
// meaning.
var auditedFields = []string{"status", "priority", "assignee"}

// SetActor attributes a programmatic ticket change to a specific identity so
// the audit hook can name it — e.g. the requester whose comment auto-reopened
// a ticket. API-driven changes get the actor from the request auth instead.
func SetActor(r *core.Record, collection, id string) {
	r.SetRaw(actorColField, collection)
	r.SetRaw(actorIDField, id)
}

// Register binds the actor-capture request hook and the diff-logging
// after-success hook.
func Register(app *pocketbase.PocketBase) {
	app.OnRecordUpdateRequest("tickets").BindFunc(func(e *core.RecordRequestEvent) error {
		if e.Auth != nil {
			SetActor(e.Record, e.Auth.Collection().Name, e.Auth.Id)
		}
		return e.Next()
	})

	app.OnRecordAfterUpdateSuccess("tickets").BindFunc(func(e *core.RecordEvent) error {
		orig := e.Record.Original()
		for _, f := range auditedFields {
			if old, now := orig.GetString(f), e.Record.GetString(f); old != now {
				logEvent(e.App, e.Record, f, old, now)
			}
		}
		return e.Next()
	})
}

func logEvent(app core.App, ticket *core.Record, field, old, now string) {
	col, err := app.FindCollectionByNameOrId("ticket_events")
	if err != nil {
		slog.Warn("ticket_events collection missing", "err", err)
		return
	}
	rec := core.NewRecord(col)
	rec.Set("ticket", ticket.Id)
	rec.Set("field", field)
	rec.Set("old_value", displayValue(app, field, old))
	rec.Set("new_value", displayValue(app, field, now))
	switch ticket.GetString(actorColField) {
	case "staff":
		rec.Set("actor_staff", ticket.GetString(actorIDField))
	case "users":
		rec.Set("actor_user", ticket.GetString(actorIDField))
	}
	if err := app.Save(rec); err != nil {
		slog.Warn("write ticket event failed", "ticket", ticket.Id, "field", field, "err", err)
	}
}

// displayValue turns a stored value into something the timeline can show
// verbatim: an assignee id becomes a staff name (empty → "Unassigned");
// status and priority are already their own labels.
func displayValue(app core.App, field, val string) string {
	if field != "assignee" {
		return val
	}
	if val == "" {
		return "Unassigned"
	}
	if s, err := app.FindRecordById("staff", val); err == nil {
		return s.GetString("name")
	}
	return val
}
