package timers

import (
	"testing"
	"time"

	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/types"

	"github.com/stone-age-io/helpdesk/internal/testutil"
	"github.com/stone-age-io/helpdesk/internal/visits"
)

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

type fixtures struct {
	customer *core.Record
	staff    *core.Record
	ticket   *core.Record
}

// setup boots a real app with BOTH the timer create hook and the visits guard
// registered — the guard is what stamps completed_at when a stop completes a
// visit. No notifier is wired, so nothing here sends mail (time_entries saves
// never do, and visit completion is silent anyway).
func setup(t *testing.T) (*pocketbase.PocketBase, *fixtures) {
	t.Helper()
	app := testutil.SetupApp(t)
	Register(app)
	visits.Register(app)

	customer := seedRecord(t, app, "customers", map[string]any{"name": "Acme", "active": true})
	staff := seedRecord(t, app, "staff", map[string]any{
		"email": "sam@816tech.example", "password": "secret123456",
		"name": "Sam", "role": "agent", "active": true,
	})
	// tickets.Register isn't loaded here; set number manually for the unique index.
	ticket := seedRecord(t, app, "tickets", map[string]any{
		"customer": customer.Id, "title": "pump", "number": 1,
	})
	return app, &fixtures{customer: customer, staff: staff, ticket: ticket}
}

func TestStartedAtStampedServerSide(t *testing.T) {
	app, f := setup(t)
	col, _ := app.FindCollectionByNameOrId("time_sessions")
	rec := core.NewRecord(col)
	rec.Set("staff", f.staff.Id)
	rec.Set("ticket", f.ticket.Id)
	// Client tries to backdate the start; the hook must overwrite it.
	rec.Set("started_at", "2000-01-01 00:00:00.000Z")
	if err := app.Save(rec); err != nil {
		t.Fatalf("save session: %v", err)
	}
	if since := time.Since(rec.GetDateTime("started_at").Time()); since > time.Minute {
		t.Errorf("started_at not server-stamped: %v ago", since)
	}
}

func TestOneOpenTimerPerAgent(t *testing.T) {
	app, f := setup(t)
	seedRecord(t, app, "time_sessions", map[string]any{"staff": f.staff.Id, "ticket": f.ticket.Id})

	// A second open timer for the same agent violates the unique index.
	col, _ := app.FindCollectionByNameOrId("time_sessions")
	rec := core.NewRecord(col)
	rec.Set("staff", f.staff.Id)
	rec.Set("ticket", f.ticket.Id)
	if err := app.Save(rec); err == nil {
		t.Error("a second open timer for the same agent should be rejected")
	}
}

func TestStopCreatesEntryAndDeletesSession(t *testing.T) {
	app, f := setup(t)
	session := seedRecord(t, app, "time_sessions", map[string]any{
		"staff": f.staff.Id, "ticket": f.ticket.Id, "note": "wired the panel",
	})

	entry, err := Stop(app, session, StopOpts{Minutes: 45})
	if err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if got := entry.GetInt("minutes"); got != 45 {
		t.Errorf("minutes: got %d, want 45", got)
	}
	if entry.GetString("ticket") != f.ticket.Id {
		t.Error("entry not keyed to the session's ticket")
	}
	if entry.GetString("staff") != f.staff.Id {
		t.Error("entry not attributed to the session's staff")
	}
	if got := entry.GetString("note"); got != "wired the panel" {
		t.Errorf("session note not carried onto entry: %q", got)
	}
	if _, err := app.FindRecordById("time_sessions", session.Id); err == nil {
		t.Error("session should be deleted after stop")
	}
}

func TestStopRoundsElapsedToNearest5(t *testing.T) {
	app, f := setup(t)
	session := seedRecord(t, app, "time_sessions", map[string]any{"staff": f.staff.Id, "ticket": f.ticket.Id})

	// Rewind the server-stamped start ~12 minutes; an update doesn't re-stamp.
	past, err := types.ParseDateTime(time.Now().Add(-12 * time.Minute))
	if err != nil {
		t.Fatalf("parse time: %v", err)
	}
	session.Set("started_at", past)
	if err := app.Save(session); err != nil {
		t.Fatalf("rewind started_at: %v", err)
	}

	entry, err := Stop(app, session, StopOpts{}) // no override → compute + round
	if err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if got := entry.GetInt("minutes"); got != 10 {
		t.Errorf("elapsed rounding: got %d, want 10 (12min → nearest 5)", got)
	}
}

func TestStopCompletesVisit(t *testing.T) {
	app, f := setup(t)
	visit := seedRecord(t, app, "visits", map[string]any{
		"ticket": f.ticket.Id, "assignee": f.staff.Id, "status": "scheduled",
		"scheduled_at": "2026-07-14 14:00:00.000Z",
	})
	session := seedRecord(t, app, "time_sessions", map[string]any{
		"staff": f.staff.Id, "ticket": f.ticket.Id, "visit": visit.Id,
	})

	entry, err := Stop(app, session, StopOpts{Minutes: 30, CompleteVisit: true})
	if err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if entry.GetString("visit") != visit.Id {
		t.Error("entry should be tagged to the visit")
	}

	got, err := app.FindRecordById("visits", visit.Id)
	if err != nil {
		t.Fatalf("reload visit: %v", err)
	}
	if got.GetString("status") != "completed" {
		t.Errorf("visit status: got %q, want completed", got.GetString("status"))
	}
	if got.GetDateTime("completed_at").IsZero() {
		t.Error("completed_at should be stamped by the visits guard")
	}
}

func TestOwnerOrAdmin(t *testing.T) {
	app, f := setup(t)
	other := seedRecord(t, app, "staff", map[string]any{
		"email": "amy@816tech.example", "password": "secret123456",
		"name": "Amy", "role": "agent", "active": true,
	})
	admin := seedRecord(t, app, "staff", map[string]any{
		"email": "boss@816tech.example", "password": "secret123456",
		"name": "Boss", "role": "admin", "active": true,
	})
	requester := seedRecord(t, app, "users", map[string]any{
		"email": "rita@acme.example", "password": "secret123456",
		"name": "Rita", "customer": f.customer.Id, "active": true,
	})
	session := seedRecord(t, app, "time_sessions", map[string]any{"staff": f.staff.Id, "ticket": f.ticket.Id})

	if !ownerOrAdmin(f.staff, session) {
		t.Error("owner should be allowed")
	}
	if ownerOrAdmin(other, session) {
		t.Error("another agent must not stop someone else's timer")
	}
	if !ownerOrAdmin(admin, session) {
		t.Error("admin should be allowed to clear a stuck timer")
	}
	if ownerOrAdmin(requester, session) {
		t.Error("a requester must never pass")
	}
	if ownerOrAdmin(nil, session) {
		t.Error("nil auth must be denied")
	}
}
