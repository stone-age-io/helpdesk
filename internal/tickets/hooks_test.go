package tickets

import (
	"testing"

	"github.com/pocketbase/pocketbase/core"

	"github.com/stone-age-io/helpdesk/internal/testutil"
)

func seedCustomer(t *testing.T, app core.App, name string) *core.Record {
	t.Helper()
	col, err := app.FindCollectionByNameOrId("customers")
	if err != nil {
		t.Fatalf("find customers: %v", err)
	}
	rec := core.NewRecord(col)
	rec.Set("name", name)
	rec.Set("active", true)
	if err := app.Save(rec); err != nil {
		t.Fatalf("save customer: %v", err)
	}
	return rec
}

func TestTicketNumberAssignment(t *testing.T) {
	app := testutil.SetupApp(t)
	Register(app)

	customer := seedCustomer(t, app, "Acme")
	col, err := app.FindCollectionByNameOrId("tickets")
	if err != nil {
		t.Fatalf("find tickets: %v", err)
	}

	for want := 1; want <= 3; want++ {
		rec := core.NewRecord(col)
		rec.Set("customer", customer.Id)
		rec.Set("title", "test")
		if err := app.Save(rec); err != nil {
			t.Fatalf("save ticket %d: %v", want, err)
		}
		if got := rec.GetInt("number"); got != want {
			t.Errorf("ticket number: got %d, want %d", got, want)
		}
		if got := rec.GetString("status"); got != "open" {
			t.Errorf("default status: got %q, want open", got)
		}
		if got := rec.GetString("priority"); got != "normal" {
			t.Errorf("default priority: got %q, want normal", got)
		}
	}
}

func TestTicketNumberPreservedWhenSet(t *testing.T) {
	app := testutil.SetupApp(t)
	Register(app)

	customer := seedCustomer(t, app, "Acme")
	col, _ := app.FindCollectionByNameOrId("tickets")

	rec := core.NewRecord(col)
	rec.Set("customer", customer.Id)
	rec.Set("title", "explicit number")
	rec.Set("number", 42)
	rec.Set("status", "waiting")
	if err := app.Save(rec); err != nil {
		t.Fatalf("save: %v", err)
	}
	if got := rec.GetInt("number"); got != 42 {
		t.Errorf("number overwritten: got %d, want 42", got)
	}
	if got := rec.GetString("status"); got != "waiting" {
		t.Errorf("status overwritten: got %q, want waiting", got)
	}
}
