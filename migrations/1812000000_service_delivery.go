package migrations

import (
	"fmt"

	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"

	"github.com/stone-age-io/helpdesk/internal/authz"
)

// Service-delivery expansion (light path): helpdesk grows from reactive
// ticketing into proactive project / installation / field work by adding a
// planning-and-grouping layer *above* the existing ticket → visit → time
// ledger. Nothing about the execution ledger changes — visits and time stay
// parented to tickets — so this is purely additive to the way work is done.
// See docs/service-delivery-plan.md.
//
//   locations — the customer's physical places. `code` is the join key to the
//     platform's Location concept: machine intakes carry it in the payload and
//     it resolves per (customer, code), which is what makes location a
//     queryable reporting dimension (tickets/installs/visits/time by location).
//     Deliberately NOT a CMDB — a place with an address and access notes, not
//     an asset catalog. The recurrence of a project revisiting the same site
//     over weeks is what earns this a relation where a one-off ticket visit's
//     free-text location did not (1803000000).
//
//   projects — a durable container grouping 1..N tickets at one location over
//     a target window. `lead` is whole-rollout accountability, distinct from
//     the per-ticket assignees (in a multi-trade project none of them is the
//     owner). Sequential `number` assigned by internal/projects, mirroring
//     tickets.
//
//   tickets — gain `project` (optional grouping), `type` (issue|install, the
//     reactive-vs-planned discriminator, defaulted by the ticket create hook),
//     and a `location` RELATION. The pre-existing free-text `location` is
//     renamed `location_note` and demoted to the unmatched-code fallback: when
//     intake can't resolve a code, the raw string lands there. The requester
//     (portal) create rule gains isset guards so project/type/location stay
//     staff-classified — same shape as the category guard in 1806000000.
//
//   users — gain `phone` (the requester's direct line; the on-site contact
//     lives on the location).

func init() {
	m.Register(serviceDeliveryUp, serviceDeliveryDown)
}

// serviceTicketsCreateRule supersedes 1806000000's ticketsCreateRule(true):
// the requester (portal) branch additionally blocks the staff-managed service
// fields — `project`, `type`, and the `location` relation — so a requester can
// still only file a plain portal ticket. Kept in sync with ticketsCreateRule.
// `location_note` is intentionally unguarded (harmless free text, matching the
// pre-rename behavior of the free-text location field).
func serviceTicketsCreateRule() string {
	return "(" + authz.StaffRule + ")" +
		" || (" + authz.RequesterRule +
		" && @request.body.customer = @request.auth.customer" +
		" && @request.body.requester = @request.auth.id" +
		" && @request.body.assignee:isset = false" +
		" && @request.body.category:isset = false" +
		" && @request.body.project:isset = false" +
		" && @request.body.type:isset = false" +
		" && @request.body.location:isset = false" +
		" && @request.body.source = 'portal')"
}

func serviceDeliveryUp(app core.App) error {
	customers, err := app.FindCollectionByNameOrId("customers")
	if err != nil {
		return fmt.Errorf("find customers: %w", err)
	}
	staff, err := app.FindCollectionByNameOrId("staff")
	if err != nil {
		return fmt.Errorf("find staff: %w", err)
	}

	locations, err := createLocationsCollection(app, customers)
	if err != nil {
		return err
	}
	projects, err := createProjectsCollection(app, customers, locations, staff)
	if err != nil {
		return err
	}
	if err := amendTicketsForService(app, projects, locations); err != nil {
		return err
	}
	return addRequesterPhone(app)
}

func createLocationsCollection(app core.App, customers *core.Collection) (*core.Collection, error) {
	if existing, err := app.FindCollectionByNameOrId("locations"); err == nil {
		return existing, nil // idempotent
	}

	loc := core.NewBaseCollection("locations")
	loc.Fields.Add(&core.RelationField{
		Name:         "customer",
		CollectionId: customers.Id,
		Required:     true,
		MaxSelect:    1,
	})
	// The platform Location join key. Optional (a hand-entered site for a
	// non-platform customer needs none); resolved per (customer, code).
	loc.Fields.Add(&core.TextField{Name: "code", Max: 100})
	loc.Fields.Add(&core.TextField{Name: "name", Required: true, Max: 200})
	loc.Fields.Add(&core.TextField{Name: "address", Max: 500})
	loc.Fields.Add(&core.TextField{Name: "notes", Max: 2000})
	loc.Fields.Add(&core.TextField{Name: "contact", Max: 200})
	loc.Fields.Add(&core.TextField{Name: "contact_phone", Max: 50})
	loc.Fields.Add(&core.AutodateField{Name: "created", OnCreate: true})
	loc.Fields.Add(&core.AutodateField{Name: "updated", OnCreate: true, OnUpdate: true})

	loc.AddIndex("idx_locations_customer", false, "customer", "")
	// A code is unique within a customer; different customers may reuse codes.
	loc.AddIndex("idx_locations_code", true, "customer, code", "code != ''")

	staffRule := authz.StaffRule
	adminRule := authz.AdminRule
	// Same relation-hop shape as visits (1803000000): a requester reads their
	// own company's locations. Staff create (so inline quick-create works from
	// the project/ticket form); only admins curate the roster.
	portalRead := authz.StaffRule +
		" || (" + authz.RequesterRule + " && customer = @request.auth.customer)"
	loc.ListRule = &portalRead
	loc.ViewRule = &portalRead
	loc.CreateRule = &staffRule
	loc.UpdateRule = &adminRule
	loc.DeleteRule = &adminRule

	if err := app.Save(loc); err != nil {
		return nil, fmt.Errorf("save locations: %w", err)
	}
	return loc, nil
}

func createProjectsCollection(app core.App, customers, locations, staff *core.Collection) (*core.Collection, error) {
	if existing, err := app.FindCollectionByNameOrId("projects"); err == nil {
		return existing, nil // idempotent
	}

	proj := core.NewBaseCollection("projects")
	// Sequential project number, assigned by internal/projects; unique index
	// is the collision backstop.
	proj.Fields.Add(&core.NumberField{Name: "number", OnlyInt: true})
	proj.Fields.Add(&core.RelationField{
		Name:         "customer",
		CollectionId: customers.Id,
		Required:     true,
		MaxSelect:    1,
	})
	proj.Fields.Add(&core.RelationField{
		Name:         "location",
		CollectionId: locations.Id,
		MaxSelect:    1,
	})
	proj.Fields.Add(&core.TextField{Name: "title", Required: true, Max: 300})
	proj.Fields.Add(&core.TextField{Name: "description", Max: 10000})
	proj.Fields.Add(&core.SelectField{
		Name:      "status",
		Values:    []string{"planned", "active", "completed", "canceled"},
		MaxSelect: 1,
	})
	proj.Fields.Add(&core.DateField{Name: "start_date"})
	proj.Fields.Add(&core.DateField{Name: "target_date"})
	// Whole-rollout accountability — not captured by per-ticket assignees.
	proj.Fields.Add(&core.RelationField{
		Name:         "lead",
		CollectionId: staff.Id,
		MaxSelect:    1,
	})
	proj.Fields.Add(&core.AutodateField{Name: "created", OnCreate: true})
	proj.Fields.Add(&core.AutodateField{Name: "updated", OnCreate: true, OnUpdate: true})

	proj.AddIndex("idx_projects_number", true, "number", "")
	proj.AddIndex("idx_projects_customer", false, "customer", "")
	proj.AddIndex("idx_projects_location", false, "location", "")
	proj.AddIndex("idx_projects_status", false, "status", "")

	staffRule := authz.StaffRule
	adminRule := authz.AdminRule
	portalRead := authz.StaffRule +
		" || (" + authz.RequesterRule + " && customer = @request.auth.customer)"
	proj.ListRule = &portalRead
	proj.ViewRule = &portalRead
	proj.CreateRule = &staffRule
	proj.UpdateRule = &staffRule
	proj.DeleteRule = &adminRule

	if err := app.Save(proj); err != nil {
		return nil, fmt.Errorf("save projects: %w", err)
	}
	return proj, nil
}

func amendTicketsForService(app core.App, projects, locations *core.Collection) error {
	tickets, err := app.FindCollectionByNameOrId("tickets")
	if err != nil {
		return fmt.Errorf("find tickets: %w", err)
	}

	// Rename the free-text `location` (1806000000) → `location_note`. Mutating
	// the existing field's Name preserves its id (a rename, not drop+add). It
	// becomes the unmatched-code fallback for machine intake.
	if f := tickets.Fields.GetByName("location"); f != nil {
		if tf, ok := f.(*core.TextField); ok {
			tf.Name = "location_note"
		}
	}
	// The `location` relation takes the freed name — the reporting axis.
	if tickets.Fields.GetByName("location") == nil {
		tickets.Fields.Add(&core.RelationField{
			Name:         "location",
			CollectionId: locations.Id,
			MaxSelect:    1,
		})
		tickets.AddIndex("idx_tickets_location", false, "location", "")
	}
	// Optional grouping into a project.
	if tickets.Fields.GetByName("project") == nil {
		tickets.Fields.Add(&core.RelationField{
			Name:         "project",
			CollectionId: projects.Id,
			MaxSelect:    1,
		})
		tickets.AddIndex("idx_tickets_project", false, "project", "")
	}
	// Reactive issue vs. planned install; default set by the ticket create hook.
	if tickets.Fields.GetByName("type") == nil {
		tickets.Fields.Add(&core.SelectField{
			Name:      "type",
			Values:    []string{"issue", "install"},
			MaxSelect: 1,
		})
		tickets.AddIndex("idx_tickets_type", false, "type", "")
	}

	createRule := serviceTicketsCreateRule()
	tickets.CreateRule = &createRule

	if err := app.Save(tickets); err != nil {
		return fmt.Errorf("save tickets: %w", err)
	}
	return nil
}

func addRequesterPhone(app core.App) error {
	users, err := app.FindCollectionByNameOrId("users")
	if err != nil {
		return fmt.Errorf("find users: %w", err)
	}
	if users.Fields.GetByName("phone") == nil {
		users.Fields.Add(&core.TextField{Name: "phone", Max: 50})
		if err := app.Save(users); err != nil {
			return fmt.Errorf("save users: %w", err)
		}
	}
	return nil
}

// serviceDeliveryDown is dev-loop only, like the other down paths.
func serviceDeliveryDown(app core.App) error {
	if tickets, err := app.FindCollectionByNameOrId("tickets"); err == nil {
		tickets.Fields.RemoveByName("project")
		tickets.Fields.RemoveByName("location") // the relation
		tickets.Fields.RemoveByName("type")
		// Restore the free-text location name.
		if f := tickets.Fields.GetByName("location_note"); f != nil {
			if tf, ok := f.(*core.TextField); ok {
				tf.Name = "location"
			}
		}
		createRule := ticketsCreateRule(true) // 1806000000's rule (category guard only)
		tickets.CreateRule = &createRule
		if err := app.Save(tickets); err != nil {
			return fmt.Errorf("save tickets: %w", err)
		}
	}
	// Projects references locations, so drop it first.
	for _, name := range []string{"projects", "locations"} {
		if c, err := app.FindCollectionByNameOrId(name); err == nil {
			if err := app.Delete(c); err != nil {
				return fmt.Errorf("delete %s: %w", name, err)
			}
		}
	}
	if users, err := app.FindCollectionByNameOrId("users"); err == nil {
		users.Fields.RemoveByName("phone")
		if err := app.Save(users); err != nil {
			return fmt.Errorf("save users: %w", err)
		}
	}
	return nil
}
