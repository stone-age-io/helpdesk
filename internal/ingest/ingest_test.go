package ingest

import (
	"testing"

	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/core"

	"github.com/stone-age-io/helpdesk/internal/subjects"
	"github.com/stone-age-io/helpdesk/internal/testutil"
	"github.com/stone-age-io/helpdesk/internal/tickets"
)

func setup(t *testing.T) (*pocketbase.PocketBase, *Consumer, *core.Record) {
	t.Helper()
	app := testutil.SetupApp(t)
	tickets.Register(app)

	col, _ := app.FindCollectionByNameOrId("customers")
	customer := core.NewRecord(col)
	customer.Set("name", "Acme Corp")
	customer.Set("active", true)
	customer.Set("platform_org_id", "org123")
	if err := app.Save(customer); err != nil {
		t.Fatalf("save customer: %v", err)
	}

	// js is nil — Project never touches the broker.
	c := New(app, nil, "HELPDESK_EVENTS", "helpdesk-ingest", subjects.Default())
	return app, c, customer
}

func countTickets(t *testing.T, app core.App) int {
	t.Helper()
	rows, err := app.FindRecordsByFilter("tickets", "", "", 0, 0)
	if err != nil {
		t.Fatalf("list tickets: %v", err)
	}
	return len(rows)
}

func TestProjectCreatesTicketWithProvenance(t *testing.T) {
	app, c, customer := setup(t)

	out := c.Project("helpdesk.org123.tickets.create",
		[]byte(`{"title":"pump fault","body":"overcurrent","priority":"high","thing":"pump-7","location":"line-3"}`))
	if out != Ack {
		t.Fatalf("Project = %v, want Ack", out)
	}

	rec, err := app.FindFirstRecordByFilter("tickets", "source = 'nats'")
	if err != nil {
		t.Fatalf("ticket not created: %v", err)
	}
	if got := rec.GetString("customer"); got != customer.Id {
		t.Errorf("customer: got %q, want %q", got, customer.Id)
	}
	if got := rec.GetString("origin_subject"); got != "helpdesk.org123.tickets.create" {
		t.Errorf("origin_subject: got %q", got)
	}
	if got := rec.GetString("priority"); got != "high" {
		t.Errorf("priority: got %q", got)
	}
	if got := rec.GetInt("number"); got != 1 {
		t.Errorf("ticket number hook did not fire: number=%d", got)
	}
	if got := rec.GetString("status"); got != "open" {
		t.Errorf("status default: got %q", got)
	}
	// Provenance is now structured fields, not folded into the body.
	if got := rec.GetString("body"); got != "overcurrent" {
		t.Errorf("body: got %q, want %q", got, "overcurrent")
	}
	if got := rec.GetString("asset"); got != "pump-7" {
		t.Errorf("asset: got %q, want pump-7", got)
	}
	if got := rec.GetString("location"); got != "line-3" {
		t.Errorf("location: got %q, want line-3", got)
	}
}

func TestProjectResolvesCategoryByKey(t *testing.T) {
	app, c, _ := setup(t)

	catCol, _ := app.FindCollectionByNameOrId("ticket_categories")
	cat := core.NewRecord(catCol)
	cat.Set("name", "Pumps")
	cat.Set("key", "pumps")
	cat.Set("active", true)
	if err := app.Save(cat); err != nil {
		t.Fatalf("seed category: %v", err)
	}

	// Known key → classified.
	if out := c.Project("helpdesk.org123.tickets.create",
		[]byte(`{"title":"a","category":"pumps"}`)); out != Ack {
		t.Fatalf("known category: %v", out)
	}
	rec, err := app.FindFirstRecordByFilter("tickets", "title = 'a'")
	if err != nil {
		t.Fatalf("ticket: %v", err)
	}
	if got := rec.GetString("category"); got != cat.Id {
		t.Errorf("category: got %q, want %q", got, cat.Id)
	}

	// Unknown key → created but unclassified (no drop, no error).
	if out := c.Project("helpdesk.org123.tickets.create",
		[]byte(`{"title":"b","category":"nonexistent"}`)); out != Ack {
		t.Fatalf("unknown category: %v", out)
	}
	rec2, err := app.FindFirstRecordByFilter("tickets", "title = 'b'")
	if err != nil {
		t.Fatalf("ticket b: %v", err)
	}
	if got := rec2.GetString("category"); got != "" {
		t.Errorf("unknown category should leave ticket unclassified, got %q", got)
	}
}

func TestProjectUnknownOrgAcksWithoutTicket(t *testing.T) {
	app, c, _ := setup(t)
	out := c.Project("helpdesk.other-org.tickets.create", []byte(`{"title":"x"}`))
	if out != Ack {
		t.Fatalf("unknown org: got %v, want Ack (operator maps later; no redelivery storm)", out)
	}
	if n := countTickets(t, app); n != 0 {
		t.Errorf("unknown org created %d tickets", n)
	}
}

func TestProjectDedupeKeyIsIdempotent(t *testing.T) {
	app, c, _ := setup(t)
	payload := []byte(`{"title":"pump fault","dedupe_key":"pump-7-overcurrent"}`)

	if out := c.Project("helpdesk.org123.tickets.create", payload); out != Ack {
		t.Fatalf("first: %v", out)
	}
	if out := c.Project("helpdesk.org123.tickets.create", payload); out != Ack {
		t.Fatalf("second: %v", out)
	}
	if n := countTickets(t, app); n != 1 {
		t.Errorf("dedupe failed: %d tickets", n)
	}
}

func TestProjectRejectsGarbage(t *testing.T) {
	app, c, _ := setup(t)
	cases := map[string][2]string{
		"bad json":        {"helpdesk.org123.tickets.create", `{"title":`},
		"missing title":   {"helpdesk.org123.tickets.create", `{"body":"no title"}`},
		"unknown verb":    {"helpdesk.org123.tickets.resolve", `{"title":"x"}`},
		"unparseable":     {"helpdesk.tickets.create", `{"title":"x"}`},
		"invalid priority": {"helpdesk.org123.tickets.create", `{"title":"prio","priority":"catastrophic"}`},
	}
	for name, c2 := range cases {
		if out := c.Project(c2[0], []byte(c2[1])); out != Ack {
			t.Errorf("%s: got %v, want Ack (terminal, no redelivery)", name, out)
		}
	}
	// Only the invalid-priority case creates a ticket (clamped to normal).
	rec, err := app.FindFirstRecordByFilter("tickets", "title = 'prio'")
	if err != nil {
		t.Fatalf("clamped-priority ticket missing: %v", err)
	}
	if got := rec.GetString("priority"); got != "normal" {
		t.Errorf("priority clamp: got %q, want normal", got)
	}
	if n := countTickets(t, app); n != 1 {
		t.Errorf("garbage created tickets: %d total", n)
	}
}
