// Package visits owns visit lifecycle glue that can't live in collection
// rules: status defaulting and the scheduled-visit invariant.
package visits

import (
	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/types"
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
		// completed_at is the trustworthy "when did this happen" for reporting
		// (unlike `updated`, which bumps on any edit). Stamp it when a visit
		// enters `completed`, leaving a staff-supplied value intact so a visit
		// can be back-dated; clear it if the visit moves back out of completed.
		// Who completed it is the visit's `assignee` (the dispatched tech).
		if e.Record.GetString("status") == "completed" {
			if e.Record.GetDateTime("completed_at").IsZero() {
				e.Record.Set("completed_at", types.NowDateTime())
			}
		} else {
			e.Record.Set("completed_at", "")
		}
		return e.Next()
	}
	app.OnRecordCreate("visits").BindFunc(guard)
	app.OnRecordUpdate("visits").BindFunc(guard)
}
