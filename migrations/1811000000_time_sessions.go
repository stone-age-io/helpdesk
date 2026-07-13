package migrations

import (
	"fmt"

	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"

	"github.com/stone-age-io/helpdesk/internal/authz"
)

// time_sessions — the running-timer scratchpad, NOT a second labor ledger.
//
// A row's existence means "this agent has a timer running." There is at most
// one open row per agent, enforced by a unique index on `staff` — that unique
// index is the whole one-timer-per-agent invariant, at the DB level. Stopping
// or canceling a timer DELETES the row: the durable record is the time_entries
// row that internal/timers mints from it on stop. The collection therefore
// holds only the open interval's start and never accumulates history — the
// ergonomic front-end to the existing manual-minutes ledger, not a rework of
// it.
//
//   staff       the agent timing (required); unique → one timer each.
//   ticket      what's being worked (required, cascade — a timer can't outlive
//               its ticket).
//   visit       optional on-site session this timer belongs to; no cascade,
//               matching time_entries.visit (1809000000) — though the row is
//               short-lived in practice.
//   started_at  server-stamped by internal/timers on create; the base the stop
//               route rounds into minutes.
//   note        optional running note, carried onto the resulting entry.
//
// Staff-only, mirroring time_entries (1800000000_init.go): you create and stop
// your own timer; an admin can clear a stuck one. Requesters never see it.

func init() {
	m.Register(timeSessionsUp, timeSessionsDown)
}

func timeSessionsUp(app core.App) error {
	if _, err := app.FindCollectionByNameOrId("time_sessions"); err == nil {
		return nil // idempotent: already created
	}

	staff, err := app.FindCollectionByNameOrId("staff")
	if err != nil {
		return fmt.Errorf("find staff: %w", err)
	}
	tickets, err := app.FindCollectionByNameOrId("tickets")
	if err != nil {
		return fmt.Errorf("find tickets: %w", err)
	}
	visits, err := app.FindCollectionByNameOrId("visits")
	if err != nil {
		return fmt.Errorf("find visits: %w", err)
	}

	sessions := core.NewBaseCollection("time_sessions")
	sessions.Fields.Add(&core.RelationField{
		Name:         "staff",
		CollectionId: staff.Id,
		Required:     true,
		MaxSelect:    1,
	})
	sessions.Fields.Add(&core.RelationField{
		Name:          "ticket",
		CollectionId:  tickets.Id,
		Required:      true,
		MaxSelect:     1,
		CascadeDelete: true,
	})
	sessions.Fields.Add(&core.RelationField{
		Name:         "visit",
		CollectionId: visits.Id,
		MaxSelect:    1,
		// No CascadeDelete: consistent with time_entries.visit — a timer's
		// draft must not vanish because a visit was removed mid-work.
	})
	sessions.Fields.Add(&core.DateField{Name: "started_at", Required: true})
	sessions.Fields.Add(&core.TextField{Name: "note", Max: 1000})
	sessions.Fields.Add(&core.AutodateField{Name: "created", OnCreate: true})

	// One open timer per agent — the invariant lives here, not in a hook.
	sessions.AddIndex("idx_time_sessions_staff", true, "staff", "")
	sessions.AddIndex("idx_time_sessions_ticket", false, "ticket", "")

	// Staff-only; own-or-admin for writes, exactly like time_entries.
	staffRule := authz.StaffRule
	createRule := authz.StaffRule + " && @request.body.staff = @request.auth.id"
	ownOrAdmin := authz.AdminRule + " || (staff = @request.auth.id && " + authz.StaffRule + ")"
	sessions.ListRule = &staffRule
	sessions.ViewRule = &staffRule
	sessions.CreateRule = &createRule
	sessions.UpdateRule = &ownOrAdmin
	sessions.DeleteRule = &ownOrAdmin

	if err := app.Save(sessions); err != nil {
		return fmt.Errorf("save time_sessions: %w", err)
	}
	return nil
}

// timeSessionsDown is dev-loop only, like the other down paths.
func timeSessionsDown(app core.App) error {
	if sessions, err := app.FindCollectionByNameOrId("time_sessions"); err == nil {
		return app.Delete(sessions)
	}
	return nil
}
