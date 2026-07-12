package migrations_test

import (
	"testing"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"

	"github.com/stone-age-io/helpdesk/internal/testutil"
)

// These exercise the 1809000000 field-service migration: the two nullable
// additions and, crucially, the data-model invariant behind change #1 — the
// ticket stays the canonical labor ledger, the visit is only a dimension.

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

func sumMinutes(t *testing.T, app core.App, filter string, params dbx.Params) int {
	t.Helper()
	recs, err := app.FindRecordsByFilter("time_entries", filter, "", 0, 0, params)
	if err != nil {
		t.Fatalf("query time_entries: %v", err)
	}
	total := 0
	for _, r := range recs {
		total += r.GetInt("minutes")
	}
	return total
}

// fieldServiceGraph seeds customer → ticket, a technician, and one scheduled
// visit, returning the ids the tests build on.
func fieldServiceGraph(t *testing.T, app core.App) (ticket, tech, visit *core.Record) {
	t.Helper()
	customer := seed(t, app, "customers", map[string]any{"name": "Acme", "active": true})
	tech = seed(t, app, "staff", map[string]any{
		"email": "sam@816tech.example", "password": "secret123456",
		"name": "Sam Staff", "role": "agent", "active": true,
	})
	ticket = seed(t, app, "tickets", map[string]any{
		"customer": customer.Id, "title": "pump fault", "number": 1,
	})
	visit = seed(t, app, "visits", map[string]any{
		"ticket": ticket.Id, "assignee": tech.Id, "status": "scheduled",
		"scheduled_at": "2026-07-14 14:00:00.000Z", "duration_minutes": 120,
	})
	return ticket, tech, visit
}

func TestFieldServiceFieldsExist(t *testing.T) {
	app := testutil.SetupApp(t)

	visits, err := app.FindCollectionByNameOrId("visits")
	if err != nil {
		t.Fatalf("find visits: %v", err)
	}
	if visits.Fields.GetByName("duration_minutes") == nil {
		t.Error("visits.duration_minutes field missing")
	}

	entries, err := app.FindCollectionByNameOrId("time_entries")
	if err != nil {
		t.Fatalf("find time_entries: %v", err)
	}
	if entries.Fields.GetByName("visit") == nil {
		t.Error("time_entries.visit field missing")
	}
}

func TestVisitCarriesDuration(t *testing.T) {
	app := testutil.SetupApp(t)
	_, _, visit := fieldServiceGraph(t, app)
	if got := visit.GetInt("duration_minutes"); got != 120 {
		t.Errorf("duration_minutes: got %d, want 120", got)
	}
}

// The ledger invariant: field time is tagged to the visit AND still counts
// toward the ticket total, while desk time (no visit) counts only toward the
// ticket. No rollup logic — both totals are plain filters over one table.
func TestTimeRollsUpToTicketAndVisit(t *testing.T) {
	app := testutil.SetupApp(t)
	ticket, tech, visit := fieldServiceGraph(t, app)

	// 90 min on-site (tagged to the visit) + 30 min desk work (untagged).
	seed(t, app, "time_entries", map[string]any{
		"ticket": ticket.Id, "staff": tech.Id, "minutes": 90,
		"work_date": "2026-07-14 16:00:00.000Z", "visit": visit.Id,
	})
	seed(t, app, "time_entries", map[string]any{
		"ticket": ticket.Id, "staff": tech.Id, "minutes": 30,
		"work_date": "2026-07-15 09:00:00.000Z",
	})

	if got := sumMinutes(t, app, "ticket = {:t}", dbx.Params{"t": ticket.Id}); got != 120 {
		t.Errorf("ticket total: got %d, want 120 (all labor rolls up to the ticket)", got)
	}
	if got := sumMinutes(t, app, "visit = {:v}", dbx.Params{"v": visit.Id}); got != 90 {
		t.Errorf("visit total: got %d, want 90 (only field time)", got)
	}
	if got := sumMinutes(t, app, "ticket = {:t} && visit = ''", dbx.Params{"t": ticket.Id}); got != 30 {
		t.Errorf("desk total: got %d, want 30 (untagged time)", got)
	}
}

// Labor is real: deleting a visit must never delete the time logged against
// it. The entry survives with its ticket relation intact (no cascade on the
// visit FK), so the ticket total is unaffected.
func TestDeletingVisitKeepsTimeEntries(t *testing.T) {
	app := testutil.SetupApp(t)
	ticket, tech, visit := fieldServiceGraph(t, app)

	entry := seed(t, app, "time_entries", map[string]any{
		"ticket": ticket.Id, "staff": tech.Id, "minutes": 45,
		"work_date": "2026-07-14 16:00:00.000Z", "visit": visit.Id,
	})

	if err := app.Delete(visit); err != nil {
		t.Fatalf("delete visit: %v", err)
	}

	survivor, err := app.FindRecordById("time_entries", entry.Id)
	if err != nil {
		t.Fatalf("time entry should survive visit deletion: %v", err)
	}
	if got := survivor.GetString("ticket"); got != ticket.Id {
		t.Errorf("ticket relation lost after visit delete: got %q, want %q", got, ticket.Id)
	}
	if got := sumMinutes(t, app, "ticket = {:t}", dbx.Params{"t": ticket.Id}); got != 45 {
		t.Errorf("ticket total after visit delete: got %d, want 45", got)
	}
}
