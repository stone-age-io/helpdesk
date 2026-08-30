package timeentries_test

import (
	"testing"

	"github.com/pocketbase/pocketbase/core"

	"github.com/stone-age-io/helpdesk/internal/testutil"
	"github.com/stone-age-io/helpdesk/internal/timeentries"
)

// summaryGraph seeds two customers with a ticket each, and time on both, so a
// tenant leak would be visible as an extra key in the map. Customer A's ticket
// carries 30 billable + 45 non-billable on 2026-07-14, plus 60 billable on
// 2026-08-02 — enough to exercise both the billable filter and the window.
func summaryGraph(t *testing.T, app core.App) (custA string, ticketA, ticketB *core.Record) {
	t.Helper()
	a := seed(t, app, "customers", map[string]any{"name": "Acme", "active": true, "show_time_to_requester": true})
	b := seed(t, app, "customers", map[string]any{"name": "Globex", "active": true})
	staffRec := seed(t, app, "staff", map[string]any{
		"email": "sam@msp.example", "password": "secret123456",
		"name": "Sam", "role": "agent", "active": true,
	})
	ticketA = seed(t, app, "tickets", map[string]any{"customer": a.Id, "title": "pump", "number": 1})
	ticketB = seed(t, app, "tickets", map[string]any{"customer": b.Id, "title": "door", "number": 2})

	seed(t, app, "time_entries", map[string]any{"ticket": ticketA.Id, "staff": staffRec.Id, "minutes": 30, "work_date": "2026-07-14 09:00:00.000Z"})
	seed(t, app, "time_entries", map[string]any{"ticket": ticketA.Id, "staff": staffRec.Id, "minutes": 45, "work_date": "2026-07-14 10:00:00.000Z", "non_billable": true})
	seed(t, app, "time_entries", map[string]any{"ticket": ticketA.Id, "staff": staffRec.Id, "minutes": 60, "work_date": "2026-08-02 09:00:00.000Z"})
	seed(t, app, "time_entries", map[string]any{"ticket": ticketB.Id, "staff": staffRec.Id, "minutes": 15, "work_date": "2026-07-14 09:00:00.000Z"})
	return a.Id, ticketA, ticketB
}

// The customer scope is a relation hop to the ticket — time_entries itself
// carries no customer field, so this is the only thing standing between two
// tenants' ledgers.
func TestSumMinutesByTicketScopesToCustomer(t *testing.T) {
	app := testutil.SetupApp(t)
	custA, ticketA, ticketB := summaryGraph(t, app)

	got, err := timeentries.SumMinutesByTicket(app, custA, "", "", false)
	if err != nil {
		t.Fatalf("SumMinutesByTicket: %v", err)
	}
	if _, leaked := got[ticketB.Id]; leaked {
		t.Error("another customer's ticket appeared in the map")
	}
	if got[ticketA.Id] != 135 {
		t.Errorf("ticket A full total: got %d, want 135", got[ticketA.Id])
	}
}

// The requester-facing figure excludes non_billable, matching SumMinutes and
// the per-ticket time-total route.
func TestSumMinutesByTicketBillableOnly(t *testing.T) {
	app := testutil.SetupApp(t)
	custA, ticketA, _ := summaryGraph(t, app)

	got, err := timeentries.SumMinutesByTicket(app, custA, "", "", true)
	if err != nil {
		t.Fatalf("SumMinutesByTicket: %v", err)
	}
	if got[ticketA.Id] != 90 {
		t.Errorf("ticket A billable total: got %d, want 90 (30 + 60, the 45 written off)", got[ticketA.Id])
	}
}

// The window bounds on work_date, and the upper bound is inclusive of the whole
// day — a bare date compared against a stored timestamp would otherwise drop
// every entry logged on the last day of the range.
func TestSumMinutesByTicketWindow(t *testing.T) {
	app := testutil.SetupApp(t)
	custA, ticketA, _ := summaryGraph(t, app)

	julyOnly, err := timeentries.SumMinutesByTicket(app, custA, "2026-07-01", "2026-07-14", true)
	if err != nil {
		t.Fatalf("SumMinutesByTicket july: %v", err)
	}
	if julyOnly[ticketA.Id] != 30 {
		t.Errorf("july billable: got %d, want 30 (the 2026-07-14 entry, end-of-day inclusive)", julyOnly[ticketA.Id])
	}

	augOnly, err := timeentries.SumMinutesByTicket(app, custA, "2026-08-01", "2026-08-31", true)
	if err != nil {
		t.Fatalf("SumMinutesByTicket august: %v", err)
	}
	if augOnly[ticketA.Id] != 60 {
		t.Errorf("august billable: got %d, want 60", augOnly[ticketA.Id])
	}
}

// An empty customer scope is only ever reached by a staff caller that passed no
// ?customer — it must mean "everything", not "nothing".
func TestSumMinutesByTicketUnscopedSeesAll(t *testing.T) {
	app := testutil.SetupApp(t)
	_, ticketA, ticketB := summaryGraph(t, app)

	got, err := timeentries.SumMinutesByTicket(app, "", "", "", false)
	if err != nil {
		t.Fatalf("SumMinutesByTicket: %v", err)
	}
	if got[ticketA.Id] != 135 || got[ticketB.Id] != 15 {
		t.Errorf("unscoped totals: got A=%d B=%d, want A=135 B=15", got[ticketA.Id], got[ticketB.Id])
	}
}

// --- policy: who gets what from the batch route ---

// scopeGraph seeds one customer (opt-in configurable) with a requester and a
// staffer.
func scopeGraph(t *testing.T, app core.App, showTime bool) (custID string, staffRec, req *core.Record) {
	t.Helper()
	cust := seed(t, app, "customers", map[string]any{"name": "Acme", "active": true, "show_time_to_requester": showTime})
	staffRec = seed(t, app, "staff", map[string]any{
		"email": "sam@msp.example", "password": "secret123456",
		"name": "Sam", "role": "agent", "active": true,
	})
	req = seed(t, app, "users", map[string]any{
		"email": "rita@acme.example", "password": "secret123456",
		"name": "Rita", "customer": cust.Id, "active": true,
	})
	return cust.Id, staffRec, req
}

// A requester is always pinned to their own customer and always billable-only —
// the ?customer query param is a staff affordance and must not steer them.
func TestResolveTimeScopeRequesterPinned(t *testing.T) {
	app := testutil.SetupApp(t)
	custID, _, req := scopeGraph(t, app, true)

	scope, ok := timeentries.ResolveTimeScope(app, req, "some-other-customer-id")
	if !ok {
		t.Fatal("owning requester should be allowed when the opt-in is on")
	}
	if !scope.Enabled {
		t.Error("scope should be enabled when the customer opted in")
	}
	if scope.CustomerID != custID {
		t.Errorf("customer: got %q, want %q — the query param must not steer a requester", scope.CustomerID, custID)
	}
	if !scope.BillableOnly {
		t.Error("a requester must always get the billable-only figure")
	}
}

// Opting out is not a 403: the caller is allowed, the answer is just empty.
func TestResolveTimeScopeRequesterOptedOut(t *testing.T) {
	app := testutil.SetupApp(t)
	_, _, req := scopeGraph(t, app, false)

	scope, ok := timeentries.ResolveTimeScope(app, req, "")
	if !ok {
		t.Fatal("an opted-out requester should be allowed, not forbidden")
	}
	if scope.Enabled {
		t.Error("scope must be disabled when show_time_to_requester is off")
	}
}

func TestResolveTimeScopeStaffUnredacted(t *testing.T) {
	app := testutil.SetupApp(t)
	custID, staffRec, _ := scopeGraph(t, app, false) // opt-in off; staff unaffected

	scope, ok := timeentries.ResolveTimeScope(app, staffRec, custID)
	if !ok || !scope.Enabled {
		t.Fatal("staff should always be allowed and enabled")
	}
	if scope.BillableOnly {
		t.Error("staff get the full total, including written-off time")
	}
	if scope.CustomerID != custID {
		t.Errorf("staff customer scope: got %q, want %q", scope.CustomerID, custID)
	}
}

func TestResolveTimeScopeNilAuthForbidden(t *testing.T) {
	app := testutil.SetupApp(t)
	if _, ok := timeentries.ResolveTimeScope(app, nil, ""); ok {
		t.Error("an unauthenticated caller must be forbidden")
	}
}
