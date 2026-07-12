// Package timeentries exposes an aggregate-only time total for a ticket:
//
//	GET /api/helpdesk/tickets/{id}/time-total  →  { "minutes": N }
//
// Staff always see it (they have the full per-entry breakdown via the
// collection API anyway). A requester sees it only for their own customer's
// ticket AND only when that customer has customers.show_time_to_requester
// enabled — an opt-in, since exposing hours is a billing-model choice. Only the
// SUM leaves the server here; the per-entry rows (staff names, candid notes)
// never do.
package timeentries

import (
	"net/http"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"

	"github.com/stone-age-io/helpdesk/internal/authz"
)

// RegisterRoutes binds the total route. Wired from cmd/helpdesk in OnServe,
// beside notifications.RegisterRoutes and inbound.Register.
func RegisterRoutes(e *core.ServeEvent) {
	e.Router.GET("/api/helpdesk/tickets/{id}/time-total", handleTimeTotal)
}

func handleTimeTotal(re *core.RequestEvent) error {
	if re.Auth == nil {
		return re.UnauthorizedError("authentication required", nil)
	}
	ticket, err := re.App.FindRecordById("tickets", re.Request.PathValue("id"))
	if err != nil {
		return re.NotFoundError("ticket not found", nil)
	}
	// One 403 for every not-allowed reason (wrong customer, flag off, wrong
	// class) — no oracle about which.
	if !AllowTimeTotal(re.App, re.Auth, ticket) {
		return re.ForbiddenError("time totals not available", nil)
	}
	total, err := SumMinutes(re.App, ticket.Id)
	if err != nil {
		return re.InternalServerError("sum time failed", err)
	}
	return re.JSON(http.StatusOK, map[string]any{"minutes": total})
}

// AllowTimeTotal is the visibility policy: staff always; a requester only for
// their own customer's ticket when that customer has opted in. Exported so it
// can be tested without HTTP (the inbound.CreateTicket convention).
func AllowTimeTotal(app core.App, auth *core.Record, ticket *core.Record) bool {
	if auth == nil {
		return false
	}
	switch auth.Collection().Name {
	case authz.StaffCollection:
		return true
	case authz.RequesterCollection:
		if ticket.GetString("customer") != auth.GetString("customer") {
			return false
		}
		customer, err := app.FindRecordById("customers", ticket.GetString("customer"))
		return err == nil && customer.GetBool("show_time_to_requester")
	}
	return false
}

// SumMinutes totals time_entries.minutes for a ticket. The aggregate is the
// only thing this package ever exposes.
func SumMinutes(app core.App, ticketID string) (int, error) {
	entries, err := app.FindRecordsByFilter("time_entries", "ticket = {:t}", "", 0, 0, dbx.Params{"t": ticketID})
	if err != nil {
		return 0, err
	}
	total := 0
	for _, r := range entries {
		total += r.GetInt("minutes")
	}
	return total, nil
}
