// Package maintenance owns preventive-maintenance recurrence: turning a
// maintenance_plan into ordinary tickets on a schedule, and advancing the plan
// afterwards.
//
// It is a planning layer above the existing ledger, exactly like
// internal/projects — the only thing it produces is a ticket, and everything
// downstream (visits, time entries, the audit trail, notifications, reports)
// works on that ticket with no knowledge of this package. See migration
// 1829000000 for the schema and the two-anchor rationale.
package maintenance

import (
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
)

// dateLayout is the day-granularity form used for a plan's next_due and a
// ticket's due_at. Both are calendar dates, not instants — "due the 14th" means
// the same thing in every timezone the crew works in.
const dateLayout = "2006-01-02"

// Generate materializes a ticket for every plan that has come due, and returns
// (created, skipped, error). `skipped` counts plans that were due but whose
// previous ticket is still open.
//
// `now` is a parameter rather than a time.Now() call because this codebase has
// no clock injection anywhere (tests control time by writing old timestamps
// into records), and a function whose entire job is date arithmetic deserves
// the one argument that makes it directly drivable. Exported and broker-free
// for the same reason tickets.AutoCloseResolved is — cmd/helpdesk wires it to a
// daily cron and to the `maintenance-run` subcommand.
//
// Best-effort per plan: one bad plan never aborts the sweep.
func Generate(app core.App, now time.Time) (int, int, error) {
	// Non-paused plans that are actually scheduled. A completion-anchored plan
	// waiting on its open ticket has an empty next_due and is invisible here,
	// which is precisely what makes it unable to stack up work.
	//
	// The "has it come due" test is deliberately NOT in this filter: lead time
	// is per-plan and PocketBase's filter DSL has no date arithmetic. Plans
	// number in the tens even for a large desk, so scanning them in Go is the
	// right trade against building a second horizon column to keep in sync.
	plans, err := app.FindRecordsByFilter(
		"maintenance_plans",
		"paused = false && next_due != ''",
		"next_due", 0, 0, nil,
	)
	if err != nil {
		return 0, 0, err
	}

	created, skipped := 0, 0
	for _, plan := range plans {
		due := plan.GetDateTime("next_due").Time()
		interval := plan.GetInt("interval_days")
		if interval < 1 {
			slog.Warn("maintenance: plan has no interval, skipping", "plan", plan.Id)
			continue
		}
		// Generate `lead_time_days` early so the work is on the board before
		// it is late.
		if now.Before(due.AddDate(0, 0, -plan.GetInt("lead_time_days"))) {
			continue
		}

		completionAnchored := plan.GetString("anchor") == "completion"

		// Skip-if-still-open, schedule-anchored only. A completion-anchored plan
		// cannot reach here with an open ticket (it would be parked), so asking
		// would be a query that can never return anything.
		if !completionAnchored {
			open, err := app.FindRecordsByFilter(
				"tickets",
				"maintenance_plan = {:plan} && status != 'resolved' && status != 'closed'",
				"", 1, 0,
				dbx.Params{"plan": plan.Id},
			)
			if err == nil && len(open) > 0 {
				// Advance anyway: one quarterly inspection nobody did should not
				// become four open tickets, and a plan that never advances falls
				// permanently behind the calendar it is supposed to track.
				slog.Info("maintenance: previous ticket still open, skipping generation",
					"plan", plan.Id, "ticket", open[0].Id, "due", due.Format(dateLayout))
				if err := advance(app, plan, due, interval, now); err != nil {
					slog.Warn("maintenance: advance failed", "plan", plan.Id, "err", err)
				}
				skipped++
				continue
			}
		}

		madeOne, err := createTicket(app, plan, due)
		if err != nil {
			slog.Warn("maintenance: ticket creation failed", "plan", plan.Id, "err", err)
			continue
		}
		if madeOne {
			created++
		}
		// A completion-anchored plan waits for its ticket; a schedule-anchored
		// one marches on regardless.
		var advErr error
		if completionAnchored {
			advErr = park(app, plan)
		} else {
			advErr = advance(app, plan, due, interval, now)
		}
		if advErr != nil {
			slog.Warn("maintenance: advance failed", "plan", plan.Id, "err", advErr)
		}
	}
	return created, skipped, nil
}

// createTicket projects one occurrence of a plan into a ticket. Returns false
// when the occurrence already had one (a re-run on the same day, or a crash
// between the save and the advance).
//
// Notifications are deliberately NOT suppressed. This is the opposite call from
// tickets.AutoCloseResolved, and for the opposite reason: auto-close is an
// administrative tidy-up nobody needs to hear about, while a newly opened
// preventive ticket is real news for whoever has to do it. A plan has no
// requester, so ticket.created's requester recipient resolves empty exactly as
// it does for a machine ticket, and only staff are mailed.
func createTicket(app core.App, plan *core.Record, due time.Time) (bool, error) {
	// pm:{planID}:{occurrence} rides the existing unique partial index on
	// tickets.dedupe_key — the same idempotency the NATS and webhook intakes
	// use, so a cron that fires twice (or a manual maintenance-run beside it)
	// costs nothing.
	dedupe := fmt.Sprintf("pm:%s:%s", plan.Id, due.Format(dateLayout))
	if existing, err := app.FindFirstRecordByFilter(
		"tickets", "dedupe_key = {:k}", dbx.Params{"k": dedupe},
	); err == nil && existing != nil {
		return false, nil
	}

	col, err := app.FindCollectionByNameOrId("tickets")
	if err != nil {
		return false, err
	}

	rec := core.NewRecord(col)
	rec.Set("customer", plan.GetString("customer"))
	rec.Set("title", plan.GetString("title"))
	rec.Set("body", plan.GetString("body"))
	rec.Set("category", plan.GetString("category"))
	rec.Set("assignee", plan.GetString("assignee"))
	rec.Set("priority", plan.GetString("priority"))
	rec.Set("thing", plan.GetString("thing"))
	rec.Set("location", plan.GetString("location"))
	rec.Set("project", plan.GetString("project"))
	if est := plan.GetInt("estimated_minutes"); est > 0 {
		rec.Set("estimated_minutes", est)
	}
	// Proactive work, so `planned` — which also keeps it out of the
	// awaiting_requester nag (migration 1818000000), since preventive service
	// is not a reply-driven conversation.
	rec.Set("type", "planned")
	rec.Set("source", "maintenance")
	rec.Set("due_at", due.Format(dateLayout))
	rec.Set("maintenance_plan", plan.Id)
	rec.Set("dedupe_key", dedupe)
	// `status` and `number` are deliberately left to the tickets create hook.

	if err := app.Save(rec); err != nil {
		if isUniqueViolation(err) {
			// Lost a race with a concurrent run; the ticket exists either way.
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// advance steps a schedule-anchored plan past the occurrence just handled: ONE
// whole interval unconditionally — the occurrence at `due` is done with, whether
// it generated a ticket or was skipped — then onward until it lands in the
// future, so a plan left dormant for a year produces one ticket on the next real
// slot rather than a year of backlog.
//
// The unconditional first step is what makes lead time work. Generating a week
// early leaves `due` still in the future, and a loop that only ran while `due`
// was past would leave next_due pointing at the occurrence it had just
// generated — re-checking it, and dedupe-rejecting it, every night until the
// date finally arrived.
func advance(app core.App, plan *core.Record, due time.Time, interval int, now time.Time) error {
	next := due.AddDate(0, 0, interval)
	for !next.After(now) {
		next = next.AddDate(0, 0, interval)
	}
	plan.Set("next_due", next.Format(dateLayout))
	return app.Save(plan)
}

// park clears next_due, which is how a completion-anchored plan waits for its
// ticket. The generator's `next_due != ”` filter then skips it entirely, and
// resumePlan in hooks.go sets the next date when the work resolves.
func park(app core.App, plan *core.Record) error {
	plan.Set("next_due", "")
	return app.Save(plan)
}

// isUniqueViolation detects a unique-index collision from PocketBase's error
// text. Copied rather than shared, following the note in internal/ingest: a
// package holding one eight-line function costs more than the copy.
func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "unique") || strings.Contains(msg, "constraint")
}
