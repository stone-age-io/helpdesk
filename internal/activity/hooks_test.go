package activity

import (
	"testing"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"

	"github.com/stone-age-io/helpdesk/internal/testutil"
)

func seed(t *testing.T, app core.App, collection string, fields map[string]any) *core.Record {
	t.Helper()
	col, err := app.FindCollectionByNameOrId(collection)
	if err != nil {
		t.Fatalf("find %s: %v", collection, err)
	}
	rec := core.NewRecord(col)
	for k, v := range fields {
		rec.Set(k, v)
	}
	if err := app.Save(rec); err != nil {
		t.Fatalf("save %s: %v", collection, err)
	}
	return rec
}

func events(t *testing.T, app core.App, ticketID string) []*core.Record {
	t.Helper()
	rows, err := app.FindRecordsByFilter("ticket_events", "ticket = {:t}", "created", 0, 0, dbx.Params{"t": ticketID})
	if err != nil {
		t.Fatalf("list events: %v", err)
	}
	return rows
}

// setup returns the app and a ticket id. Tests reload the ticket before
// mutating so Record.Original() reflects committed DB state — exactly what
// the record API does on a real update (a reused create instance carries an
// empty Original and would diff every field).
func setup(t *testing.T) (core.App, string) {
	t.Helper()
	app := testutil.SetupApp(t)
	Register(app)
	customer := seed(t, app, "customers", map[string]any{"name": "Acme", "active": true})
	ticket := seed(t, app, "tickets", map[string]any{
		"customer": customer.Id, "title": "pump", "number": 1, "status": "open", "priority": "normal",
	})
	return app, ticket.Id
}

func reload(t *testing.T, app core.App, id string) *core.Record {
	t.Helper()
	rec, err := app.FindRecordById("tickets", id)
	if err != nil {
		t.Fatalf("reload ticket: %v", err)
	}
	return rec
}

func TestStatusChangeLogged(t *testing.T) {
	app, id := setup(t)
	ticket := reload(t, app, id)
	ticket.Set("status", "resolved")
	if err := app.Save(ticket); err != nil {
		t.Fatalf("update: %v", err)
	}
	evs := events(t, app, ticket.Id)
	if len(evs) != 1 {
		t.Fatalf("want 1 event, got %d", len(evs))
	}
	e := evs[0]
	if e.GetString("field") != "status" || e.GetString("old_value") != "open" || e.GetString("new_value") != "resolved" {
		t.Errorf("bad event: field=%q old=%q new=%q", e.GetString("field"), e.GetString("old_value"), e.GetString("new_value"))
	}
}

func TestAssigneeChangeResolvesName(t *testing.T) {
	app, id := setup(t)
	tech := seed(t, app, "staff", map[string]any{
		"email": "sam@x.example", "password": "secret123456", "name": "Sam Staff", "role": "agent", "active": true,
	})
	ticket := reload(t, app, id)
	ticket.Set("assignee", tech.Id)
	if err := app.Save(ticket); err != nil {
		t.Fatalf("update: %v", err)
	}
	evs := events(t, app, ticket.Id)
	if len(evs) != 1 {
		t.Fatalf("want 1 event, got %d", len(evs))
	}
	if got := evs[0].GetString("new_value"); got != "Sam Staff" {
		t.Errorf("assignee new_value: got %q, want the staff name", got)
	}
	if got := evs[0].GetString("old_value"); got != "Unassigned" {
		t.Errorf("assignee old_value: got %q, want Unassigned", got)
	}
}

func TestActorAttribution(t *testing.T) {
	app, id := setup(t)
	tech := seed(t, app, "staff", map[string]any{
		"email": "sam@x.example", "password": "secret123456", "name": "Sam Staff", "role": "agent", "active": true,
	})
	ticket := reload(t, app, id)
	ticket.Set("priority", "urgent")
	SetActor(ticket, "staff", tech.Id)
	if err := app.Save(ticket); err != nil {
		t.Fatalf("update: %v", err)
	}
	evs := events(t, app, ticket.Id)
	if len(evs) != 1 {
		t.Fatalf("want 1 event, got %d", len(evs))
	}
	if got := evs[0].GetString("actor_staff"); got != tech.Id {
		t.Errorf("actor_staff: got %q, want %q", got, tech.Id)
	}
}

func TestUnaudittedFieldNotLogged(t *testing.T) {
	app, id := setup(t)
	ticket := reload(t, app, id)
	ticket.Set("title", "renamed")
	if err := app.Save(ticket); err != nil {
		t.Fatalf("update: %v", err)
	}
	if evs := events(t, app, ticket.Id); len(evs) != 0 {
		t.Errorf("title change should not log an event, got %d", len(evs))
	}
}
