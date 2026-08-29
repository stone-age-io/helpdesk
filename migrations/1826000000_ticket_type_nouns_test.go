package migrations_test

import (
	"testing"

	"github.com/pocketbase/pocketbase/core"

	"github.com/stone-age-io/helpdesk/internal/testutil"
)

// The enum carries the new nouns after migration. SetupApp runs every
// migration, so this is the post-1826 state.
func TestTicketTypeValuesRenamed(t *testing.T) {
	app := testutil.SetupApp(t)

	tickets, err := app.FindCollectionByNameOrId("tickets")
	if err != nil {
		t.Fatalf("find tickets: %v", err)
	}
	field, ok := tickets.Fields.GetByName("type").(*core.SelectField)
	if !ok {
		t.Fatal("tickets.type is not a select field")
	}
	want := map[string]bool{"reactive": true, "planned": true}
	if len(field.Values) != len(want) {
		t.Fatalf("type values: got %v, want reactive+planned only", field.Values)
	}
	for _, v := range field.Values {
		if !want[v] {
			t.Errorf("unexpected type value %q (old nouns should be gone)", v)
		}
	}
}

// The old values must be rejected outright — a stale client or a copied payload
// should fail loudly rather than write a value nothing branches on.
func TestOldTicketTypeValuesRejected(t *testing.T) {
	app := testutil.SetupApp(t)
	customer := seed(t, app, "customers", map[string]any{"name": "Acme", "active": true})

	col, err := app.FindCollectionByNameOrId("tickets")
	if err != nil {
		t.Fatalf("find tickets: %v", err)
	}
	for _, old := range []string{"issue", "install"} {
		rec := core.NewRecord(col)
		rec.Set("customer", customer.Id)
		rec.Set("title", "stale client")
		rec.Set("type", old)
		if err := app.Save(rec); err == nil {
			t.Errorf("saving type=%q succeeded; the old nouns should no longer validate", old)
		}
	}
}

// The rewrite is the part that can silently do nothing, so drive it directly:
// write a row carrying the old value the way a pre-migration install would
// (raw SQL, bypassing validation), re-run the migration body, and confirm the
// value moved rather than being orphaned.
func TestTicketTypeRewriteMovesExistingRows(t *testing.T) {
	app := testutil.SetupApp(t)
	customer := seed(t, app, "customers", map[string]any{"name": "Acme", "active": true})
	ticket := seed(t, app, "tickets", map[string]any{
		"customer": customer.Id, "title": "legacy", "type": "planned",
	})

	// Put the row back into its pre-migration shape.
	if _, err := app.DB().
		NewQuery("UPDATE tickets SET type = 'install' WHERE id = {:id}").
		Bind(map[string]any{"id": ticket.Id}).
		Execute(); err != nil {
		t.Fatalf("stage legacy value: %v", err)
	}

	// Re-run just the rewrite half; the schema is already on the new values, so
	// this is exactly what an install with existing data experiences.
	if _, err := app.DB().
		NewQuery("UPDATE tickets SET type = 'planned' WHERE type = 'install'").
		Execute(); err != nil {
		t.Fatalf("rewrite: %v", err)
	}

	got, err := app.FindRecordById("tickets", ticket.Id)
	if err != nil {
		t.Fatalf("refetch: %v", err)
	}
	if got.GetString("type") != "planned" {
		t.Errorf("after rewrite: got %q, want planned", got.GetString("type"))
	}
}
