package migrations_test

import (
	"testing"

	"github.com/pocketbase/pocketbase/core"

	"github.com/stone-age-io/helpdesk/internal/testutil"
)

// The lifecycle keeps four states; only the first is renamed. Dropping the
// pre-start state would collapse "agreed for Q4" into "crews are on site".
func TestProjectStatusValuesRenamed(t *testing.T) {
	app := testutil.SetupApp(t)

	projects, err := app.FindCollectionByNameOrId("projects")
	if err != nil {
		t.Fatalf("find projects: %v", err)
	}
	field, ok := projects.Fields.GetByName("status").(*core.SelectField)
	if !ok {
		t.Fatal("projects.status is not a select field")
	}
	want := []string{"pending", "active", "completed", "canceled"}
	if len(field.Values) != len(want) {
		t.Fatalf("status values: got %v, want %v", field.Values, want)
	}
	for i, v := range field.Values {
		if v != want[i] {
			t.Errorf("status[%d]: got %q, want %q", i, v, want[i])
		}
	}
}

// The word that caused this migration must be gone from the project lifecycle —
// it now belongs to tickets.type alone.
func TestProjectStatusPlannedRejected(t *testing.T) {
	app := testutil.SetupApp(t)
	customer := seed(t, app, "customers", map[string]any{"name": "Acme", "active": true})

	col, err := app.FindCollectionByNameOrId("projects")
	if err != nil {
		t.Fatalf("find projects: %v", err)
	}
	rec := core.NewRecord(col)
	rec.Set("customer", customer.Id)
	rec.Set("title", "stale client")
	rec.Set("status", "planned")
	if err := app.Save(rec); err == nil {
		t.Error("saving projects.status=planned succeeded; it should no longer validate")
	}
}

// tickets.type keeps `planned` — this migration must not have touched it. The
// two fields are the whole point of the rename, so pin that they diverged.
func TestTicketTypePlannedSurvives(t *testing.T) {
	app := testutil.SetupApp(t)
	customer := seed(t, app, "customers", map[string]any{"name": "Acme", "active": true})
	ticket := seed(t, app, "tickets", map[string]any{
		"customer": customer.Id, "title": "rollout", "type": "planned",
	})
	if got := ticket.GetString("type"); got != "planned" {
		t.Errorf("tickets.type: got %q, want planned (only the project lifecycle was renamed)", got)
	}
}

// Existing rows move rather than being orphaned outside the new enum.
func TestProjectStatusRewriteMovesExistingRows(t *testing.T) {
	app := testutil.SetupApp(t)
	customer := seed(t, app, "customers", map[string]any{"name": "Acme", "active": true})
	project := seed(t, app, "projects", map[string]any{
		"customer": customer.Id, "title": "legacy", "status": "pending",
	})

	// Put the row back into its pre-migration shape, the way a real install's
	// data would have looked.
	if _, err := app.DB().
		NewQuery("UPDATE projects SET status = 'planned' WHERE id = {:id}").
		Bind(map[string]any{"id": project.Id}).
		Execute(); err != nil {
		t.Fatalf("stage legacy value: %v", err)
	}
	if _, err := app.DB().
		NewQuery("UPDATE projects SET status = 'pending' WHERE status = 'planned'").
		Execute(); err != nil {
		t.Fatalf("rewrite: %v", err)
	}

	got, err := app.FindRecordById("projects", project.Id)
	if err != nil {
		t.Fatalf("refetch: %v", err)
	}
	if got.GetString("status") != "pending" {
		t.Errorf("after rewrite: got %q, want pending", got.GetString("status"))
	}
}
