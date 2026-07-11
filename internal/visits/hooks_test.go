package visits

import (
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase/core"

	"github.com/stone-age-io/helpdesk/internal/testutil"
)

type fixtures struct {
	customer *core.Record
	tech     *core.Record
	ticket   *core.Record
}

func seedRecord(t *testing.T, app core.App, collection string, fields map[string]any) *core.Record {
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

// setup boots a real app with the guard registered and seeds the minimum
// graph a visit needs: customer → ticket, plus one technician.
func setup(t *testing.T) (core.App, *fixtures) {
	t.Helper()
	app := testutil.SetupApp(t)
	Register(app)

	customer := seedRecord(t, app, "customers", map[string]any{"name": "Acme", "active": true})
	tech := seedRecord(t, app, "staff", map[string]any{
		"email": "sam@816tech.example", "password": "secret123456",
		"name": "Sam Staff", "role": "agent", "active": true,
	})
	// tickets.Register isn't loaded here; set number manually to satisfy
	// the unique index.
	ticket := seedRecord(t, app, "tickets", map[string]any{
		"customer": customer.Id, "title": "pump fault", "number": 1,
	})
	return app, &fixtures{customer: customer, tech: tech, ticket: ticket}
}

func newVisit(t *testing.T, app core.App, fields map[string]any) (*core.Record, error) {
	t.Helper()
	col, err := app.FindCollectionByNameOrId("visits")
	if err != nil {
		t.Fatalf("find visits: %v", err)
	}
	rec := core.NewRecord(col)
	for k, v := range fields {
		rec.Set(k, v)
	}
	return rec, app.Save(rec)
}

func TestEmptyStatusDefaultsToRequested(t *testing.T) {
	app, f := setup(t)
	rec, err := newVisit(t, app, map[string]any{"ticket": f.ticket.Id})
	if err != nil {
		t.Fatalf("save visit: %v", err)
	}
	if got := rec.GetString("status"); got != "requested" {
		t.Errorf("status: got %q, want requested", got)
	}
}

func TestEmptyStatusDefaultsToScheduledWhenTimeSet(t *testing.T) {
	app, f := setup(t)
	rec, err := newVisit(t, app, map[string]any{
		"ticket": f.ticket.Id, "assignee": f.tech.Id,
		"scheduled_at": "2026-07-14 14:00:00.000Z",
	})
	if err != nil {
		t.Fatalf("save visit: %v", err)
	}
	if got := rec.GetString("status"); got != "scheduled" {
		t.Errorf("status: got %q, want scheduled", got)
	}
}

func TestScheduledWithoutTimeRejected(t *testing.T) {
	app, f := setup(t)
	_, err := newVisit(t, app, map[string]any{
		"ticket": f.ticket.Id, "assignee": f.tech.Id, "status": "scheduled",
	})
	if err == nil || !strings.Contains(err.Error(), "time and a technician") {
		t.Errorf("want guard rejection, got %v", err)
	}
}

func TestScheduledWithoutAssigneeRejected(t *testing.T) {
	app, f := setup(t)
	_, err := newVisit(t, app, map[string]any{
		"ticket": f.ticket.Id, "status": "scheduled",
		"scheduled_at": "2026-07-14 14:00:00.000Z",
	})
	if err == nil || !strings.Contains(err.Error(), "time and a technician") {
		t.Errorf("want guard rejection, got %v", err)
	}
}

func TestRequestedToScheduledRequiresTimeAndTech(t *testing.T) {
	app, f := setup(t)
	rec, err := newVisit(t, app, map[string]any{"ticket": f.ticket.Id, "status": "requested"})
	if err != nil {
		t.Fatalf("save requested visit: %v", err)
	}

	rec.Set("status", "scheduled")
	if err := app.Save(rec); err == nil {
		t.Error("scheduling without time+tech should be rejected")
	}

	rec.Set("scheduled_at", "2026-07-14 14:00:00.000Z")
	rec.Set("assignee", f.tech.Id)
	if err := app.Save(rec); err != nil {
		t.Errorf("scheduling with time+tech: %v", err)
	}
}

func TestCompletedStampsCompletedAt(t *testing.T) {
	app, f := setup(t)
	rec, err := newVisit(t, app, map[string]any{
		"ticket": f.ticket.Id, "assignee": f.tech.Id, "status": "scheduled",
		"scheduled_at": "2026-07-14 14:00:00.000Z",
	})
	if err != nil {
		t.Fatalf("save scheduled visit: %v", err)
	}
	if !rec.GetDateTime("completed_at").IsZero() {
		t.Error("completed_at should be empty on a scheduled visit")
	}

	rec.Set("status", "completed")
	if err := app.Save(rec); err != nil {
		t.Fatalf("complete visit: %v", err)
	}
	if rec.GetDateTime("completed_at").IsZero() {
		t.Error("completed_at should be stamped when a visit is completed")
	}
}

func TestCompletedAtPreservedWhenSupplied(t *testing.T) {
	app, f := setup(t)
	// A visit closed out after the fact carries its real completion time.
	rec, err := newVisit(t, app, map[string]any{
		"ticket": f.ticket.Id, "assignee": f.tech.Id, "status": "completed",
		"completed_at": "2026-07-10 09:30:00.000Z",
	})
	if err != nil {
		t.Fatalf("save completed visit: %v", err)
	}
	if got := rec.GetDateTime("completed_at").String(); got[:10] != "2026-07-10" {
		t.Errorf("completed_at overwritten: got %q, want the supplied 2026-07-10", got)
	}
}

func TestCompletedAtClearedWhenReopened(t *testing.T) {
	app, f := setup(t)
	rec, err := newVisit(t, app, map[string]any{
		"ticket": f.ticket.Id, "assignee": f.tech.Id, "status": "completed",
	})
	if err != nil {
		t.Fatalf("save completed visit: %v", err)
	}
	if rec.GetDateTime("completed_at").IsZero() {
		t.Fatal("precondition: completed_at should be stamped")
	}

	rec.Set("status", "scheduled")
	rec.Set("scheduled_at", "2026-07-20 10:00:00.000Z")
	if err := app.Save(rec); err != nil {
		t.Fatalf("reschedule visit: %v", err)
	}
	if !rec.GetDateTime("completed_at").IsZero() {
		t.Error("completed_at should clear when a visit leaves completed")
	}
}

// TestPortalVisitReadRule exercises the migration's relation-hop read rule
// directly (no HTTP): a requester sees visits on their own company's
// tickets and nothing else.
func TestPortalVisitReadRule(t *testing.T) {
	app, f := setup(t)

	otherCustomer := seedRecord(t, app, "customers", map[string]any{"name": "Globex", "active": true})
	requesterA := seedRecord(t, app, "users", map[string]any{
		"email": "rita@acme.example", "password": "secret123456",
		"name": "Rita", "customer": f.customer.Id, "active": true,
	})
	requesterB := seedRecord(t, app, "users", map[string]any{
		"email": "gus@globex.example", "password": "secret123456",
		"name": "Gus", "customer": otherCustomer.Id, "active": true,
	})

	visit, err := newVisit(t, app, map[string]any{"ticket": f.ticket.Id, "status": "requested"})
	if err != nil {
		t.Fatalf("save visit: %v", err)
	}

	col, _ := app.FindCollectionByNameOrId("visits")
	if col.ViewRule == nil {
		t.Fatal("visits ViewRule is nil")
	}

	canA, err := app.CanAccessRecord(visit, &core.RequestInfo{Auth: requesterA}, col.ViewRule)
	if err != nil {
		t.Fatalf("CanAccessRecord(requesterA): %v", err)
	}
	if !canA {
		t.Error("requester of the ticket's customer should see the visit")
	}

	canB, err := app.CanAccessRecord(visit, &core.RequestInfo{Auth: requesterB}, col.ViewRule)
	if err != nil {
		t.Fatalf("CanAccessRecord(requesterB): %v", err)
	}
	if canB {
		t.Error("requester of another customer must not see the visit")
	}
}
