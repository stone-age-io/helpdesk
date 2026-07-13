package migrations_test

import (
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase/core"

	"github.com/stone-age-io/helpdesk/internal/testutil"
)

// These exercise the 1812000000 service-delivery migration: the two new
// collections, the ticket amendments (crucially the free-text location →
// relation swap with location_note as the fallback), the requester create
// guards, and the portal read rules.

func TestServiceDeliverySchema(t *testing.T) {
	app := testutil.SetupApp(t)

	loc, err := app.FindCollectionByNameOrId("locations")
	if err != nil {
		t.Fatalf("find locations: %v", err)
	}
	for _, f := range []string{"customer", "code", "name", "address", "notes", "contact", "contact_phone"} {
		if loc.Fields.GetByName(f) == nil {
			t.Errorf("locations.%s missing", f)
		}
	}

	proj, err := app.FindCollectionByNameOrId("projects")
	if err != nil {
		t.Fatalf("find projects: %v", err)
	}
	for _, f := range []string{"number", "customer", "location", "title", "description", "status", "start_date", "target_date", "lead"} {
		if proj.Fields.GetByName(f) == nil {
			t.Errorf("projects.%s missing", f)
		}
	}

	tickets, err := app.FindCollectionByNameOrId("tickets")
	if err != nil {
		t.Fatalf("find tickets: %v", err)
	}
	// The free-text location was renamed to location_note; the `location` name
	// now belongs to the relation (the reporting axis).
	if _, ok := tickets.Fields.GetByName("location").(*core.RelationField); !ok {
		t.Error("tickets.location should be a relation field")
	}
	if _, ok := tickets.Fields.GetByName("location_note").(*core.TextField); !ok {
		t.Error("tickets.location_note should be a text field (renamed from location)")
	}
	if tickets.Fields.GetByName("project") == nil {
		t.Error("tickets.project missing")
	}
	if _, ok := tickets.Fields.GetByName("type").(*core.SelectField); !ok {
		t.Error("tickets.type should be a select field")
	}

	// The requester (portal) create rule keeps the new service fields staff-only.
	if r := tickets.CreateRule; r == nil ||
		!strings.Contains(*r, "@request.body.project:isset = false") ||
		!strings.Contains(*r, "@request.body.type:isset = false") ||
		!strings.Contains(*r, "@request.body.location:isset = false") {
		t.Errorf("tickets create rule missing service-field guards: %v", tickets.CreateRule)
	}

	// Requesters read their own company's locations and projects.
	for _, c := range []*core.Collection{loc, proj} {
		if c.ListRule == nil || !strings.Contains(*c.ListRule, "customer = @request.auth.customer") {
			t.Errorf("%s list rule missing requester relation-hop: %v", c.Name, c.ListRule)
		}
	}

	users, err := app.FindCollectionByNameOrId("users")
	if err != nil {
		t.Fatalf("find users: %v", err)
	}
	if users.Fields.GetByName("phone") == nil {
		t.Error("users.phone missing")
	}
}

// A project groups an install ticket that also carries a location — the whole
// point of the layer. Verifies the relations actually wire up end to end.
func TestServiceDeliveryWiring(t *testing.T) {
	app := testutil.SetupApp(t)

	customer := seed(t, app, "customers", map[string]any{"name": "Acme", "active": true})
	location := seed(t, app, "locations", map[string]any{
		"customer": customer.Id, "code": "BLDG-C",
		"name": "Acme HQ - Bldg C", "address": "123 Main St",
	})
	project := seed(t, app, "projects", map[string]any{
		"customer": customer.Id, "location": location.Id,
		"title": "Security Rollout", "number": 1, "status": "active",
	})
	ticket := seed(t, app, "tickets", map[string]any{
		"customer": customer.Id, "title": "Install access control", "number": 1,
		"type": "install", "project": project.Id, "location": location.Id,
	})

	got, err := app.FindRecordById("tickets", ticket.Id)
	if err != nil {
		t.Fatalf("reload ticket: %v", err)
	}
	if got.GetString("project") != project.Id {
		t.Errorf("ticket.project not wired: got %q, want %q", got.GetString("project"), project.Id)
	}
	if got.GetString("location") != location.Id {
		t.Errorf("ticket.location not wired: got %q, want %q", got.GetString("location"), location.Id)
	}
	if got.GetString("type") != "install" {
		t.Errorf("ticket.type: got %q, want install", got.GetString("type"))
	}
}

// The location code is unique per customer (so intake resolves deterministically),
// but two different customers may reuse the same code.
func TestLocationCodeUniquePerCustomer(t *testing.T) {
	app := testutil.SetupApp(t)

	acme := seed(t, app, "customers", map[string]any{"name": "Acme", "active": true})
	globex := seed(t, app, "customers", map[string]any{"name": "Globex", "active": true})

	seed(t, app, "locations", map[string]any{"customer": acme.Id, "code": "HQ", "name": "Acme HQ"})

	// Same code, different customer — allowed.
	col, _ := app.FindCollectionByNameOrId("locations")
	rec := core.NewRecord(col)
	rec.Set("customer", globex.Id)
	rec.Set("code", "HQ")
	rec.Set("name", "Globex HQ")
	if err := app.Save(rec); err != nil {
		t.Fatalf("same code for a different customer should be allowed: %v", err)
	}

	// Same code, same customer — rejected by the partial unique index.
	dup := core.NewRecord(col)
	dup.Set("customer", acme.Id)
	dup.Set("code", "HQ")
	dup.Set("name", "Acme Annex")
	if err := app.Save(dup); err == nil {
		t.Error("duplicate (customer, code) should violate the unique index")
	}
}
