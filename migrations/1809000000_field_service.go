package migrations

import (
	"fmt"

	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

// Field-service verboseness: two small nullable additions that make on-site
// work first-class without turning the visit into a second source of truth.
//
//   visits.duration_minutes — the SCHEDULED block length. Paired with
//     scheduled_at it turns a visit from a pin on a timeline into a real
//     calendar block (capacity planning, "can I fit another at 4?"). This is
//     PLANNED time, deliberately distinct from the ACTUAL labor logged in
//     time_entries. Optional; positivity is enforced by the UI input (staff
//     writes are trusted), so there is no server Min — an optional number
//     field + Min interacts awkwardly with the empty value.
//
//   time_entries.visit — optionally attributes a labor entry to one on-site
//     session. The ticket stays the canonical ledger: `ticket` remains
//     required, so the ticket total is unchanged (sum of minutes by ticket);
//     `visit` is an added dimension giving per-visit subtotals and the
//     field-vs-desk split for free. No cascade delete — labor is real and must
//     survive a visit being removed: the entry keeps its ticket, and the
//     dangling visit ref simply resolves to nothing (same reasoning as
//     tickets.category in 1806000000). A ticket delete still cascades both the
//     visit and the time entry via their required `ticket` relations.
//
// No access-rule changes: both are added fields on existing collections.
// time_entries stays staff-only; requesters never see it.

func init() {
	m.Register(fieldServiceUp, fieldServiceDown)
}

func fieldServiceUp(app core.App) error {
	visits, err := app.FindCollectionByNameOrId("visits")
	if err != nil {
		return fmt.Errorf("find visits: %w", err)
	}
	if visits.Fields.GetByName("duration_minutes") == nil {
		visits.Fields.Add(&core.NumberField{Name: "duration_minutes", OnlyInt: true})
		if err := app.Save(visits); err != nil {
			return fmt.Errorf("save visits: %w", err)
		}
	}

	entries, err := app.FindCollectionByNameOrId("time_entries")
	if err != nil {
		return fmt.Errorf("find time_entries: %w", err)
	}
	if entries.Fields.GetByName("visit") == nil {
		entries.Fields.Add(&core.RelationField{
			Name:         "visit",
			CollectionId: visits.Id,
			MaxSelect:    1,
			// No CascadeDelete: deleting a visit must never delete labor.
		})
		entries.AddIndex("idx_time_entries_visit", false, "visit", "")
		if err := app.Save(entries); err != nil {
			return fmt.Errorf("save time_entries: %w", err)
		}
	}
	return nil
}

// fieldServiceDown is dev-loop only, like the other down paths.
func fieldServiceDown(app core.App) error {
	if entries, err := app.FindCollectionByNameOrId("time_entries"); err == nil {
		entries.Fields.RemoveByName("visit")
		if err := app.Save(entries); err != nil {
			return fmt.Errorf("save time_entries: %w", err)
		}
	}
	if visits, err := app.FindCollectionByNameOrId("visits"); err == nil {
		visits.Fields.RemoveByName("duration_minutes")
		if err := app.Save(visits); err != nil {
			return fmt.Errorf("save visits: %w", err)
		}
	}
	return nil
}
