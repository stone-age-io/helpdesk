package maintenance

import (
	"testing"
	"time"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/core"

	"github.com/stone-age-io/helpdesk/internal/testutil"
	"github.com/stone-age-io/helpdesk/internal/tickets"
)

// Time is controlled two ways here: by writing dates into next_due, and by the
// explicit `now` Generate takes. That second one is why Generate has the
// parameter — there is no clock injection anywhere in this codebase, and a
// function whose whole job is date arithmetic would otherwise only be testable
// by backdating everything around it.

func setup(t *testing.T) (*pocketbase.PocketBase, *core.Record) {
	t.Helper()
	app := testutil.SetupApp(t)
	// The ticket create hook owns `number` and `status`; without it every
	// generated ticket gets number 0 and the second collides on the unique index.
	tickets.Register(app)
	Register(app)

	col, err := app.FindCollectionByNameOrId("customers")
	if err != nil {
		t.Fatalf("find customers: %v", err)
	}
	cust := core.NewRecord(col)
	cust.Set("name", "Acme Corp")
	cust.Set("active", true)
	if err := app.Save(cust); err != nil {
		t.Fatalf("save customer: %v", err)
	}
	return app, cust
}

// seedPlan creates a plan, defaulting the fields most tests don't care about.
func seedPlan(t *testing.T, app core.App, customer string, set map[string]any) *core.Record {
	t.Helper()
	col, err := app.FindCollectionByNameOrId("maintenance_plans")
	if err != nil {
		t.Fatalf("find maintenance_plans: %v", err)
	}
	rec := core.NewRecord(col)
	rec.Set("customer", customer)
	rec.Set("title", "Quarterly service")
	rec.Set("interval_days", 90)
	for k, v := range set {
		rec.Set(k, v)
	}
	if err := app.Save(rec); err != nil {
		t.Fatalf("save plan: %v", err)
	}
	return rec
}

func day(offset int) string {
	return time.Now().UTC().AddDate(0, 0, offset).Format(dateLayout)
}

func planTickets(t *testing.T, app core.App, planID string) []*core.Record {
	t.Helper()
	recs, err := app.FindRecordsByFilter(
		"tickets", "maintenance_plan = {:p}", "created", 0, 0, dbx.Params{"p": planID})
	if err != nil {
		t.Fatalf("query plan tickets: %v", err)
	}
	return recs
}

func reload(t *testing.T, app core.App, rec *core.Record) *core.Record {
	t.Helper()
	out, err := app.FindRecordById(rec.Collection().Name, rec.Id)
	if err != nil {
		t.Fatalf("reload %s: %v", rec.Collection().Name, err)
	}
	return out
}

// TestGenerateScheduleAnchor is the base case: a plan due today produces one
// ticket carrying the plan's triage, and the plan steps forward one interval.
func TestGenerateScheduleAnchor(t *testing.T) {
	app, cust := setup(t)

	staffCol, _ := app.FindCollectionByNameOrId("staff")
	tech := core.NewRecord(staffCol)
	tech.Set("name", "Sam Staff")
	tech.Set("email", "sam@msp.example")
	tech.Set("role", "field")
	tech.Set("active", true)
	tech.SetPassword("test-password-123")
	if err := app.Save(tech); err != nil {
		t.Fatalf("save staff: %v", err)
	}

	plan := seedPlan(t, app, cust.Id, map[string]any{
		"title":             "Replace door controller battery",
		"body":              "Annual battery swap.",
		"next_due":          day(0),
		"interval_days":     90,
		"assignee":          tech.Id,
		"priority":          "high",
		"estimated_minutes": 45,
	})

	created, skipped, err := Generate(app, time.Now().UTC())
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if created != 1 || skipped != 0 {
		t.Fatalf("created=%d skipped=%d, want 1/0", created, skipped)
	}

	got := planTickets(t, app, plan.Id)
	if len(got) != 1 {
		t.Fatalf("got %d tickets, want 1", len(got))
	}
	tk := got[0]

	if tk.GetString("title") != "Replace door controller battery" {
		t.Errorf("title = %q", tk.GetString("title"))
	}
	// Proactive work: `planned` also keeps it out of the awaiting_requester nag.
	if tk.GetString("type") != "planned" {
		t.Errorf("type = %q, want planned", tk.GetString("type"))
	}
	// Provenance — the scheduler must be distinguishable from a human at the desk.
	if tk.GetString("source") != "maintenance" {
		t.Errorf("source = %q, want maintenance", tk.GetString("source"))
	}
	if tk.GetString("priority") != "high" {
		t.Errorf("priority = %q, want high", tk.GetString("priority"))
	}
	if tk.GetString("assignee") != tech.Id {
		t.Errorf("assignee = %q, want the plan's", tk.GetString("assignee"))
	}
	if tk.GetInt("estimated_minutes") != 45 {
		t.Errorf("estimated_minutes = %d, want 45", tk.GetInt("estimated_minutes"))
	}
	// The whole reason due_at and plans shipped together.
	if d := tk.GetDateTime("due_at").Time().Format(dateLayout); d != day(0) {
		t.Errorf("due_at = %s, want %s", d, day(0))
	}
	// The create hook still owns these.
	if tk.GetString("status") != "open" {
		t.Errorf("status = %q, want open", tk.GetString("status"))
	}
	if tk.GetInt("number") == 0 {
		t.Error("number was never assigned")
	}

	if next := reload(t, app, plan).GetDateTime("next_due").Time().Format(dateLayout); next != day(90) {
		t.Errorf("next_due = %s, want %s", next, day(90))
	}
}

func TestGenerateSkipsNotYetDueAndPaused(t *testing.T) {
	app, cust := setup(t)

	future := seedPlan(t, app, cust.Id, map[string]any{"next_due": day(10)})
	paused := seedPlan(t, app, cust.Id, map[string]any{"next_due": day(-1), "paused": true})
	// A plan that has never been scheduled is parked, not due.
	unscheduled := seedPlan(t, app, cust.Id, nil)

	created, skipped, err := Generate(app, time.Now().UTC())
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if created != 0 || skipped != 0 {
		t.Fatalf("created=%d skipped=%d, want 0/0", created, skipped)
	}
	for name, p := range map[string]*core.Record{
		"future": future, "paused": paused, "unscheduled": unscheduled,
	} {
		if n := len(planTickets(t, app, p.Id)); n != 0 {
			t.Errorf("%s plan generated %d tickets", name, n)
		}
	}
	// A paused plan must not silently drift forward either — un-pausing it
	// should resume from where it was, not from wherever the cron walked it.
	if next := reload(t, app, paused).GetDateTime("next_due").Time().Format(dateLayout); next != day(-1) {
		t.Errorf("paused plan next_due moved to %s", next)
	}
}

// TestGenerateLeadTime: work should be on the board before it is late.
func TestGenerateLeadTime(t *testing.T) {
	app, cust := setup(t)
	plan := seedPlan(t, app, cust.Id, map[string]any{
		"next_due": day(5), "lead_time_days": 7, "interval_days": 30,
	})

	created, _, err := Generate(app, time.Now().UTC())
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if created != 1 {
		t.Fatalf("created = %d, want 1 (5 days out, 7 days lead)", created)
	}
	// The ticket is due when the work is due, not when it was generated.
	tk := planTickets(t, app, plan.Id)[0]
	if d := tk.GetDateTime("due_at").Time().Format(dateLayout); d != day(5) {
		t.Errorf("due_at = %s, want %s", d, day(5))
	}
	if next := reload(t, app, plan).GetDateTime("next_due").Time().Format(dateLayout); next != day(35) {
		t.Errorf("next_due = %s, want %s", next, day(35))
	}
}

// TestGenerateDormantPlanDoesNotBacklog: a plan nobody ran for over a year
// should produce ONE ticket on the next real slot, not a year of them.
func TestGenerateDormantPlanDoesNotBacklog(t *testing.T) {
	app, cust := setup(t)
	plan := seedPlan(t, app, cust.Id, map[string]any{
		"next_due": day(-400), "interval_days": 90,
	})

	created, _, err := Generate(app, time.Now().UTC())
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if created != 1 {
		t.Fatalf("created = %d, want 1", created)
	}
	next := reload(t, app, plan).GetDateTime("next_due").Time()
	if !next.After(time.Now().UTC()) {
		t.Errorf("next_due = %s, want a future date", next.Format(dateLayout))
	}
}

// TestGenerateIsIdempotent: the dedupe key means a cron firing twice, or an
// operator running maintenance-run beside it, costs nothing.
func TestGenerateIdempotentPerOccurrence(t *testing.T) {
	app, cust := setup(t)
	plan := seedPlan(t, app, cust.Id, map[string]any{"next_due": day(0), "interval_days": 90})

	if _, _, err := Generate(app, time.Now().UTC()); err != nil {
		t.Fatalf("first generate: %v", err)
	}
	// Wind the plan back to the same occurrence, as a crash between the ticket
	// save and the advance would leave it.
	p := reload(t, app, plan)
	p.Set("next_due", day(0))
	if err := app.Save(p); err != nil {
		t.Fatalf("rewind plan: %v", err)
	}

	created, _, err := Generate(app, time.Now().UTC())
	if err != nil {
		t.Fatalf("second generate: %v", err)
	}
	if created != 0 {
		t.Errorf("created = %d on re-run, want 0", created)
	}
	if n := len(planTickets(t, app, plan.Id)); n != 1 {
		t.Errorf("got %d tickets for one occurrence, want 1", n)
	}
	// It still advances, so the rewound plan doesn't re-run forever.
	if next := reload(t, app, plan).GetDateTime("next_due").Time().Format(dateLayout); next != day(90) {
		t.Errorf("next_due = %s, want %s", next, day(90))
	}
}

// TestGenerateSkipsWhenPreviousStillOpen: one quarterly inspection nobody did
// must not become four open tickets — but the plan still tracks the calendar.
func TestGenerateSkipsWhenPreviousStillOpen(t *testing.T) {
	app, cust := setup(t)
	plan := seedPlan(t, app, cust.Id, map[string]any{"next_due": day(0), "interval_days": 30})

	if _, _, err := Generate(app, time.Now().UTC()); err != nil {
		t.Fatalf("first generate: %v", err)
	}

	// Wind to a DIFFERENT occurrence with the first ticket still open, so that
	// what stops the second ticket is the skip guard and not the dedupe key.
	p := reload(t, app, plan)
	p.Set("next_due", day(-1))
	if err := app.Save(p); err != nil {
		t.Fatalf("rewind plan: %v", err)
	}

	created, skipped, err := Generate(app, time.Now().UTC())
	if err != nil {
		t.Fatalf("second generate: %v", err)
	}
	if created != 0 || skipped != 1 {
		t.Errorf("created=%d skipped=%d, want 0/1", created, skipped)
	}
	if n := len(planTickets(t, app, plan.Id)); n != 1 {
		t.Errorf("got %d tickets, want 1 — the skip should not have created one", n)
	}
	if next := reload(t, app, plan).GetDateTime("next_due").Time().Format(dateLayout); next != day(29) {
		t.Errorf("next_due = %s, want %s — a skip still advances", next, day(29))
	}

	// Resolving the open one lets the next occurrence through.
	tk := planTickets(t, app, plan.Id)[0]
	tk.Set("status", "resolved")
	if err := app.Save(tk); err != nil {
		t.Fatalf("resolve: %v", err)
	}
	p = reload(t, app, plan)
	p.Set("next_due", day(-2)) // a third, still-ungenerated occurrence
	if err := app.Save(p); err != nil {
		t.Fatalf("rewind plan: %v", err)
	}
	created, skipped, err = Generate(app, time.Now().UTC())
	if err != nil {
		t.Fatalf("third generate: %v", err)
	}
	if created != 1 || skipped != 0 {
		t.Errorf("after resolving: created=%d skipped=%d, want 1/0", created, skipped)
	}
}

// TestCompletionAnchorParksThenResumes is the second anchor end to end.
func TestCompletionAnchorParksThenResumes(t *testing.T) {
	app, cust := setup(t)
	plan := seedPlan(t, app, cust.Id, map[string]any{
		"next_due": day(0), "interval_days": 90, "anchor": "completion",
	})

	created, _, err := Generate(app, time.Now().UTC())
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if created != 1 {
		t.Fatalf("created = %d, want 1", created)
	}

	// Parked: an empty next_due is invisible to the cron's filter, which is what
	// makes a completion-anchored plan structurally unable to stack up work.
	if !reload(t, app, plan).GetDateTime("next_due").IsZero() {
		t.Fatal("completion-anchored plan should be parked (empty next_due) after generating")
	}
	if n, _, _ := Generate(app, time.Now().UTC()); n != 0 {
		t.Errorf("parked plan generated %d tickets, want 0", n)
	}

	// Resolving restarts the clock from when the work was done.
	tk := planTickets(t, app, plan.Id)[0]
	tk.Set("status", "resolved")
	if err := app.Save(tk); err != nil {
		t.Fatalf("resolve: %v", err)
	}
	next := reload(t, app, plan).GetDateTime("next_due").Time()
	if next.Format(dateLayout) != day(90) {
		t.Errorf("next_due = %s, want %s", next.Format(dateLayout), day(90))
	}

	// The usual resolved → closed journey must NOT move it again: the resolve
	// already restarted the clock, and closing clears resolved_at.
	tk = reload(t, app, tk)
	tk.Set("status", "closed")
	if err := app.Save(tk); err != nil {
		t.Fatalf("close: %v", err)
	}
	if again := reload(t, app, plan).GetDateTime("next_due").Time().Format(dateLayout); again != day(90) {
		t.Errorf("closing after resolve moved next_due to %s, want it unchanged at %s", again, day(90))
	}
}

// TestCompletionAnchorClosedWithoutResolving covers the fallback: a ticket
// closed straight from open has no resolved_at, so the clock starts now.
func TestCompletionAnchorClosedWithoutResolving(t *testing.T) {
	app, cust := setup(t)
	plan := seedPlan(t, app, cust.Id, map[string]any{
		"next_due": day(0), "interval_days": 30, "anchor": "completion",
	})
	if _, _, err := Generate(app, time.Now().UTC()); err != nil {
		t.Fatalf("generate: %v", err)
	}

	tk := planTickets(t, app, plan.Id)[0]
	tk.Set("status", "closed")
	if err := app.Save(tk); err != nil {
		t.Fatalf("close: %v", err)
	}
	if next := reload(t, app, plan).GetDateTime("next_due").Time().Format(dateLayout); next != day(30) {
		t.Errorf("next_due = %s, want %s", next, day(30))
	}
}

// TestScheduleAnchorIgnoresResolution: the resume hook must not touch a
// schedule-anchored plan, or the cron and the hook would fight over next_due.
func TestScheduleAnchorIgnoresResolution(t *testing.T) {
	app, cust := setup(t)
	plan := seedPlan(t, app, cust.Id, map[string]any{
		"next_due": day(0), "interval_days": 90, "anchor": "schedule",
	})
	if _, _, err := Generate(app, time.Now().UTC()); err != nil {
		t.Fatalf("generate: %v", err)
	}

	tk := planTickets(t, app, plan.Id)[0]
	tk.Set("status", "resolved")
	if err := app.Save(tk); err != nil {
		t.Fatalf("resolve: %v", err)
	}
	// Still the cron's number, untouched by the resolve.
	if next := reload(t, app, plan).GetDateTime("next_due").Time().Format(dateLayout); next != day(90) {
		t.Errorf("next_due = %s, want %s — the hook must leave schedule plans alone", next, day(90))
	}
}

func TestPlanCreateDefaults(t *testing.T) {
	app, cust := setup(t)
	plan := seedPlan(t, app, cust.Id, nil)

	if got := plan.GetString("anchor"); got != "schedule" {
		t.Errorf("anchor = %q, want the schedule default", got)
	}
	if got := plan.GetString("priority"); got != "normal" {
		t.Errorf("priority = %q, want normal", got)
	}

	// "Only if empty" — an explicit value must survive.
	explicit := seedPlan(t, app, cust.Id, map[string]any{"anchor": "completion", "priority": "urgent"})
	if explicit.GetString("anchor") != "completion" || explicit.GetString("priority") != "urgent" {
		t.Error("create hook overwrote explicit anchor/priority")
	}
}
