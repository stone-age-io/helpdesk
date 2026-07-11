package migrations

import (
	"fmt"

	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"

	"github.com/stone-age-io/helpdesk/internal/authz"
)

// ticket_events is a lightweight audit trail: one row per workflow-field
// change (status, priority, assignee), stamped with who did it. The
// internal/activity hooks write it; the staff ticket view renders it as a
// timeline. Values are stored already human-readable (assignee resolved to a
// name), so the timeline needs no expands beyond the actor.
//
// Staff-only reads — the trail names technicians, so it never goes portal-
// side. Writes have no API rule (superuser-only); the hooks write server-side
// via app.Save, which bypasses collection rules, so the trail can't be forged
// or edited through the record API.

func init() {
	m.Register(ticketEventsUp, ticketEventsDown)
}

func ticketEventsUp(app core.App) error {
	if _, err := app.FindCollectionByNameOrId("ticket_events"); err == nil {
		return nil // idempotent
	}
	tickets, err := app.FindCollectionByNameOrId("tickets")
	if err != nil {
		return fmt.Errorf("find tickets: %w", err)
	}
	staff, err := app.FindCollectionByNameOrId("staff")
	if err != nil {
		return fmt.Errorf("find staff: %w", err)
	}
	users, err := app.FindCollectionByNameOrId("users")
	if err != nil {
		return fmt.Errorf("find users: %w", err)
	}

	events := core.NewBaseCollection("ticket_events")
	events.Fields.Add(&core.RelationField{
		Name:          "ticket",
		CollectionId:  tickets.Id,
		Required:      true,
		MaxSelect:     1,
		CascadeDelete: true,
	})
	events.Fields.Add(&core.TextField{Name: "field", Required: true})
	events.Fields.Add(&core.TextField{Name: "old_value"})
	events.Fields.Add(&core.TextField{Name: "new_value"})
	events.Fields.Add(&core.RelationField{Name: "actor_staff", CollectionId: staff.Id, MaxSelect: 1})
	events.Fields.Add(&core.RelationField{Name: "actor_user", CollectionId: users.Id, MaxSelect: 1})
	events.Fields.Add(&core.AutodateField{Name: "created", OnCreate: true})

	events.AddIndex("idx_ticket_events_ticket", false, "ticket", "")

	staffRule := authz.StaffRule
	events.ListRule = &staffRule
	events.ViewRule = &staffRule
	// Create/Update/Delete rules stay nil: only the server hooks write here.

	if err := app.Save(events); err != nil {
		return fmt.Errorf("save ticket_events: %w", err)
	}
	return nil
}

// ticketEventsDown is dev-loop only, like the other down paths.
func ticketEventsDown(app core.App) error {
	col, err := app.FindCollectionByNameOrId("ticket_events")
	if err != nil {
		return nil
	}
	return app.Delete(col)
}
