package timeentries_test

import (
	"testing"

	"github.com/pocketbase/pocketbase/core"

	"github.com/stone-age-io/helpdesk/internal/testutil"
	"github.com/stone-age-io/helpdesk/internal/timeentries"
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

// graph seeds two customers, a requester in each, a staffer, and a ticket for
// customer A with 30+90 minutes logged.
func graph(t *testing.T, app core.App, showTime bool) (ticket, staffRec, reqA, reqB *core.Record) {
	t.Helper()
	custA := seed(t, app, "customers", map[string]any{"name": "Acme", "active": true, "show_time_to_requester": showTime})
	custB := seed(t, app, "customers", map[string]any{"name": "Globex", "active": true})
	staffRec = seed(t, app, "staff", map[string]any{
		"email": "sam@816tech.example", "password": "secret123456",
		"name": "Sam", "role": "agent", "active": true,
	})
	reqA = seed(t, app, "users", map[string]any{
		"email": "rita@acme.example", "password": "secret123456",
		"name": "Rita", "customer": custA.Id, "active": true,
	})
	reqB = seed(t, app, "users", map[string]any{
		"email": "gus@globex.example", "password": "secret123456",
		"name": "Gus", "customer": custB.Id, "active": true,
	})
	ticket = seed(t, app, "tickets", map[string]any{"customer": custA.Id, "title": "pump", "number": 1})
	seed(t, app, "time_entries", map[string]any{"ticket": ticket.Id, "staff": staffRec.Id, "minutes": 30, "work_date": "2026-07-14 09:00:00.000Z"})
	seed(t, app, "time_entries", map[string]any{"ticket": ticket.Id, "staff": staffRec.Id, "minutes": 90, "work_date": "2026-07-14 10:00:00.000Z"})
	return ticket, staffRec, reqA, reqB
}

func TestSumMinutes(t *testing.T) {
	app := testutil.SetupApp(t)
	ticket, _, _, _ := graph(t, app, false)
	got, err := timeentries.SumMinutes(app, ticket.Id, false)
	if err != nil {
		t.Fatalf("SumMinutes: %v", err)
	}
	if got != 120 {
		t.Errorf("SumMinutes: got %d, want 120", got)
	}
}

// billableOnly excludes entries flagged non_billable; the full total keeps
// counting them. The customer-facing figure is the billable one.
func TestSumMinutesBillableOnly(t *testing.T) {
	app := testutil.SetupApp(t)
	ticket, staffRec, _, _ := graph(t, app, false) // 30 + 90 billable already seeded
	seed(t, app, "time_entries", map[string]any{
		"ticket": ticket.Id, "staff": staffRec.Id, "minutes": 45,
		"work_date": "2026-07-14 11:00:00.000Z", "non_billable": true,
	})

	full, err := timeentries.SumMinutes(app, ticket.Id, false)
	if err != nil {
		t.Fatalf("SumMinutes full: %v", err)
	}
	if full != 165 {
		t.Errorf("full total: got %d, want 165", full)
	}

	billable, err := timeentries.SumMinutes(app, ticket.Id, true)
	if err != nil {
		t.Fatalf("SumMinutes billable: %v", err)
	}
	if billable != 120 {
		t.Errorf("billable total: got %d, want 120", billable)
	}
}

func TestStaffAlwaysAllowed(t *testing.T) {
	app := testutil.SetupApp(t)
	ticket, staffRec, _, _ := graph(t, app, false) // flag off, staff still allowed
	if !timeentries.AllowTimeTotal(app, staffRec, ticket) {
		t.Error("staff should always see the total")
	}
}

func TestRequesterBlockedWhenFlagOff(t *testing.T) {
	app := testutil.SetupApp(t)
	ticket, _, reqA, _ := graph(t, app, false)
	if timeentries.AllowTimeTotal(app, reqA, ticket) {
		t.Error("owning requester must NOT see the total when the customer opt-in is off")
	}
}

func TestRequesterAllowedWhenFlagOn(t *testing.T) {
	app := testutil.SetupApp(t)
	ticket, _, reqA, _ := graph(t, app, true)
	if !timeentries.AllowTimeTotal(app, reqA, ticket) {
		t.Error("owning requester should see the total when the customer opt-in is on")
	}
}

func TestOtherCustomerRequesterBlocked(t *testing.T) {
	app := testutil.SetupApp(t)
	ticket, _, _, reqB := graph(t, app, true) // flag on, but reqB is another customer
	if timeentries.AllowTimeTotal(app, reqB, ticket) {
		t.Error("a requester of another customer must never see the total")
	}
}

func TestNilAuthBlocked(t *testing.T) {
	app := testutil.SetupApp(t)
	ticket, _, _, _ := graph(t, app, true)
	if timeentries.AllowTimeTotal(app, nil, ticket) {
		t.Error("unauthenticated caller must be blocked")
	}
}
