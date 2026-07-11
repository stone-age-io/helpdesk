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
	body := rec.GetString("body")
	if body != "overcurrent\n\n[thing: pump-7 · location: line-3]" {
		t.Errorf("body with provenance tags: got %q", body)
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
