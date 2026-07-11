package migrations

import (
	"fmt"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"

	"github.com/stone-age-io/helpdesk/internal/authz"
	"github.com/stone-age-io/helpdesk/internal/notifications"
)

// Visits grow a lite dispatch workflow: an agent can promote a ticket to
// on-site work before anyone knows which technician goes or when. A
// `requested` status models that gap, so `scheduled_at` and `assignee`
// become optional — the internal/visits guard hook enforces that a
// *scheduled* visit still has both. A free-text `location` field carries
// dispatch directions ("Main St branch, rear entrance"); it deliberately
// replaces nothing structured — no sites collection until multi-location
// pain is real.
//
// Requesters gain read access to visits on their own company's tickets
// (someone has to unlock the door for the tech); writes stay staff-only.
//
// The same pass seeds the two new notification templates
// (visit.rescheduled, visit.canceled) — the seeding loop is idempotent and
// skips rows that already exist.

func init() {
	m.Register(visitsDispatchUp, visitsDispatchDown)
}

func visitsDispatchUp(app core.App) error {
	visits, err := app.FindCollectionByNameOrId("visits")
	if err != nil {
		return fmt.Errorf("find visits: %w", err)
	}

	status, ok := visits.Fields.GetByName("status").(*core.SelectField)
	if !ok {
		return fmt.Errorf("visits.status is not a select field")
	}
	status.Values = []string{"requested", "scheduled", "completed", "canceled"}

	assignee, ok := visits.Fields.GetByName("assignee").(*core.RelationField)
	if !ok {
		return fmt.Errorf("visits.assignee is not a relation field")
	}
	assignee.Required = false

	scheduledAt, ok := visits.Fields.GetByName("scheduled_at").(*core.DateField)
	if !ok {
		return fmt.Errorf("visits.scheduled_at is not a date field")
	}
	scheduledAt.Required = false

	if visits.Fields.GetByName("location") == nil {
		visits.Fields.Add(&core.TextField{Name: "location", Max: 500})
	}

	// Same relation-hop shape as ticket_comments: a requester reads visits
	// on their own company's tickets. Writes remain staff-only.
	portalRead := authz.StaffRule +
		" || (" + authz.RequesterRule + " && ticket.customer = @request.auth.customer)"
	visits.ListRule = &portalRead
	visits.ViewRule = &portalRead

	if err := app.Save(visits); err != nil {
		return fmt.Errorf("save visits: %w", err)
	}

	templates, err := app.FindCollectionByNameOrId(notifications.CollectionName)
	if err != nil {
		return fmt.Errorf("find %s: %w", notifications.CollectionName, err)
	}
	return seedNotificationTemplates(app, templates)
}

// visitsDispatchDown is dev-loop only, like the other down paths.
func visitsDispatchDown(app core.App) error {
	visits, err := app.FindCollectionByNameOrId("visits")
	if err != nil {
		return nil
	}

	if status, ok := visits.Fields.GetByName("status").(*core.SelectField); ok {
		status.Values = []string{"scheduled", "completed", "canceled"}
	}
	if assignee, ok := visits.Fields.GetByName("assignee").(*core.RelationField); ok {
		assignee.Required = true
	}
	if scheduledAt, ok := visits.Fields.GetByName("scheduled_at").(*core.DateField); ok {
		scheduledAt.Required = true
	}
	visits.Fields.RemoveByName("location")

	staffRule := authz.StaffRule
	visits.ListRule = &staffRule
	visits.ViewRule = &staffRule

	if err := app.Save(visits); err != nil {
		return fmt.Errorf("save visits: %w", err)
	}

	for _, et := range []string{notifications.EventTypeVisitRescheduled, notifications.EventTypeVisitCanceled} {
		rec, err := app.FindFirstRecordByFilter(
			notifications.CollectionName, "event_type = {:t}", dbx.Params{"t": et})
		if err != nil || rec == nil {
			continue
		}
		if err := app.Delete(rec); err != nil {
			return fmt.Errorf("delete template %s: %w", et, err)
		}
	}
	return nil
}
