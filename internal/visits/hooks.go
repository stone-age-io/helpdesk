// Package visits owns visit lifecycle glue that can't live in collection
// rules: status defaulting and the scheduled-visit invariant.
package visits

import (
	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
)

// Register binds the visit create/update guard. A visit with no status
// defaults to `scheduled` when a time is set (the pre-dispatch creation
// path) and `requested` otherwise (an agent promoting a ticket to on-site
// work before anyone knows the tech or the time). A `scheduled` visit must
// carry both a time and a technician — that invariant is the whole state
// machine; legal transitions are deliberately not enforced, staff writes
// are trusted.
func Register(app *pocketbase.PocketBase) {
	guard := func(e *core.RecordEvent) error {
		if e.Record.GetString("status") == "" {
			if e.Record.GetDateTime("scheduled_at").IsZero() {
				e.Record.Set("status", "requested")
			} else {
				e.Record.Set("status", "scheduled")
			}
		}
		if e.Record.GetString("status") == "scheduled" &&
			(e.Record.GetDateTime("scheduled_at").IsZero() || e.Record.GetString("assignee") == "") {
			return apis.NewBadRequestError("a scheduled visit needs a time and a technician", nil)
		}
		return e.Next()
	}
	app.OnRecordCreate("visits").BindFunc(guard)
	app.OnRecordUpdate("visits").BindFunc(guard)
}
