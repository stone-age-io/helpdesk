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
	// The tenant token on subject token 2 (ADR 0002). platform_org_id is set too
	// so a test that regressed to resolving by it would still fail: the two must
	// not be interchangeable.
	customer.Set("code", "acme")
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

	out := c.Project("helpdesk.acme.tickets.create",
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
	if got := rec.GetString("origin_subject"); got != "helpdesk.acme.tickets.create" {
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
	if got := rec.GetString("thing_note"); got != "pump-7" {
		t.Errorf("thing_note: got %q, want pump-7", got)
	}
	if got := rec.GetString("location_note"); got != "line-3" {
		t.Errorf("location_note: got %q, want line-3", got)
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
	if out := c.Project("helpdesk.acme.tickets.create",
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
	if out := c.Project("helpdesk.acme.tickets.create",
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

	if out := c.Project("helpdesk.acme.tickets.create", payload); out != Ack {
		t.Fatalf("first: %v", out)
	}
	if out := c.Project("helpdesk.acme.tickets.create", payload); out != Ack {
		t.Fatalf("second: %v", out)
	}
	if n := countTickets(t, app); n != 1 {
		t.Errorf("dedupe failed: %d tickets", n)
	}
}

func TestProjectRejectsGarbage(t *testing.T) {
	app, c, _ := setup(t)
	cases := map[string][2]string{
		"bad json":        {"helpdesk.acme.tickets.create", `{"title":`},
		"missing title":   {"helpdesk.acme.tickets.create", `{"body":"no title"}`},
		"unknown verb":    {"helpdesk.acme.tickets.resolve", `{"title":"x"}`},
		"unparseable":     {"helpdesk.tickets.create", `{"title":"x"}`},
		"invalid priority": {"helpdesk.acme.tickets.create", `{"title":"prio","priority":"catastrophic"}`},
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

func TestProjectResolvesLocationByCode(t *testing.T) {
	app, c, customer := setup(t)

	locCol, _ := app.FindCollectionByNameOrId("locations")
	loc := core.NewRecord(locCol)
	loc.Set("customer", customer.Id)
	loc.Set("code", "BLDG-C")
	loc.Set("name", "Acme HQ - Bldg C")
	if err := app.Save(loc); err != nil {
		t.Fatalf("seed location: %v", err)
	}

	// Matching code → the structured relation is set.
	if out := c.Project("helpdesk.acme.tickets.create",
		[]byte(`{"title":"a","location_code":"BLDG-C"}`)); out != Ack {
		t.Fatalf("matching code: %v", out)
	}
	recA, err := app.FindFirstRecordByFilter("tickets", "title = 'a'")
	if err != nil {
		t.Fatalf("ticket a: %v", err)
	}
	if got := recA.GetString("location"); got != loc.Id {
		t.Errorf("location relation: got %q, want %q", got, loc.Id)
	}

	// Unknown code → no relation; the code is kept as a breadcrumb in the note.
	if out := c.Project("helpdesk.acme.tickets.create",
		[]byte(`{"title":"b","location_code":"NOPE"}`)); out != Ack {
		t.Fatalf("unknown code: %v", out)
	}
	recB, err := app.FindFirstRecordByFilter("tickets", "title = 'b'")
	if err != nil {
		t.Fatalf("ticket b: %v", err)
	}
	if got := recB.GetString("location"); got != "" {
		t.Errorf("unknown code should leave location empty, got %q", got)
	}
	if got := recB.GetString("location_note"); got != "NOPE" {
		t.Errorf("unknown-code breadcrumb: got %q, want NOPE", got)
	}

	// Free-text location wins the note even alongside an unresolved code.
	if out := c.Project("helpdesk.acme.tickets.create",
		[]byte(`{"title":"cc","location":"rear dock","location_code":"NOPE"}`)); out != Ack {
		t.Fatalf("freetext+code: %v", out)
	}
	recC, _ := app.FindFirstRecordByFilter("tickets", "title = 'cc'")
	if got := recC.GetString("location_note"); got != "rear dock" {
		t.Errorf("free-text should win location_note: got %q, want 'rear dock'", got)
	}
}

func TestProjectResolvesThingByCode(t *testing.T) {
	app, c, customer := setup(t)

	thingCol, _ := app.FindCollectionByNameOrId("things")
	thing := core.NewRecord(thingCol)
	thing.Set("customer", customer.Id)
	thing.Set("code", "PUMP-7")
	thing.Set("name", "Line 3 Feed Pump")
	if err := app.Save(thing); err != nil {
		t.Fatalf("seed thing: %v", err)
	}

	// Matching code → the structured relation is set.
	if out := c.Project("helpdesk.acme.tickets.create",
		[]byte(`{"title":"a","thing_code":"PUMP-7"}`)); out != Ack {
		t.Fatalf("matching code: %v", out)
	}
	recA, err := app.FindFirstRecordByFilter("tickets", "title = 'a'")
	if err != nil {
		t.Fatalf("ticket a: %v", err)
	}
	if got := recA.GetString("thing"); got != thing.Id {
		t.Errorf("thing relation: got %q, want %q", got, thing.Id)
	}

	// Unknown code → no relation; the code is kept as a breadcrumb in the note.
	if out := c.Project("helpdesk.acme.tickets.create",
		[]byte(`{"title":"b","thing_code":"NOPE"}`)); out != Ack {
		t.Fatalf("unknown code: %v", out)
	}
	recB, err := app.FindFirstRecordByFilter("tickets", "title = 'b'")
	if err != nil {
		t.Fatalf("ticket b: %v", err)
	}
	if got := recB.GetString("thing"); got != "" {
		t.Errorf("unknown code should leave thing empty, got %q", got)
	}
	if got := recB.GetString("thing_note"); got != "NOPE" {
		t.Errorf("unknown-code breadcrumb: got %q, want NOPE", got)
	}

	// Free-text thing wins the note even alongside an unresolved code.
	if out := c.Project("helpdesk.acme.tickets.create",
		[]byte(`{"title":"cc","thing":"the big pump","thing_code":"NOPE"}`)); out != Ack {
		t.Fatalf("freetext+code: %v", out)
	}
	recC, _ := app.FindFirstRecordByFilter("tickets", "title = 'cc'")
	if got := recC.GetString("thing_note"); got != "the big pump" {
		t.Errorf("free-text should win thing_note: got %q, want 'the big pump'", got)
	}

	// A resolved thing must NOT backfill the ticket's location, even though the
	// thing has one. The two resolvers stay independent; inference is UI work.
	locCol, _ := app.FindCollectionByNameOrId("locations")
	loc := core.NewRecord(locCol)
	loc.Set("customer", customer.Id)
	loc.Set("name", "Plant Floor")
	if err := app.Save(loc); err != nil {
		t.Fatalf("seed location: %v", err)
	}
	thing.Set("location", loc.Id)
	if err := app.Save(thing); err != nil {
		t.Fatalf("update thing: %v", err)
	}
	if out := c.Project("helpdesk.acme.tickets.create",
		[]byte(`{"title":"dd","thing_code":"PUMP-7"}`)); out != Ack {
		t.Fatalf("placed thing: %v", out)
	}
	recD, _ := app.FindFirstRecordByFilter("tickets", "title = 'dd'")
	if got := recD.GetString("location"); got != "" {
		t.Errorf("resolved thing should not backfill ticket location, got %q", got)
	}
}

// The subject's org token is changing shape: it has always been the platform's
// PocketBase org id, and under ADR 0002 it is the organization code.
//
// The transition is over: the platform_org_id fallback is gone, and this pins
// that it stays gone. The harness customer carries BOTH a code and a
// platform_org_id, so a regression that reinstated the fallback — or resolved
// against the wrong column outright — fails the second subtest instead of
// quietly widening what the subject means.
func TestProjectResolvesCustomerByCodeOnly(t *testing.T) {
	app, c, customer := setup(t)

	t.Run("the organization code resolves", func(t *testing.T) {
		subject := "helpdesk.acme.tickets.create"
		if out := c.Project(subject, []byte(`{"title":"by-code"}`)); out != Ack {
			t.Fatalf("Project = %v, want Ack", out)
		}
		rec, err := app.FindFirstRecordByFilter("tickets", "title = 'by-code'")
		if err != nil {
			t.Fatalf("ticket not created: %v", err)
		}
		if got := rec.GetString("customer"); got != customer.Id {
			t.Errorf("customer: got %q, want %q", got, customer.Id)
		}
		if got := rec.GetString("origin_subject"); got != subject {
			t.Errorf("origin_subject: got %q, want %q", got, subject)
		}
	})

	t.Run("the platform org id no longer resolves", func(t *testing.T) {
		before := countTickets(t, app)
		// "org123" IS this customer's platform_org_id. It must not be a tenant
		// token: one subject position resolving against two columns is the
		// ambiguity ADR 0002 removed.
		if out := c.Project("helpdesk.org123.tickets.create", []byte(`{"title":"by-id"}`)); out != Ack {
			t.Fatalf("Project = %v, want Ack", out)
		}
		if after := countTickets(t, app); after != before {
			t.Errorf("ticket count changed %d -> %d; platform_org_id still resolves as a tenant token", before, after)
		}
	})
}

// A code belonging to no customer is an unmapped org: acked, not retried, and
// no ticket.
func TestProjectUnknownCodeAcksWithoutTicket(t *testing.T) {
	app, c, _ := setup(t)

	before := countTickets(t, app)
	if out := c.Project("helpdesk.not-a-tenant.tickets.create", []byte(`{"title":"x"}`)); out != Ack {
		t.Fatalf("Project = %v, want Ack", out)
	}
	if after := countTickets(t, app); after != before {
		t.Errorf("ticket count changed %d -> %d, want no ticket", before, after)
	}
}
