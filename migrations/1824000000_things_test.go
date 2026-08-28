package migrations_test

import (
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase/core"

	"github.com/stone-age-io/helpdesk/internal/authz"
	"github.com/stone-age-io/helpdesk/internal/testutil"
)

// These exercise the 1824000000 things migration: the three new collections, the
// locations amendments, the tickets asset → thing/thing_note swap, and the
// create-rule supersession.

func TestThingsSchema(t *testing.T) {
	app := testutil.SetupApp(t)

	for _, name := range []string{"thing_types", "location_types"} {
		col, err := app.FindCollectionByNameOrId(name)
		if err != nil {
			t.Fatalf("find %s: %v", name, err)
		}
		for _, f := range []string{"customer", "code", "name", "description", "metadata_schema"} {
			if col.Fields.GetByName(f) == nil {
				t.Errorf("%s.%s missing", name, f)
			}
		}
		if _, ok := col.Fields.GetByName("metadata_schema").(*core.JSONField); !ok {
			t.Errorf("%s.metadata_schema should be a json field", name)
		}
		// Requesters read types so a portal ticket can render a typed metadata
		// field; only admins curate them.
		if col.ListRule == nil || !strings.Contains(*col.ListRule, "customer = @request.auth.customer") {
			t.Errorf("%s list rule should scope requesters to their customer", name)
		}
		if col.CreateRule == nil || *col.CreateRule != authz.AdminRule {
			t.Errorf("%s create rule should be admin-only", name)
		}
		if col.DeleteRule == nil || *col.DeleteRule != authz.AdminRule {
			t.Errorf("%s delete rule should be admin-only", name)
		}
	}

	things, err := app.FindCollectionByNameOrId("things")
	if err != nil {
		t.Fatalf("find things: %v", err)
	}
	for _, f := range []string{"customer", "code", "name", "type", "location", "notes", "retired", "metadata"} {
		if things.Fields.GetByName(f) == nil {
			t.Errorf("things.%s missing", f)
		}
	}
	// type is a RELATION, not free text — the metadata_schema on the type is what
	// keeps `metadata` from becoming a bag of drifting key spellings. Do not
	// "simplify" this to a string.
	if _, ok := things.Fields.GetByName("type").(*core.RelationField); !ok {
		t.Error("things.type should be a relation to thing_types")
	}
	if _, ok := things.Fields.GetByName("metadata").(*core.JSONField); !ok {
		t.Error("things.metadata should be a json field")
	}
	if rel, ok := things.Fields.GetByName("customer").(*core.RelationField); !ok || !rel.Required {
		t.Error("things.customer should be a required relation")
	}
	if things.CreateRule == nil || *things.CreateRule != authz.StaffRule {
		t.Error("things create rule should be StaffRule")
	}
	if things.DeleteRule == nil || *things.DeleteRule != authz.AdminRule {
		t.Error("things delete rule should be AdminRule")
	}
	if things.ListRule == nil || !strings.Contains(*things.ListRule, "customer = @request.auth.customer") {
		t.Error("things list rule should scope requesters to their customer")
	}

	locations, err := app.FindCollectionByNameOrId("locations")
	if err != nil {
		t.Fatalf("find locations: %v", err)
	}
	if _, ok := locations.Fields.GetByName("type").(*core.RelationField); !ok {
		t.Error("locations.type should be a relation to location_types")
	}
	if _, ok := locations.Fields.GetByName("metadata").(*core.JSONField); !ok {
		t.Error("locations.metadata should be a json field")
	}
	parent, ok := locations.Fields.GetByName("parent").(*core.RelationField)
	if !ok {
		t.Fatal("locations.parent should be a relation")
	}
	if parent.CollectionId != locations.Id {
		t.Error("locations.parent should be a self-relation")
	}
	if parent.CascadeDelete {
		t.Error("locations.parent must not cascade delete — deleting a site would take its children with it")
	}

	tickets, err := app.FindCollectionByNameOrId("tickets")
	if err != nil {
		t.Fatalf("find tickets: %v", err)
	}
	if tickets.Fields.GetByName("asset") != nil {
		t.Error("tickets.asset should be gone, replaced by thing/thing_note")
	}
	if _, ok := tickets.Fields.GetByName("thing").(*core.RelationField); !ok {
		t.Error("tickets.thing should be a relation field")
	}
	if _, ok := tickets.Fields.GetByName("thing_note").(*core.TextField); !ok {
		t.Error("tickets.thing_note should be a text field")
	}
}

// TestTicketsCreateRuleIntact pins the WHOLE requester branch, not just the
// clause this migration added. The rule has been restated verbatim by four
// migrations now (1800, 1806, 1812, 1815, and this one); nothing composes it, so
// a copy-paste that dropped `assignee:isset` or `source = 'portal'` would be a
// silent portal privilege escalation that no other test would catch.
func TestTicketsCreateRuleIntact(t *testing.T) {
	app := testutil.SetupApp(t)

	tickets, err := app.FindCollectionByNameOrId("tickets")
	if err != nil {
		t.Fatalf("find tickets: %v", err)
	}
	if tickets.CreateRule == nil {
		t.Fatal("tickets create rule is nil")
	}
	rule := *tickets.CreateRule

	for _, clause := range []string{
		authz.StaffRule,
		authz.RequesterRule,
		"@request.body.customer = @request.auth.customer",
		"@request.body.requester = @request.auth.id",
		"@request.body.assignee:isset = false",
		"@request.body.category:isset = false",
		"@request.body.project:isset = false",
		"@request.body.type:isset = false",
		"@request.body.estimated_minutes:isset = false",
		"@request.body.source = 'portal'",
		// `location` and `thing` were :isset-guarded here, but 1825000000 traded
		// both guards for tenant-scoped relation hops so a requester can name
		// their own site and device at intake. The hops are what keeps that from
		// being a cross-tenant write, and they are pinned — behaviourally, over
		// real HTTP — in 1825000000_portal_site_device_test.go.
		"@request.body.location.customer = @request.auth.customer",
		"@request.body.thing.customer = @request.auth.customer",
	} {
		if !strings.Contains(rule, clause) {
			t.Errorf("tickets create rule missing %q", clause)
		}
	}

	// The free-text fallbacks stay unguarded — harmless text, and guarding them
	// would break the portal's optional location/device hints.
	for _, clause := range []string{"thing_note:isset", "location_note:isset"} {
		if strings.Contains(rule, clause) {
			t.Errorf("tickets create rule should not guard %q", clause)
		}
	}
}

func TestThingWiring(t *testing.T) {
	app := testutil.SetupApp(t)

	customer := seed(t, app, "customers", map[string]any{"name": "Acme", "active": true})
	thingType := seed(t, app, "thing_types", map[string]any{
		"customer": customer.Id, "code": "door-controller", "name": "Door Controller",
		"metadata_schema": map[string]any{
			"type":       "object",
			"properties": map[string]any{"serial": map[string]any{"type": "string"}},
		},
	})
	locationType := seed(t, app, "location_types", map[string]any{
		"customer": customer.Id, "code": "building", "name": "Building",
	})
	site := seed(t, app, "locations", map[string]any{
		"customer": customer.Id, "name": "HQ", "code": "HQ", "type": locationType.Id,
	})
	floor := seed(t, app, "locations", map[string]any{
		"customer": customer.Id, "name": "Floor 2", "code": "HQ-F2", "parent": site.Id,
	})
	thing := seed(t, app, "things", map[string]any{
		"customer": customer.Id, "code": "RDR-01", "name": "North Door Reader",
		"type": thingType.Id, "location": floor.Id,
		"metadata": map[string]any{"serial": "SN-9931"},
	})
	ticket := seed(t, app, "tickets", map[string]any{
		"customer": customer.Id, "title": "reader offline",
		"thing": thing.Id, "location": floor.Id,
	})

	got, err := app.FindRecordById("tickets", ticket.Id)
	if err != nil {
		t.Fatalf("reload ticket: %v", err)
	}
	if got.GetString("thing") != thing.Id {
		t.Errorf("ticket.thing = %q, want %q", got.GetString("thing"), thing.Id)
	}

	gotThing, err := app.FindRecordById("things", thing.Id)
	if err != nil {
		t.Fatalf("reload thing: %v", err)
	}
	if gotThing.GetString("type") != thingType.Id {
		t.Errorf("thing.type = %q, want %q", gotThing.GetString("type"), thingType.Id)
	}
	if gotThing.GetString("location") != floor.Id {
		t.Errorf("thing.location = %q, want %q", gotThing.GetString("location"), floor.Id)
	}
	// retired defaults to false — a hand-created thing is in service. This is why
	// the field stores the exception rather than mirroring the platform's `active`.
	if gotThing.GetBool("retired") {
		t.Error("a newly created thing should not be retired")
	}

	gotFloor, err := app.FindRecordById("locations", floor.Id)
	if err != nil {
		t.Fatalf("reload location: %v", err)
	}
	if gotFloor.GetString("parent") != site.Id {
		t.Errorf("location.parent = %q, want %q", gotFloor.GetString("parent"), site.Id)
	}
}

func TestThingCodeUniquePerCustomer(t *testing.T) {
	app := testutil.SetupApp(t)

	acme := seed(t, app, "customers", map[string]any{"name": "Acme", "active": true})
	globex := seed(t, app, "customers", map[string]any{"name": "Globex", "active": true})

	for _, collection := range []string{"things", "thing_types"} {
		// The same code under two customers is correct and expected.
		seed(t, app, collection, map[string]any{"customer": acme.Id, "name": "A", "code": "RDR-01"})
		seed(t, app, collection, map[string]any{"customer": globex.Id, "name": "B", "code": "RDR-01"})

		col, err := app.FindCollectionByNameOrId(collection)
		if err != nil {
			t.Fatalf("find %s: %v", collection, err)
		}
		dup := core.NewRecord(col)
		dup.Set("customer", acme.Id)
		dup.Set("name", "C")
		dup.Set("code", "RDR-01")
		if err := app.Save(dup); err == nil {
			t.Errorf("%s: duplicate (customer, code) should be rejected", collection)
		}

		// Blank codes are exempt — SQLite treats '' as a value, not NULL, so
		// without the `code != ''` partial predicate the second of these would
		// collide. This is the only case that actually exercises it.
		seed(t, app, collection, map[string]any{"customer": acme.Id, "name": "No code 1"})
		seed(t, app, collection, map[string]any{"customer": acme.Id, "name": "No code 2"})
	}
}
