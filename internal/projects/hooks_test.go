package projects

import (
	"testing"

	"github.com/pocketbase/pocketbase/core"

	"github.com/stone-age-io/helpdesk/internal/testutil"
)

func seedCustomer(t *testing.T, app core.App) *core.Record {
	t.Helper()
	col, err := app.FindCollectionByNameOrId("customers")
	if err != nil {
		t.Fatalf("find customers: %v", err)
	}
	rec := core.NewRecord(col)
	rec.Set("name", "Acme")
	rec.Set("active", true)
	if err := app.Save(rec); err != nil {
		t.Fatalf("save customer: %v", err)
	}
	return rec
}

func TestProjectNumberAssignment(t *testing.T) {
	app := testutil.SetupApp(t)
	Register(app)

	customer := seedCustomer(t, app)
	col, err := app.FindCollectionByNameOrId("projects")
	if err != nil {
		t.Fatalf("find projects: %v", err)
	}

	for want := 1; want <= 3; want++ {
		rec := core.NewRecord(col)
		rec.Set("customer", customer.Id)
		rec.Set("title", "Rollout")
		if err := app.Save(rec); err != nil {
			t.Fatalf("save project %d: %v", want, err)
		}
		if got := rec.GetInt("number"); got != want {
			t.Errorf("project number: got %d, want %d", got, want)
		}
		if got := rec.GetString("status"); got != "planned" {
			t.Errorf("default status: got %q, want planned", got)
		}
	}
}

func TestProjectNumberPreservedWhenSet(t *testing.T) {
	app := testutil.SetupApp(t)
	Register(app)

	customer := seedCustomer(t, app)
	col, _ := app.FindCollectionByNameOrId("projects")

	rec := core.NewRecord(col)
	rec.Set("customer", customer.Id)
	rec.Set("title", "explicit number")
	rec.Set("number", 42)
	rec.Set("status", "active")
	if err := app.Save(rec); err != nil {
		t.Fatalf("save: %v", err)
	}
	if got := rec.GetInt("number"); got != 42 {
		t.Errorf("number overwritten: got %d, want 42", got)
	}
	if got := rec.GetString("status"); got != "active" {
		t.Errorf("status overwritten: got %q, want active", got)
	}
}
