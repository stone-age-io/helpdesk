// Package migrations holds schema-as-code definitions for the helpdesk's
// collections, registered via init() side effects and applied automatically
// on app start (migratecmd Automigrate), following the kiosk/access-control
// sibling-app pattern.
//
// The initial migration creates the two identity classes (`staff` fresh; the
// default `users` collection is repurposed in place as requesters), the
// `customers` company directory, and the ticketing core (`tickets`,
// `ticket_comments`, `time_entries`, `visits`). One bootstrap staff admin is
// seeded, its password printed to stdout exactly once.
package migrations

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"os"

	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"

	"github.com/stone-age-io/helpdesk/internal/authz"
)

const (
	bootstrapEmail  = "admin@helpdesk.local"
	bootstrapName   = "Bootstrap Admin"
	passwordEntropy = 12 // bytes; ~96 bits → 16-char URL-safe base64
)

func init() {
	m.Register(up, down)
}

func up(app core.App) error {
	customers, err := createCustomersCollection(app)
	if err != nil {
		return err
	}
	staff, err := createStaffCollection(app)
	if err != nil {
		return err
	}
	users, err := modifyUsersCollection(app, customers)
	if err != nil {
		return err
	}
	tickets, err := createTicketsCollection(app, customers, staff, users)
	if err != nil {
		return err
	}
	if err := createTicketCommentsCollection(app, tickets, staff, users); err != nil {
		return err
	}
	if err := createTimeEntriesCollection(app, tickets, staff); err != nil {
		return err
	}
	if err := createVisitsCollection(app, tickets, staff); err != nil {
		return err
	}
	return seedBootstrapAdmin(app, staff)
}

func down(app core.App) error {
	// Reverse order to respect FK constraints. Skip silently if not found.
	// The default users collection is left in place (reverting structural
	// changes to a system collection is risky; the down path is dev-loop only).
	for _, name := range []string{
		"visits",
		"time_entries",
		"ticket_comments",
		"tickets",
		"staff",
		"customers",
	} {
		c, err := app.FindCollectionByNameOrId(name)
		if err != nil {
			continue
		}
		if err := app.Delete(c); err != nil {
			return fmt.Errorf("delete %s: %w", name, err)
		}
	}
	return nil
}

func createCustomersCollection(app core.App) (*core.Collection, error) {
	customers := core.NewBaseCollection("customers")
	customers.Fields.Add(&core.TextField{Name: "name", Required: true})
	customers.Fields.Add(&core.BoolField{Name: "active"})
	// Maps this customer to a platform organization for NATS-ingested tickets:
	// the org id token in the hub-side subject helpdesk.{platform_org_id}.>.
	customers.Fields.Add(&core.TextField{Name: "platform_org_id"})
	// Shared secret for the inbound webhook route (/api/helpdesk/inbound/{token}).
	// Hidden: never leaves the server via the record API; staff use the reveal route.
	customers.Fields.Add(&core.TextField{Name: "webhook_token", Hidden: true})
	customers.Fields.Add(&core.TextField{Name: "notes", Max: 2000})
	customers.Fields.Add(&core.AutodateField{Name: "created", OnCreate: true})
	customers.Fields.Add(&core.AutodateField{Name: "updated", OnCreate: true, OnUpdate: true})

	customers.AddIndex("idx_customers_name", true, "name", "")
	customers.AddIndex("idx_customers_platform_org", true, "platform_org_id", "platform_org_id != ''")
	customers.AddIndex("idx_customers_webhook_token", true, "webhook_token", "webhook_token != ''")

	staffRule := authz.StaffRule
	adminRule := authz.AdminRule
	customers.ListRule = &staffRule
	customers.ViewRule = &staffRule
	customers.CreateRule = &adminRule
	customers.UpdateRule = &adminRule
	customers.DeleteRule = &adminRule

	if err := app.Save(customers); err != nil {
		return nil, fmt.Errorf("save customers: %w", err)
	}
	return customers, nil
}

func createStaffCollection(app core.App) (*core.Collection, error) {
	staff := core.NewAuthCollection("staff")
	staff.Fields.Add(&core.TextField{Name: "name", Required: true})
	staff.Fields.Add(&core.SelectField{
		Name:      "role",
		Values:    []string{"agent", "admin"},
		Required:  true,
		MaxSelect: 1,
	})
	staff.Fields.Add(&core.BoolField{Name: "active"})

	// Any staff member can see the roster (needed for assignee pickers);
	// only admins manage accounts. Self-update allowed for profile fields —
	// role escalation is blocked by the body guard.
	staffRule := authz.StaffRule
	adminRule := authz.AdminRule
	selfOrAdmin := authz.AdminRule + " || (id = @request.auth.id && @request.body.role:isset = false && @request.body.active:isset = false)"
	staff.ListRule = &staffRule
	staff.ViewRule = &staffRule
	staff.CreateRule = &adminRule
	staff.UpdateRule = &selfOrAdmin
	staff.DeleteRule = &adminRule

	authRule := "active = true"
	staff.AuthRule = &authRule

	if err := app.Save(staff); err != nil {
		return nil, fmt.Errorf("save staff: %w", err)
	}
	return staff, nil
}

// modifyUsersCollection repurposes the default PocketBase users collection as
// the requester directory: end-customer contacts who log into the portal.
func modifyUsersCollection(app core.App, customers *core.Collection) (*core.Collection, error) {
	users, err := app.FindCollectionByNameOrId("users")
	if err != nil {
		return nil, fmt.Errorf("find users: %w", err)
	}
	users.Fields.Add(&core.RelationField{
		Name:         "customer",
		CollectionId: customers.Id,
		Required:     true,
		MaxSelect:    1,
	})
	users.Fields.Add(&core.BoolField{Name: "active"})

	users.AddIndex("idx_users_customer", false, "customer", "")

	// Staff see all requesters; a requester sees only themselves. Only admins
	// create/delete accounts; requesters may update their own profile but not
	// reassign themselves to another customer or toggle active.
	listRule := authz.StaffRule + " || id = @request.auth.id"
	adminRule := authz.AdminRule
	selfUpdate := authz.AdminRule + " || (id = @request.auth.id && @request.body.customer:isset = false && @request.body.active:isset = false)"
	users.ListRule = &listRule
	users.ViewRule = &listRule
	users.CreateRule = &adminRule
	users.UpdateRule = &selfUpdate
	users.DeleteRule = &adminRule

	authRule := "active = true && customer != ''"
	users.AuthRule = &authRule

	if err := app.Save(users); err != nil {
		return nil, fmt.Errorf("save users: %w", err)
	}
	return users, nil
}

func createTicketsCollection(app core.App, customers, staff, users *core.Collection) (*core.Collection, error) {
	tickets := core.NewBaseCollection("tickets")
	// Human-facing sequential ticket number, assigned by the create hook
	// (internal/tickets); unique index is the collision backstop.
	tickets.Fields.Add(&core.NumberField{Name: "number", OnlyInt: true})
	tickets.Fields.Add(&core.RelationField{
		Name:         "customer",
		CollectionId: customers.Id,
		Required:     true,
		MaxSelect:    1,
	})
	tickets.Fields.Add(&core.TextField{Name: "title", Required: true, Max: 300})
	tickets.Fields.Add(&core.TextField{Name: "body", Max: 10000})
	tickets.Fields.Add(&core.SelectField{
		Name:      "status",
		Values:    []string{"open", "in_progress", "waiting", "resolved", "closed"},
		MaxSelect: 1,
	})
	tickets.Fields.Add(&core.SelectField{
		Name:      "priority",
		Values:    []string{"low", "normal", "high", "urgent"},
		MaxSelect: 1,
	})
	tickets.Fields.Add(&core.RelationField{
		Name:         "assignee",
		CollectionId: staff.Id,
		MaxSelect:    1,
	})
	// Optional: machine-generated tickets have no requester.
	tickets.Fields.Add(&core.RelationField{
		Name:         "requester",
		CollectionId: users.Id,
		MaxSelect:    1,
	})
	tickets.Fields.Add(&core.SelectField{
		Name:      "source",
		Values:    []string{"portal", "agent", "nats", "webhook"},
		MaxSelect: 1,
	})
	// Full hub-side NATS subject for machine tickets (provenance record).
	tickets.Fields.Add(&core.TextField{Name: "origin_subject"})
	// Ingestion idempotency key; unique when present.
	tickets.Fields.Add(&core.TextField{Name: "dedupe_key"})
	tickets.Fields.Add(&core.AutodateField{Name: "created", OnCreate: true})
	tickets.Fields.Add(&core.AutodateField{Name: "updated", OnCreate: true, OnUpdate: true})

	tickets.AddIndex("idx_tickets_number", true, "number", "")
	tickets.AddIndex("idx_tickets_customer", false, "customer", "")
	tickets.AddIndex("idx_tickets_status", false, "status", "")
	tickets.AddIndex("idx_tickets_assignee", false, "assignee", "")
	tickets.AddIndex("idx_tickets_dedupe", true, "dedupe_key", "dedupe_key != ''")

	// Requesters see and open tickets for their own company; staff work all of
	// them. Ticket field changes (status, assignee, ...) are staff actions —
	// requesters interact through comments.
	listRule := authz.StaffRule +
		" || (" + authz.RequesterRule + " && customer = @request.auth.customer)"
	createRule := "(" + authz.StaffRule + ")" +
		" || (" + authz.RequesterRule +
		" && @request.body.customer = @request.auth.customer" +
		" && @request.body.requester = @request.auth.id" +
		" && @request.body.assignee:isset = false" +
		" && @request.body.source = 'portal')"
	staffRule := authz.StaffRule
	adminRule := authz.AdminRule
	tickets.ListRule = &listRule
	tickets.ViewRule = &listRule
	tickets.CreateRule = &createRule
	tickets.UpdateRule = &staffRule
	tickets.DeleteRule = &adminRule

	if err := app.Save(tickets); err != nil {
		return nil, fmt.Errorf("save tickets: %w", err)
	}
	return tickets, nil
}

func createTicketCommentsCollection(app core.App, tickets, staff, users *core.Collection) error {
	comments := core.NewBaseCollection("ticket_comments")
	comments.Fields.Add(&core.RelationField{
		Name:          "ticket",
		CollectionId:  tickets.Id,
		Required:      true,
		MaxSelect:     1,
		CascadeDelete: true,
	})
	// Exactly one author field is set, matching the author's identity class.
	comments.Fields.Add(&core.RelationField{
		Name:         "author_staff",
		CollectionId: staff.Id,
		MaxSelect:    1,
	})
	comments.Fields.Add(&core.RelationField{
		Name:         "author_user",
		CollectionId: users.Id,
		MaxSelect:    1,
	})
	comments.Fields.Add(&core.TextField{Name: "body", Required: true, Max: 10000})
	// Internal notes are staff-only working notes, invisible to requesters.
	comments.Fields.Add(&core.BoolField{Name: "internal"})
	comments.Fields.Add(&core.AutodateField{Name: "created", OnCreate: true})

	comments.AddIndex("idx_ticket_comments_ticket", false, "ticket", "")

	listRule := authz.StaffRule +
		" || (" + authz.RequesterRule + " && ticket.customer = @request.auth.customer && internal = false)"
	createRule := "(" + authz.StaffRule + " && @request.body.author_staff = @request.auth.id && @request.body.author_user:isset = false)" +
		" || (" + authz.RequesterRule +
		" && @request.body.ticket.customer = @request.auth.customer" +
		" && @request.body.author_user = @request.auth.id" +
		" && @request.body.author_staff:isset = false" +
		" && @request.body.internal:isset = false)"
	adminRule := authz.AdminRule
	comments.ListRule = &listRule
	comments.ViewRule = &listRule
	comments.CreateRule = &createRule
	comments.UpdateRule = &adminRule
	comments.DeleteRule = &adminRule

	if err := app.Save(comments); err != nil {
		return fmt.Errorf("save ticket_comments: %w", err)
	}
	return nil
}

func createTimeEntriesCollection(app core.App, tickets, staff *core.Collection) error {
	entries := core.NewBaseCollection("time_entries")
	entries.Fields.Add(&core.RelationField{
		Name:          "ticket",
		CollectionId:  tickets.Id,
		Required:      true,
		MaxSelect:     1,
		CascadeDelete: true,
	})
	entries.Fields.Add(&core.RelationField{
		Name:         "staff",
		CollectionId: staff.Id,
		Required:     true,
		MaxSelect:    1,
	})
	min := 1.0
	entries.Fields.Add(&core.NumberField{Name: "minutes", Required: true, OnlyInt: true, Min: &min})
	entries.Fields.Add(&core.DateField{Name: "work_date", Required: true})
	entries.Fields.Add(&core.TextField{Name: "note", Max: 1000})
	entries.Fields.Add(&core.AutodateField{Name: "created", OnCreate: true})

	entries.AddIndex("idx_time_entries_ticket", false, "ticket", "")
	entries.AddIndex("idx_time_entries_staff", false, "[staff]", "")

	// Staff-only. Own entries editable; admins can correct anyone's.
	staffRule := authz.StaffRule
	createRule := authz.StaffRule + " && @request.body.staff = @request.auth.id"
	ownOrAdmin := authz.AdminRule + " || (staff = @request.auth.id && " + authz.StaffRule + ")"
	entries.ListRule = &staffRule
	entries.ViewRule = &staffRule
	entries.CreateRule = &createRule
	entries.UpdateRule = &ownOrAdmin
	entries.DeleteRule = &ownOrAdmin

	if err := app.Save(entries); err != nil {
		return fmt.Errorf("save time_entries: %w", err)
	}
	return nil
}

func createVisitsCollection(app core.App, tickets, staff *core.Collection) error {
	visits := core.NewBaseCollection("visits")
	visits.Fields.Add(&core.RelationField{
		Name:          "ticket",
		CollectionId:  tickets.Id,
		Required:      true,
		MaxSelect:     1,
		CascadeDelete: true,
	})
	visits.Fields.Add(&core.RelationField{
		Name:         "assignee",
		CollectionId: staff.Id,
		Required:     true,
		MaxSelect:    1,
	})
	visits.Fields.Add(&core.DateField{Name: "scheduled_at", Required: true})
	visits.Fields.Add(&core.SelectField{
		Name:      "status",
		Values:    []string{"scheduled", "completed", "canceled"},
		MaxSelect: 1,
	})
	visits.Fields.Add(&core.TextField{Name: "notes", Max: 2000})
	visits.Fields.Add(&core.AutodateField{Name: "created", OnCreate: true})
	visits.Fields.Add(&core.AutodateField{Name: "updated", OnCreate: true, OnUpdate: true})

	visits.AddIndex("idx_visits_ticket", false, "ticket", "")
	visits.AddIndex("idx_visits_assignee", false, "assignee", "")
	visits.AddIndex("idx_visits_scheduled_at", false, "scheduled_at", "")

	staffRule := authz.StaffRule
	visits.ListRule = &staffRule
	visits.ViewRule = &staffRule
	visits.CreateRule = &staffRule
	visits.UpdateRule = &staffRule
	visits.DeleteRule = &staffRule

	if err := app.Save(visits); err != nil {
		return fmt.Errorf("save visits: %w", err)
	}
	return nil
}

// seedBootstrapAdmin creates the first staff admin so the SPA is reachable on
// a fresh install. The generated password prints to stdout exactly once.
func seedBootstrapAdmin(app core.App, staff *core.Collection) error {
	existing, _ := app.FindAuthRecordByEmail(staff, bootstrapEmail)
	if existing != nil {
		return nil
	}

	raw := make([]byte, passwordEntropy)
	if _, err := rand.Read(raw); err != nil {
		return fmt.Errorf("generate bootstrap password: %w", err)
	}
	password := base64.RawURLEncoding.EncodeToString(raw)

	rec := core.NewRecord(staff)
	rec.Set("email", bootstrapEmail)
	rec.Set("name", bootstrapName)
	rec.Set("role", "admin")
	rec.Set("active", true)
	rec.SetPassword(password)
	if err := app.Save(rec); err != nil {
		return fmt.Errorf("seed bootstrap admin: %w", err)
	}

	fmt.Fprintf(os.Stdout, "\n=== helpdesk bootstrap admin ===\nemail:    %s\npassword: %s\n(shown once — change it after first login)\n\n", bootstrapEmail, password)
	return nil
}
