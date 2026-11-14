package migrations_test

import (
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase/core"

	"github.com/stone-age-io/helpdesk/internal/testutil"
)

// Every code index folds case, so a customer cannot hold two codes that differ
// only by case. This is what keeps the case-insensitive resolvers in
// internal/ingest and internal/inbound single-valued: without it, `rdr-01` and
// `RDR-01` could both exist and a fold-case lookup would return whichever row
// SQLite reached first — a silent wrong-record attach, strictly worse than the
// silent miss it replaced.
func TestCodeUniquenessIgnoresCase(t *testing.T) {
	app := testutil.SetupApp(t)

	acme := seed(t, app, "customers", map[string]any{"name": "Acme", "active": true})
	globex := seed(t, app, "customers", map[string]any{"name": "Globex", "active": true})

	for _, collection := range []string{"things", "thing_types", "location_types", "locations"} {
		seed(t, app, collection, map[string]any{"customer": acme.Id, "name": "A", "code": "RDR-01"})

		col, err := app.FindCollectionByNameOrId(collection)
		if err != nil {
			t.Fatalf("find %s: %v", collection, err)
		}
		dup := core.NewRecord(col)
		dup.Set("customer", acme.Id)
		dup.Set("name", "B")
		dup.Set("code", "rdr-01")
		if err := app.Save(dup); err == nil {
			t.Errorf("%s: a code differing only by case should be rejected", collection)
		}

		// Folding case must not have widened the scope past the customer: two
		// tenants both calling a reader RDR-01 stays correct and expected.
		seed(t, app, collection, map[string]any{"customer": globex.Id, "name": "C", "code": "rdr-01"})

		// Blanks stay exempt — the partial `code != ''` predicate survives the
		// rebuild, and SQLite treats '' as a value rather than NULL.
		seed(t, app, collection, map[string]any{"customer": acme.Id, "name": "No code 1"})
		seed(t, app, collection, map[string]any{"customer": acme.Id, "name": "No code 2"})
	}
}

// customers.code is global rather than per-customer (it is subject token 2),
// and folds the same way.
func TestCustomerCodeUniquenessIgnoresCase(t *testing.T) {
	app := testutil.SetupApp(t)

	seed(t, app, "customers", map[string]any{"name": "Acme", "active": true, "code": "acme"})

	col, err := app.FindCollectionByNameOrId("customers")
	if err != nil {
		t.Fatalf("find customers: %v", err)
	}
	dup := core.NewRecord(col)
	dup.Set("name", "Acme Shouting")
	dup.Set("active", true)
	dup.Set("code", "ACME")
	if err := app.Save(dup); err == nil {
		t.Error("a customer code differing only by case should be rejected")
	}

	// Codeless customers are the normal state for a tenant the platform never
	// onboarded; they must not collide with each other.
	seed(t, app, "customers", map[string]any{"name": "No code 1", "active": true})
	seed(t, app, "customers", map[string]any{"name": "No code 2", "active": true})
}

// The collation lives on the index itself, so it survives anything that reads
// the collection back and re-saves it.
func TestCodeIndexesCarryNocase(t *testing.T) {
	app := testutil.SetupApp(t)

	for _, tc := range []struct{ collection, index string }{
		{"locations", "idx_locations_code"},
		{"things", "idx_things_code"},
		{"thing_types", "idx_thing_types_code"},
		{"location_types", "idx_location_types_code"},
		{"customers", "idx_customers_code"},
	} {
		col, err := app.FindCollectionByNameOrId(tc.collection)
		if err != nil {
			t.Fatalf("find %s: %v", tc.collection, err)
		}
		idx := col.GetIndex(tc.index)
		if idx == "" {
			t.Errorf("%s: index %s is missing", tc.collection, tc.index)
			continue
		}
		if !strings.Contains(strings.ToUpper(idx), "NOCASE") {
			t.Errorf("%s: %s does not fold case: %s", tc.collection, tc.index, idx)
		}
		if !strings.Contains(idx, "code != ''") {
			t.Errorf("%s: %s lost its partial predicate: %s", tc.collection, tc.index, idx)
		}
	}
}
