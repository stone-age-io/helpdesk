package migrations

import (
	"fmt"

	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"

	"github.com/stone-age-io/helpdesk/internal/authz"
)

// Ticket classification: the missing "what is this about" dimension. Adds an
// admin-managed `ticket_categories` collection + a nullable `tickets.category`
// relation, plus free-text `asset`/`location` on tickets.
//
// Category is a *relation to a managed collection*, not a select field,
// because it is staff/admin-managed from the SPA: admins add or retire
// categories without a code deploy, renames are safe (the label lives in one
// row, not denormalized onto every ticket), and each category carries
// metadata (active, sort_order, color). It matches the app's existing grain
// (customers/staff/users are all collections).
//
// asset/location are the pragmatic "item" tier — provenance metadata, NOT a
// CMDB/asset catalog: the helpdesk is decoupled from the control plane and
// deliberately keeps no sites collection. Machine intakes populate them from
// the NATS/webhook payload (internal/ingest, internal/inbound).
//
// Category is staff-classified: the requester create rule gains a
// `category:isset = false` guard so the portal can't set it. The rule string
// is kept in sync with 1800000000_init.go's tickets create rule.

func init() {
	m.Register(ticketClassificationUp, ticketClassificationDown)
}

// starterCategories seed the picker so it isn't empty on a fresh install;
// admins edit, retire, reorder, and extend them in the SPA. Colors are hex
// (rendered as a soft badge, theme-neutral).
var starterCategories = []struct {
	name, key, color string
	order            int
}{
	{"Hardware", "hardware", "#f59e0b", 1},
	{"Network", "network", "#3b82f6", 2},
	{"Access control", "access-control", "#8b5cf6", 3},
	{"Kiosk", "kiosk", "#14b8a6", 4},
	{"IoT device", "iot-device", "#10b981", 5},
	{"Software", "software", "#6366f1", 6},
	{"Billing", "billing", "#ec4899", 7},
	{"Other", "other", "#6b7280", 8},
}

// ticketsCreateRule returns the tickets create rule. withCategoryGuard adds
// the `category:isset = false` clause to the requester branch (staff-
// classified). Shared by up (guarded) and down (original) so the two paths
// can't drift.
func ticketsCreateRule(withCategoryGuard bool) string {
	categoryGuard := ""
	if withCategoryGuard {
		categoryGuard = " && @request.body.category:isset = false"
	}
	return "(" + authz.StaffRule + ")" +
		" || (" + authz.RequesterRule +
		" && @request.body.customer = @request.auth.customer" +
		" && @request.body.requester = @request.auth.id" +
		" && @request.body.assignee:isset = false" +
		categoryGuard +
		" && @request.body.source = 'portal')"
}

func ticketClassificationUp(app core.App) error {
	categories, err := ensureCategoriesCollection(app)
	if err != nil {
		return err
	}

	tickets, err := app.FindCollectionByNameOrId("tickets")
	if err != nil {
		return fmt.Errorf("find tickets: %w", err)
	}
	if tickets.Fields.GetByName("category") == nil {
		tickets.Fields.Add(&core.RelationField{
			Name:         "category",
			CollectionId: categories.Id,
			MaxSelect:    1,
			// No cascade delete: retiring a category must never delete tickets.
			// The SPA deactivates rather than deletes; a deleted category just
			// leaves a dangling relation that renders blank.
		})
		tickets.AddIndex("idx_tickets_category", false, "category", "")
	}
	if tickets.Fields.GetByName("asset") == nil {
		tickets.Fields.Add(&core.TextField{Name: "asset", Max: 200})
	}
	if tickets.Fields.GetByName("location") == nil {
		tickets.Fields.Add(&core.TextField{Name: "location", Max: 200})
	}

	createRule := ticketsCreateRule(true)
	tickets.CreateRule = &createRule

	if err := app.Save(tickets); err != nil {
		return fmt.Errorf("save tickets: %w", err)
	}
	return nil
}

func ensureCategoriesCollection(app core.App) (*core.Collection, error) {
	if existing, err := app.FindCollectionByNameOrId("ticket_categories"); err == nil {
		return existing, nil // idempotent
	}

	col := core.NewBaseCollection("ticket_categories")
	col.Fields.Add(&core.TextField{Name: "name", Required: true})
	// key is the stable, human-readable filter/ingest handle (a slug). Renaming
	// `name` never touches it, so saved filters and machine payloads keep working.
	col.Fields.Add(&core.TextField{Name: "key", Required: true})
	col.Fields.Add(&core.BoolField{Name: "active"})
	col.Fields.Add(&core.NumberField{Name: "sort_order", OnlyInt: true})
	col.Fields.Add(&core.TextField{Name: "color", Max: 20})
	col.Fields.Add(&core.AutodateField{Name: "created", OnCreate: true})
	col.Fields.Add(&core.AutodateField{Name: "updated", OnCreate: true, OnUpdate: true})

	col.AddIndex("idx_ticket_categories_name", true, "name", "")
	col.AddIndex("idx_ticket_categories_key", true, "key", "")

	staffRule := authz.StaffRule
	adminRule := authz.AdminRule
	// Staff read the roster for the picker; only admins manage it.
	col.ListRule = &staffRule
	col.ViewRule = &staffRule
	col.CreateRule = &adminRule
	col.UpdateRule = &adminRule
	col.DeleteRule = &adminRule

	if err := app.Save(col); err != nil {
		return nil, fmt.Errorf("save ticket_categories: %w", err)
	}

	for _, c := range starterCategories {
		rec := core.NewRecord(col)
		rec.Set("name", c.name)
		rec.Set("key", c.key)
		rec.Set("active", true)
		rec.Set("sort_order", c.order)
		rec.Set("color", c.color)
		if err := app.Save(rec); err != nil {
			return nil, fmt.Errorf("seed category %q: %w", c.key, err)
		}
	}
	return col, nil
}

// ticketClassificationDown is dev-loop only, like the other down paths.
func ticketClassificationDown(app core.App) error {
	if tickets, err := app.FindCollectionByNameOrId("tickets"); err == nil {
		tickets.Fields.RemoveByName("category")
		tickets.Fields.RemoveByName("asset")
		tickets.Fields.RemoveByName("location")
		createRule := ticketsCreateRule(false)
		tickets.CreateRule = &createRule
		if err := app.Save(tickets); err != nil {
			return fmt.Errorf("save tickets: %w", err)
		}
	}
	if col, err := app.FindCollectionByNameOrId("ticket_categories"); err == nil {
		if err := app.Delete(col); err != nil {
			return fmt.Errorf("delete ticket_categories: %w", err)
		}
	}
	return nil
}
