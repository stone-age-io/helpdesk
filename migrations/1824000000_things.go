package migrations

import (
	"fmt"

	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"

	"github.com/stone-age-io/helpdesk/internal/authz"
)

// Things: the device axis, and the last free-text field promoted to a relation.
//
// 1806000000 added free-text `tickets.asset` and framed it as "the pragmatic
// item tier — provenance metadata, NOT a CMDB". That framing is what this
// migration reverses, for the same reason 1812000000 reversed it for location:
// free text cannot answer "every ticket for this device" or "which devices burn
// the most hours". The shape follows locations field-for-field.
//
//   thing_types / location_types — subset mirrors of the platform's collections
//     of the same names. Everything that makes them big upstream (capabilities,
//     subject_prefix, operations, nats_role) is NATS contract and does not cross
//     the boundary; what remains is a code, a label, and `metadata_schema`.
//     That schema is the point: it says which keys a record of this type tracks,
//     which is what keeps `metadata` from degenerating into a bag of drifting
//     key spellings (serial / Serial / sn). Same reasoning 1806000000 gave for
//     making ticket_categories a managed collection instead of a select.
//
//   things — a subset mirror of the platform's `things`, minus the entire
//     identity half (password, tokenKey, email, nats_user, nebula_host are
//     control-plane and the helpdesk must never hold them). `code` is the join
//     key, resolved per (customer, code) exactly like locations. Deliberately a
//     SUPERSET of the platform's catalog, not a copy: `code` is optional because
//     MSP work covers printers, door strikes and customer switches that were
//     never onboarded to the control plane.
//
//     There is no live sync and there cannot be one — the platform publishes no
//     event stream for things, and the only read paths are a control-plane
//     credential or an edge KV mirror. This is a hand-curated local catalog that
//     JOINS upstream by code; an operator-run export→seed is the intended
//     bulk-load path, which is why the shape stays faithful.
//
//   locations — gain `type` (→ location_types), `parent` (self-relation), and
//     `metadata`, closing the remaining gaps with the platform's shape so an
//     export seeds without translation. `parent` is stored and displayed but
//     barely editable by hand: dispatch is site-level, and a ticket that names a
//     thing already carries the precision a room-level hierarchy would provide.
//
//   tickets — free-text `asset` is DROPPED and replaced by `thing_note` (text)
//     plus a `thing` relation. Unlike 1812000000's location rename this is a
//     drop-and-add: the app is pre-release, there is no data to preserve, and
//     the new names don't collide with the old one so the in-place rename trick
//     isn't needed. (That trick is load-bearing when it IS needed: adding a
//     relation under a name a text field still holds makes FieldsList.add reuse
//     the existing field id, after which syncRecordTableSchema matches by id,
//     sees TEXT DEFAULT '' NOT NULL on both sides, and emits no DDL at all —
//     the column silently keeps its free text while the field becomes a
//     relation.)
//
// See docs/data-model.md.

func init() {
	m.Register(thingsUp, thingsDown)
}

// thingsTicketsCreateRule supersedes 1815000000's estimatedEffortTicketsCreateRule
// by adding the `thing` guard to the requester branch. Kept in sync with it.
// `thing_note` is intentionally unguarded (harmless free text, matching
// location_note's treatment in 1812000000).
func thingsTicketsCreateRule() string {
	return "(" + authz.StaffRule + ")" +
		" || (" + authz.RequesterRule +
		" && @request.body.customer = @request.auth.customer" +
		" && @request.body.requester = @request.auth.id" +
		" && @request.body.assignee:isset = false" +
		" && @request.body.category:isset = false" +
		" && @request.body.project:isset = false" +
		" && @request.body.type:isset = false" +
		" && @request.body.location:isset = false" +
		" && @request.body.estimated_minutes:isset = false" +
		" && @request.body.thing:isset = false" +
		" && @request.body.source = 'portal')"
}

func thingsUp(app core.App) error {
	customers, err := app.FindCollectionByNameOrId("customers")
	if err != nil {
		return fmt.Errorf("find customers: %w", err)
	}
	locations, err := app.FindCollectionByNameOrId("locations")
	if err != nil {
		return fmt.Errorf("find locations: %w", err)
	}

	thingTypes, err := createTypeCollection(app, customers, "thing_types")
	if err != nil {
		return err
	}
	locationTypes, err := createTypeCollection(app, customers, "location_types")
	if err != nil {
		return err
	}
	things, err := createThingsCollection(app, customers, thingTypes, locations)
	if err != nil {
		return err
	}
	if err := amendLocationsForThings(app, locationTypes); err != nil {
		return err
	}
	return amendTicketsForThings(app, things)
}

// createTypeCollection builds thing_types and location_types, which are the same
// shape once the platform's NATS-contract fields are stripped. One helper rather
// than two near-identical bodies; they stay two COLLECTIONS because that is the
// platform's shape and it makes the seed a 1:1 map.
func createTypeCollection(app core.App, customers *core.Collection, name string) (*core.Collection, error) {
	if existing, err := app.FindCollectionByNameOrId(name); err == nil {
		return existing, nil // idempotent
	}

	col := core.NewBaseCollection(name)
	col.Fields.Add(&core.RelationField{
		Name:         "customer",
		CollectionId: customers.Id,
		Required:     true,
		MaxSelect:    1,
	})
	// The platform join key. Optional (a helpdesk-only type needs none);
	// resolved per (customer, code), same as locations and things.
	col.Fields.Add(&core.TextField{Name: "code", Max: 100})
	col.Fields.Add(&core.TextField{Name: "name", Required: true, Max: 200})
	col.Fields.Add(&core.TextField{Name: "description", Max: 2000})
	// JSON Schema describing what is tracked about each record of this type.
	// Null (not an empty object) when the type has no schema — that null is what
	// makes the record form fall back to free-form key/value rows.
	col.Fields.Add(&core.JSONField{Name: "metadata_schema"})
	col.Fields.Add(&core.AutodateField{Name: "created", OnCreate: true})
	col.Fields.Add(&core.AutodateField{Name: "updated", OnCreate: true, OnUpdate: true})

	col.AddIndex("idx_"+name+"_customer", false, "customer", "")
	col.AddIndex("idx_"+name+"_code", true, "customer, code", "code != ''")

	adminRule := authz.AdminRule
	// Requesters read types because a portal ticket may render a typed metadata
	// field, which needs the schema. Only admins curate the taxonomy — the same
	// split ticket_categories uses.
	portalRead := authz.StaffRule +
		" || (" + authz.RequesterRule + " && customer = @request.auth.customer)"
	col.ListRule = &portalRead
	col.ViewRule = &portalRead
	col.CreateRule = &adminRule
	col.UpdateRule = &adminRule
	col.DeleteRule = &adminRule

	if err := app.Save(col); err != nil {
		return nil, fmt.Errorf("save %s: %w", name, err)
	}
	return col, nil
}

func createThingsCollection(app core.App, customers, thingTypes, locations *core.Collection) (*core.Collection, error) {
	if existing, err := app.FindCollectionByNameOrId("things"); err == nil {
		return existing, nil // idempotent
	}

	col := core.NewBaseCollection("things")
	col.Fields.Add(&core.RelationField{
		Name:         "customer",
		CollectionId: customers.Id,
		Required:     true,
		MaxSelect:    1,
	})
	// The platform Thing join key. Optional: gear with no platform record is
	// exactly why this catalog is a superset rather than a copy.
	col.Fields.Add(&core.TextField{Name: "code", Max: 100})
	col.Fields.Add(&core.TextField{Name: "name", Required: true, Max: 200})
	// No cascade delete on either relation: retiring a type or deleting a
	// location must never delete inventory. A dangling relation renders blank,
	// the same accepted behaviour as tickets.category (1806000000).
	col.Fields.Add(&core.RelationField{
		Name:         "type",
		CollectionId: thingTypes.Id,
		MaxSelect:    1,
	})
	col.Fields.Add(&core.RelationField{
		Name:         "location",
		CollectionId: locations.Id,
		MaxSelect:    1,
	})
	col.Fields.Add(&core.TextField{Name: "notes", Max: 2000})
	// `retired`, not the platform's `active`: a Go bool's zero value is false, so
	// an `active` field would make every hand-created row arrive inactive and be
	// skipped by any future `active = true` filter. Storing the exception is the
	// time_entries.non_billable idiom (1820000000). Seeder maps retired = !active.
	col.Fields.Add(&core.BoolField{Name: "retired"})
	col.Fields.Add(&core.JSONField{Name: "metadata"})
	col.Fields.Add(&core.AutodateField{Name: "created", OnCreate: true})
	col.Fields.Add(&core.AutodateField{Name: "updated", OnCreate: true, OnUpdate: true})

	col.AddIndex("idx_things_customer", false, "customer", "")
	// A code is unique within a customer; different customers may reuse codes
	// (two tenants both calling a reader RDR-01 is correct and expected). The
	// partial predicate exempts blanks, which SQLite treats as a value, not NULL.
	col.AddIndex("idx_things_code", true, "customer, code", "code != ''")
	col.AddIndex("idx_things_type", false, "type", "")
	col.AddIndex("idx_things_location", false, "location", "")

	staffRule := authz.StaffRule
	adminRule := authz.AdminRule
	// Same shape locations landed on in 1813000000: any agent curates inventory
	// day-to-day; delete is the one destructive op (things are referenced by
	// tickets) and stays admin.
	portalRead := authz.StaffRule +
		" || (" + authz.RequesterRule + " && customer = @request.auth.customer)"
	col.ListRule = &portalRead
	col.ViewRule = &portalRead
	col.CreateRule = &staffRule
	col.UpdateRule = &staffRule
	col.DeleteRule = &adminRule

	if err := app.Save(col); err != nil {
		return nil, fmt.Errorf("save things: %w", err)
	}
	return col, nil
}

func amendLocationsForThings(app core.App, locationTypes *core.Collection) error {
	locations, err := app.FindCollectionByNameOrId("locations")
	if err != nil {
		return fmt.Errorf("find locations: %w", err)
	}

	if locations.Fields.GetByName("type") == nil {
		locations.Fields.Add(&core.RelationField{
			Name:         "type",
			CollectionId: locationTypes.Id,
			MaxSelect:    1,
		})
		locations.AddIndex("idx_locations_type", false, "type", "")
	}
	// Self-relation, no cascade delete: deleting a parent leaves children with a
	// dangling id that renders blank rather than removing the sites under it.
	// PocketBase has no cycle detection for self-relations, but expand recursion
	// is capped (maxNestedRels), so a hand-made cycle is a data-quality problem,
	// not an availability one. Any UI that walks this must carry a visited set
	// and a depth cap.
	if locations.Fields.GetByName("parent") == nil {
		locations.Fields.Add(&core.RelationField{
			Name:         "parent",
			CollectionId: locations.Id,
			MaxSelect:    1,
		})
		locations.AddIndex("idx_locations_parent", false, "parent", "")
	}
	if locations.Fields.GetByName("metadata") == nil {
		locations.Fields.Add(&core.JSONField{Name: "metadata"})
	}

	if err := app.Save(locations); err != nil {
		return fmt.Errorf("save locations: %w", err)
	}
	return nil
}

func amendTicketsForThings(app core.App, things *core.Collection) error {
	tickets, err := app.FindCollectionByNameOrId("tickets")
	if err != nil {
		return fmt.Errorf("find tickets: %w", err)
	}

	// Drop free-text `asset` (1806000000) and add the pair that replaces it.
	// Pre-release, so this is a drop-and-add rather than 1812000000's in-place
	// rename: there is no data to preserve, and `thing`/`thing_note` don't
	// collide with `asset`, so no field id is reused and SQLite gets real DDL.
	tickets.Fields.RemoveByName("asset")

	// The unmatched-code fallback for machine intake, and a scratch description
	// when the device isn't in the curated catalog.
	if tickets.Fields.GetByName("thing_note") == nil {
		tickets.Fields.Add(&core.TextField{Name: "thing_note", Max: 200})
	}
	// The reporting axis.
	if tickets.Fields.GetByName("thing") == nil {
		tickets.Fields.Add(&core.RelationField{
			Name:         "thing",
			CollectionId: things.Id,
			MaxSelect:    1,
		})
		tickets.AddIndex("idx_tickets_thing", false, "thing", "")
	}

	createRule := thingsTicketsCreateRule()
	tickets.CreateRule = &createRule

	if err := app.Save(tickets); err != nil {
		return fmt.Errorf("save tickets: %w", err)
	}
	return nil
}

// thingsDown is dev-loop only, like the other down paths. Order matters: the
// ticket relation must go before `things` can be deleted (PocketBase refuses to
// delete a collection other collections' relations point at), and locations'
// `type` must go before location_types.
func thingsDown(app core.App) error {
	if tickets, err := app.FindCollectionByNameOrId("tickets"); err == nil {
		// Removing a field does not prune its index — Indexes is an independent
		// raw-SQL list. Left behind, the recreate pass would build it against a
		// dropped column.
		tickets.RemoveIndex("idx_tickets_thing")
		tickets.Fields.RemoveByName("thing")
		tickets.Fields.RemoveByName("thing_note")
		// Restore 1806000000's free-text field.
		if tickets.Fields.GetByName("asset") == nil {
			tickets.Fields.Add(&core.TextField{Name: "asset", Max: 200})
		}
		// 1815000000's rule, NOT 1812000000's — restoring serviceTicketsCreateRule
		// here would silently drop the estimated_minutes guard.
		createRule := estimatedEffortTicketsCreateRule()
		tickets.CreateRule = &createRule
		if err := app.Save(tickets); err != nil {
			return fmt.Errorf("save tickets: %w", err)
		}
	}

	if locations, err := app.FindCollectionByNameOrId("locations"); err == nil {
		locations.RemoveIndex("idx_locations_type")
		locations.RemoveIndex("idx_locations_parent")
		locations.Fields.RemoveByName("type")
		locations.Fields.RemoveByName("parent")
		locations.Fields.RemoveByName("metadata")
		if err := app.Save(locations); err != nil {
			return fmt.Errorf("save locations: %w", err)
		}
	}

	// things references both type collections, so it goes first.
	for _, name := range []string{"things", "thing_types", "location_types"} {
		if c, err := app.FindCollectionByNameOrId(name); err == nil {
			if err := app.Delete(c); err != nil {
				return fmt.Errorf("delete %s: %w", name, err)
			}
		}
	}
	return nil
}
