// Package timers turns the manual time_entries ledger into a start/stop timer
// without changing what gets stored. A running timer is one open row in
// time_sessions (see migrations/1811000000); this package owns the two pieces
// of glue that can't live in collection rules:
//
//   - a create hook that server-stamps started_at, so elapsed time is
//     trustworthy regardless of the client clock; and
//   - POST /api/helpdesk/tickets/../../timers/{id}/stop, which resolves an open
//     timer into a normal time_entries row and deletes the session, atomically.
//
// Everything durable is a time_entries row — the ticket stays the canonical
// labor ledger and the customer-facing total (internal/timeentries) is
// unchanged. Minute precision is deliberately loose: elapsed rounds to the
// nearest five minutes and the caller may override it outright.
package timers

import (
	"math"
	"net/http"
	"strings"
	"time"

	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/types"

	"github.com/stone-age-io/helpdesk/internal/authz"
)

// Register binds the create hook that server-stamps started_at. The one open
// timer per agent invariant is the unique index on time_sessions.staff, not a
// hook — a check-then-create hook would race two quick taps of Start.
func Register(app *pocketbase.PocketBase) {
	app.OnRecordCreate("time_sessions").BindFunc(func(e *core.RecordEvent) error {
		// Ignore any client-supplied value: the server clock is the base the
		// stop route rounds into minutes.
		e.Record.Set("started_at", types.NowDateTime())
		return e.Next()
	})
}

// StopOpts controls how an open timer resolves into a labor entry.
type StopOpts struct {
	Minutes       int    // explicit override; <= 0 means compute from elapsed
	Note          string // overrides the session note when non-empty
	CompleteVisit bool   // also mark the session's visit completed, if any
}

// Stop resolves an open timer into a time_entries row and deletes the session,
// atomically. With CompleteVisit and a visit attached, the visit is flipped to
// completed in the same transaction (the visits guard stamps completed_at).
// Exported so it can be tested without HTTP (the timeentries.AllowTimeTotal
// convention). Returns the created entry.
func Stop(app core.App, session *core.Record, opts StopOpts) (*core.Record, error) {
	minutes := opts.Minutes
	if minutes <= 0 {
		elapsed := time.Since(session.GetDateTime("started_at").Time()).Minutes()
		minutes = round5(elapsed)
	}
	if minutes < 1 {
		minutes = 1 // a timer stopped almost immediately still logs a minute
	}

	note := strings.TrimSpace(opts.Note)
	if note == "" {
		note = session.GetString("note")
	}
	visitID := session.GetString("visit")

	var entry *core.Record
	err := app.RunInTransaction(func(txApp core.App) error {
		entries, err := txApp.FindCollectionByNameOrId("time_entries")
		if err != nil {
			return err
		}
		entry = core.NewRecord(entries)
		entry.Set("ticket", session.GetString("ticket"))
		entry.Set("staff", session.GetString("staff"))
		entry.Set("minutes", minutes)
		entry.Set("work_date", types.NowDateTime())
		entry.Set("note", note)
		if visitID != "" {
			entry.Set("visit", visitID)
		}
		if err := txApp.Save(entry); err != nil {
			return err
		}

		if opts.CompleteVisit && visitID != "" {
			visit, err := txApp.FindRecordById("visits", visitID)
			if err != nil {
				return err
			}
			visit.Set("status", "completed")
			if err := txApp.Save(visit); err != nil {
				return err
			}
		}

		return txApp.Delete(session)
	})
	if err != nil {
		return nil, err
	}
	return entry, nil
}

// RegisterRoutes binds the stop route, wired from cmd/helpdesk in OnServe
// beside timeentries.RegisterRoutes.
func RegisterRoutes(e *core.ServeEvent) {
	e.Router.POST("/api/helpdesk/timers/{id}/stop", handleStop)
}

type stopRequest struct {
	Minutes       int    `json:"minutes"`
	Note          string `json:"note"`
	CompleteVisit bool   `json:"complete_visit"`
}

func handleStop(re *core.RequestEvent) error {
	if re.Auth == nil {
		return re.UnauthorizedError("authentication required", nil)
	}
	session, err := re.App.FindRecordById("time_sessions", re.Request.PathValue("id"))
	if err != nil {
		return re.NotFoundError("timer not found", nil)
	}
	if !ownerOrAdmin(re.Auth, session) {
		return re.ForbiddenError("not your timer", nil)
	}
	var body stopRequest
	if err := re.BindBody(&body); err != nil {
		return re.BadRequestError("invalid body", err)
	}
	entry, err := Stop(re.App, session, StopOpts{
		Minutes:       body.Minutes,
		Note:          body.Note,
		CompleteVisit: body.CompleteVisit,
	})
	if err != nil {
		return re.InternalServerError("stop timer failed", err)
	}
	return re.JSON(http.StatusOK, entry)
}

// ownerOrAdmin mirrors the collection UpdateRule: staff act on their own
// timer, admins on anyone's. (Collection rules don't gate a custom route.)
func ownerOrAdmin(auth, session *core.Record) bool {
	if auth == nil || auth.Collection().Name != authz.StaffCollection {
		return false
	}
	if auth.GetString("role") == "admin" {
		return true
	}
	return session.GetString("staff") == auth.Id
}

// round5 rounds elapsed minutes to the nearest five. Accuracy isn't the point
// here — the agent can override on stop — so a tidy number beats a precise one.
func round5(mins float64) int {
	return int(math.Round(mins/5.0)) * 5
}
