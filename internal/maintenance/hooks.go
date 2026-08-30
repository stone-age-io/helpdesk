package maintenance

import (
	"log/slog"
	"time"

	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/core"
)

// Register binds the plan create defaults and the completion-anchor follow-up.
func Register(app *pocketbase.PocketBase) {
	// Field defaults, "only if empty" so any caller can pre-set them — the same
	// shape internal/tickets and internal/projects use.
	app.OnRecordCreate("maintenance_plans").BindFunc(func(e *core.RecordEvent) error {
		if e.Record.GetString("anchor") == "" {
			e.Record.Set("anchor", "schedule")
		}
		if e.Record.GetString("priority") == "" {
			e.Record.Set("priority", "normal")
		}
		return e.Next()
	})

	// The completion anchor's other half: a generated ticket reaching a terminal
	// status restarts its plan's clock. After-success rather than pre-save
	// because the plan is a different record — nothing here rides the ticket's
	// own write.
	app.OnRecordAfterUpdateSuccess("tickets").BindFunc(func(e *core.RecordEvent) error {
		planID := e.Record.GetString("maintenance_plan")
		if planID == "" {
			return e.Next()
		}
		status := e.Record.GetString("status")
		if status != "resolved" && status != "closed" {
			return e.Next()
		}
		if e.Record.Original().GetString("status") == status {
			return e.Next() // some other field changed; the status didn't move
		}
		resumePlan(e.App, planID, e.Record)
		return e.Next()
	})
}

// resumePlan restarts a parked completion-anchored plan from the moment the
// work was actually done.
//
// The empty-next_due guard is what makes the usual resolved → closed journey
// safe: the resolve already restarted the clock, so the close (whether by hand
// or by the auto-close cron) finds a scheduled plan and does nothing. It also
// matters that syncResolvedAt CLEARS resolved_at on close — a ticket closed
// straight from open has none, and falls back to now.
//
// Best-effort throughout: a failed follow-up must never fail the ticket write.
func resumePlan(app core.App, planID string, ticket *core.Record) {
	plan, err := app.FindRecordById("maintenance_plans", planID)
	if err != nil {
		return
	}
	if plan.GetString("anchor") != "completion" {
		return // schedule-anchored plans advance in the cron, not here
	}
	if !plan.GetDateTime("next_due").IsZero() {
		return // already scheduled — this is the close after a resolve
	}
	interval := plan.GetInt("interval_days")
	if interval < 1 {
		return
	}

	done := ticket.GetDateTime("resolved_at").Time()
	if done.IsZero() {
		done = time.Now().UTC()
	}
	plan.Set("next_due", done.AddDate(0, 0, interval).Format(dateLayout))
	if err := app.Save(plan); err != nil {
		slog.Warn("maintenance: resume plan failed", "plan", plan.Id, "err", err)
	}
}
