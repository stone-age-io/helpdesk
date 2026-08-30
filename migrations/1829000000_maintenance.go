package migrations

import (
	"fmt"

	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"

	"github.com/stone-age-io/helpdesk/internal/authz"
)

// Preventive maintenance: the recurrence layer, and the last CMMS pillar this
// app was missing.
//
// A maintenance_plan says "this device gets serviced every 90 days" and its
// ONLY output is an ordinary ticket. That is the same shape 1812000000 gave
// projects — a planning layer ABOVE the ticket → visit → time ledger, not
// beside it — and it is what keeps this cheap: visits, the time ledger, the
// audit trail, portal visibility, notifications and every report already work
// on tickets and need no changes at all. Drop this collection and the app still
// runs; only future generation stops.
//
// # The two anchors
//
// `anchor` picks which of two behaviours a plan follows, and each stays simple
// because each has exactly ONE writer of `next_due`:
//
//	schedule    the cron owns next_due, advancing it by interval_days at
//	            generation. "Quarterly" stays quarterly however late the tech
//	            actually went — the calendar is the contract.
//	completion  the cron PARKS the plan by clearing next_due, and the ticket
//	            hook in internal/maintenance sets it to resolved_at + interval
//	            when the generated ticket resolves. "Every 90 days after last
//	            service" — for filters, batteries, anything where the clock
//	            starts when the work is done.
//
// That split has a payoff worth stating: a parked plan has an empty next_due,
// and the cron's query requires `next_due != ''`, so a completion-anchored plan
// CANNOT stack up unfinished work. The skip-if-still-open guard therefore only
// ever runs on schedule-anchored plans, and neither path has to reason about
// the other.
//
// # paused, not active
//
// A Go bool's zero value is false, so an `active` field would make every
// hand-created and seeded plan arrive inactive and be silently skipped by an
// `active = true` filter. Store the exception, like things.retired (1824000000)
// and time_entries.non_billable (1820000000).
//
// # tickets.due_at is a due date, NOT an SLA clock
//
// SLA timers and escalation remain out of scope (see CLAUDE.md). due_at is a
// plain nullable date somebody agreed to: nothing measures it, nothing
// escalates off it, and nothing but a human writes it (or the generator, which
// copies the plan's next_due). It exists because preventive work is meaningless
// without one, in the same way 1815000000's estimated_minutes is an estimate
// and pointedly not visits.duration_minutes.
//
// See docs/data-model.md and internal/maintenance.

func init() {
	m.Register(maintenanceUp, maintenanceDown)
}

// maintenanceTicketsCreateRule supersedes 1825000000's
// portalSiteDeviceTicketsCreateRule by adding the `due_at` and
// `maintenance_plan` guards. Both are judgements about the work, so they join
// the :isset bans beside category/type/project/estimated_minutes rather than
// the tenant hops that location/thing use — a requester has no business setting
// a target date or attaching a ticket to a service schedule.
//
// This is the seventh verbatim restatement of this rule and nothing composes
// it; diff it against portalSiteDeviceTicketsCreateRule before editing either.
func maintenanceTicketsCreateRule() string {
	return "(" + authz.StaffRule + ")" +
		" || (" + authz.RequesterRule +
		" && @request.body.customer = @request.auth.customer" +
		" && @request.body.requester = @request.auth.id" +
		" && @request.body.assignee:isset = false" +
		" && @request.body.category:isset = false" +
		" && @request.body.project:isset = false" +
		" && @request.body.type:isset = false" +
		" && @request.body.estimated_minutes:isset = false" +
		" && @request.body.due_at:isset = false" +
		" && @request.body.maintenance_plan:isset = false" +
		portalTenantHop("location") +
		portalTenantHop("thing") +
		" && @request.body.source = 'portal')"
}

func maintenanceUp(app core.App) error {
	customers, err := app.FindCollectionByNameOrId("customers")
	if err != nil {
		return fmt.Errorf("find customers: %w", err)
	}
	staff, err := app.FindCollectionByNameOrId("staff")
	if err != nil {
		return fmt.Errorf("find staff: %w", err)
	}
	locations, err := app.FindCollectionByNameOrId("locations")
	if err != nil {
		return fmt.Errorf("find locations: %w", err)
	}
	things, err := app.FindCollectionByNameOrId("things")
	if err != nil {
		return fmt.Errorf("find things: %w", err)
	}
	projects, err := app.FindCollectionByNameOrId("projects")
	if err != nil {
		return fmt.Errorf("find projects: %w", err)
	}
	categories, err := app.FindCollectionByNameOrId("ticket_categories")
	if err != nil {
		return fmt.Errorf("find ticket_categories: %w", err)
	}

	plans, err := createMaintenancePlansCollection(app, customers, staff, locations, things, projects, categories)
	if err != nil {
		return err
	}
	return amendTicketsForMaintenance(app, plans)
}

func createMaintenancePlansCollection(
	app core.App,
	customers, staff, locations, things, projects, categories *core.Collection,
) (*core.Collection, error) {
	if existing, err := app.FindCollectionByNameOrId("maintenance_plans"); err == nil {
		return existing, nil // idempotent
	}

	plans := core.NewBaseCollection("maintenance_plans")
	plans.Fields.Add(&core.RelationField{
		Name:         "customer",
		CollectionId: customers.Id,
		Required:     true,
		MaxSelect:    1,
	})
	plans.Fields.Add(&core.TextField{Name: "title", Required: true, Max: 300})
	plans.Fields.Add(&core.TextField{Name: "body", Max: 10000})

	// What gets serviced, and where. No cascade delete on any relation here or
	// below: retiring a device or deleting a site must never delete the
	// schedule that services it. A dangling relation renders blank, the same
	// accepted behaviour as tickets.category (1806000000).
	plans.Fields.Add(&core.RelationField{
		Name:         "thing",
		CollectionId: things.Id,
		MaxSelect:    1,
	})
	plans.Fields.Add(&core.RelationField{
		Name:         "location",
		CollectionId: locations.Id,
		MaxSelect:    1,
	})
	plans.Fields.Add(&core.RelationField{
		Name:         "project",
		CollectionId: projects.Id,
		MaxSelect:    1,
	})

	// Stamped onto every generated ticket, so a plan is also the triage the
	// desk would otherwise redo each cycle.
	plans.Fields.Add(&core.RelationField{
		Name:         "category",
		CollectionId: categories.Id,
		MaxSelect:    1,
	})
	plans.Fields.Add(&core.RelationField{
		Name:         "assignee",
		CollectionId: staff.Id,
		MaxSelect:    1,
	})
	plans.Fields.Add(&core.SelectField{
		Name:      "priority",
		Values:    []string{"low", "normal", "high", "urgent"},
		MaxSelect: 1,
	})
	estMin := 1.0
	plans.Fields.Add(&core.NumberField{Name: "estimated_minutes", OnlyInt: true, Min: &estMin})

	// The schedule itself.
	intervalMin := 1.0
	plans.Fields.Add(&core.NumberField{
		Name:     "interval_days",
		Required: true,
		OnlyInt:  true,
		Min:      &intervalMin,
	})
	plans.Fields.Add(&core.SelectField{
		Name:      "anchor",
		Values:    []string{"schedule", "completion"},
		MaxSelect: 1,
	})
	// Generate this many days ahead of next_due, so the work is on the board
	// before it is late. 0 = generate on the day.
	leadMin := 0.0
	plans.Fields.Add(&core.NumberField{Name: "lead_time_days", OnlyInt: true, Min: &leadMin})
	// Empty means "parked": either the plan has never been scheduled, or it is
	// completion-anchored and waiting on its open ticket. Either way the cron
	// skips it — see the header.
	plans.Fields.Add(&core.DateField{Name: "next_due"})
	plans.Fields.Add(&core.BoolField{Name: "paused"})

	plans.Fields.Add(&core.AutodateField{Name: "created", OnCreate: true})
	plans.Fields.Add(&core.AutodateField{Name: "updated", OnCreate: true, OnUpdate: true})

	plans.AddIndex("idx_maintenance_plans_customer", false, "customer", "")
	plans.AddIndex("idx_maintenance_plans_due", false, "next_due", "")
	plans.AddIndex("idx_maintenance_plans_thing", false, "thing", "")

	// Any staff member creates and edits; delete stays admin. Same split
	// 1813000000 gave locations, for the same reason: the person who learns a
	// site needs quarterly service is usually the tech standing in it, while
	// deleting a schedule is a decision with a history behind it.
	//
	// Read is staff-only for now. The generated tickets already give requesters
	// everything that matters, and a plan carries `assignee` — the MSP roster
	// the portal visit and project views are careful never to expose.
	staffRule := authz.StaffRule
	adminRule := authz.AdminRule
	plans.ListRule = &staffRule
	plans.ViewRule = &staffRule
	plans.CreateRule = &staffRule
	plans.UpdateRule = &staffRule
	plans.DeleteRule = &adminRule

	if err := app.Save(plans); err != nil {
		return nil, fmt.Errorf("save maintenance_plans: %w", err)
	}
	return plans, nil
}

func amendTicketsForMaintenance(app core.App, plans *core.Collection) error {
	tickets, err := app.FindCollectionByNameOrId("tickets")
	if err != nil {
		return fmt.Errorf("find tickets: %w", err)
	}

	// The target date. Nullable, no backfill — existing tickets simply have
	// none, same as resolved_at in 1821000000.
	if tickets.Fields.GetByName("due_at") == nil {
		tickets.Fields.Add(&core.DateField{Name: "due_at"})
		tickets.AddIndex("idx_tickets_due_at", false, "due_at", "")
	}

	// Back-pointer to the plan that generated this ticket. It earns its place
	// three times over: it is the "is the last one still open" guard, it is how
	// the completion hook finds its way back to the plan, and it is the
	// generated-ticket history on the plan detail view.
	if tickets.Fields.GetByName("maintenance_plan") == nil {
		tickets.Fields.Add(&core.RelationField{
			Name:         "maintenance_plan",
			CollectionId: plans.Id,
			MaxSelect:    1,
		})
		tickets.AddIndex("idx_tickets_maintenance_plan", false, "maintenance_plan", "")
	}

	// `source` gains `maintenance`. None of portal/agent/nats/webhook/email is
	// an honest fit for a ticket the scheduler opened, and provenance is the
	// entire point of this field — the same reasoning that added `email` in
	// 1823000000.
	if f := tickets.Fields.GetByName("source"); f != nil {
		if sf, ok := f.(*core.SelectField); ok && !hasValue(sf.Values, "maintenance") {
			sf.Values = append(sf.Values, "maintenance")
		}
	}

	createRule := maintenanceTicketsCreateRule()
	tickets.CreateRule = &createRule

	if err := app.Save(tickets); err != nil {
		return fmt.Errorf("save tickets: %w", err)
	}
	return nil
}

// maintenanceDown is dev-loop only, like the other down paths. It restores
// 1825000000's rule — the one with the location/thing tenant hops. Restoring an
// earlier builder would silently re-close the portal's site and device pickers.
func maintenanceDown(app core.App) error {
	if tickets, err := app.FindCollectionByNameOrId("tickets"); err == nil {
		tickets.Fields.RemoveByName("due_at")
		tickets.Fields.RemoveByName("maintenance_plan")
		if f := tickets.Fields.GetByName("source"); f != nil {
			if sf, ok := f.(*core.SelectField); ok {
				var kept []string
				for _, v := range sf.Values {
					if v != "maintenance" {
						kept = append(kept, v)
					}
				}
				sf.Values = kept
			}
		}
		rule := portalSiteDeviceTicketsCreateRule()
		tickets.CreateRule = &rule
		if err := app.Save(tickets); err != nil {
			return fmt.Errorf("save tickets: %w", err)
		}
	}
	// Tickets reference plans, so drop the collection only after the relation
	// field is gone.
	if plans, err := app.FindCollectionByNameOrId("maintenance_plans"); err == nil {
		if err := app.Delete(plans); err != nil {
			return fmt.Errorf("delete maintenance_plans: %w", err)
		}
	}
	return nil
}
