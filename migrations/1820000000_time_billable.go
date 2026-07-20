package migrations

import (
	"fmt"

	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

// Billability: one bool on `time_entries`. It is stored as `non_billable`
// (default false) rather than `billable` (default true) on purpose — a
// PocketBase bool has no unset state, so its zero value is false. Naming the
// flag for the exception makes the zero value mean "billable", which is the
// intended default, with NO backfill of existing rows, NO defaulting hook, and
// NO per-writer discipline: every present and future writer (the UI, the timer
// stop route, even a raw API create) is safe by construction. A field named
// `billable` would instead default to false = un-billed on any omission — a
// silent revenue leak. Reports compute billable = total − non_billable; the
// (future) NATS envelope would publish `billable: !non_billable`.
//
// Labor is the right home for the flag, not the ticket: one ticket routinely
// mixes billable work with non-billable rework/goodwill, so billability is a
// property of the entry, not the issue. Staff-only already (the collection's
// rules are unchanged) — this is just data on the ledger.

func init() {
	m.Register(timeBillableUp, timeBillableDown)
}

func timeBillableUp(app core.App) error {
	entries, err := app.FindCollectionByNameOrId("time_entries")
	if err != nil {
		return fmt.Errorf("find time_entries: %w", err)
	}
	if entries.Fields.GetByName("non_billable") == nil {
		entries.Fields.Add(&core.BoolField{Name: "non_billable"})
		if err := app.Save(entries); err != nil {
			return fmt.Errorf("save time_entries: %w", err)
		}
	}
	return nil
}

// timeBillableDown is dev-loop only, like the other down paths.
func timeBillableDown(app core.App) error {
	if entries, err := app.FindCollectionByNameOrId("time_entries"); err == nil {
		entries.Fields.RemoveByName("non_billable")
		if err := app.Save(entries); err != nil {
			return fmt.Errorf("save time_entries: %w", err)
		}
	}
	return nil
}
