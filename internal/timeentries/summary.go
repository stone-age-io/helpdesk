package timeentries

import (
	"net/http"
	"time"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"

	"github.com/stone-age-io/helpdesk/internal/authz"
)

// Batch companion to the per-ticket total:
//
//	GET /api/helpdesk/reports/time-by-ticket?from=YYYY-MM-DD&to=YYYY-MM-DD
//	  →  { "enabled": true, "minutes": { "<ticketId>": 120, ... } }
//
// It exposes NOTHING the per-ticket route doesn't already: the same policy
// (AllowTimeTotal), the same billable-only redaction for requesters, and the
// same aggregate-only shape — a caller could get identical numbers by hitting
// /tickets/{id}/time-total once per ticket. This just avoids the round trips,
// which is what makes a portal report page possible at all.
//
// Keyed by ticket id rather than pre-grouped by site or device on purpose: the
// portal already holds those tickets client-side with their location and thing
// expanded, so it rolls up whatever axis it wants without this route growing a
// second copy of the staff report's grouping logic.
//
// `enabled: false` (with an empty map) is the honest answer for a requester
// whose customer has not opted into show_time_to_requester — the page hides its
// hours section rather than rendering a misleading zero.

// registerSummaryRoute is called from RegisterRoutes.
func registerSummaryRoute(e *core.ServeEvent) {
	e.Router.GET("/api/helpdesk/reports/time-by-ticket", handleTimeByTicket)
}

// TimeScope is the whole policy of this route in one value: whose time the
// caller may see, and how redacted. Split out from the handler so it can be
// tested without HTTP — the AllowTimeTotal / inbound.CreateTicket convention.
type TimeScope struct {
	// False when the caller is a requester whose customer has not opted into
	// show_time_to_requester. Not an error — the honest answer is "no hours for
	// you", and the portal hides its hours section rather than showing a zero.
	Enabled bool
	// "" means unscoped, which only a staff caller that passed no ?customer
	// ever reaches.
	CustomerID string
	// Requesters see the invoiceable figure, never internal rework or goodwill.
	BillableOnly bool
}

// ResolveTimeScope decides what the caller gets. ok=false is a flat 403 with no
// oracle about which reason (wrong class, no customer); ok=true with
// Enabled=false is the opt-out answer.
func ResolveTimeScope(app core.App, auth *core.Record, queryCustomer string) (TimeScope, bool) {
	if auth == nil {
		return TimeScope{}, false
	}
	switch auth.Collection().Name {
	case authz.StaffCollection:
		// Staff read time_entries directly through the collection API anyway;
		// this branch exists so both shells can share one client helper.
		return TimeScope{Enabled: true, CustomerID: queryCustomer}, true
	case authz.RequesterCollection:
		customerID := auth.GetString("customer")
		if customerID == "" {
			return TimeScope{}, false
		}
		customer, err := app.FindRecordById("customers", customerID)
		if err != nil || !customer.GetBool("show_time_to_requester") {
			return TimeScope{Enabled: false, CustomerID: customerID, BillableOnly: true}, true
		}
		return TimeScope{Enabled: true, CustomerID: customerID, BillableOnly: true}, true
	}
	return TimeScope{}, false
}

func handleTimeByTicket(re *core.RequestEvent) error {
	scope, ok := ResolveTimeScope(re.App, re.Auth, re.Request.URL.Query().Get("customer"))
	if !ok {
		if re.Auth == nil {
			return re.UnauthorizedError("authentication required", nil)
		}
		return re.ForbiddenError("time totals not available", nil)
	}
	if !scope.Enabled {
		return re.JSON(http.StatusOK, map[string]any{"enabled": false, "minutes": map[string]int{}})
	}

	from, to, err := parseRange(re.Request.URL.Query().Get("from"), re.Request.URL.Query().Get("to"))
	if err != nil {
		return re.BadRequestError("invalid date range", err)
	}

	minutes, err := SumMinutesByTicket(re.App, scope.CustomerID, from, to, scope.BillableOnly)
	if err != nil {
		return re.InternalServerError("sum time failed", err)
	}
	return re.JSON(http.StatusOK, map[string]any{"enabled": true, "minutes": minutes})
}

// parseRange bounds the window. Both ends are optional; an absent end means
// "unbounded", which is what a report with no date filter asks for.
func parseRange(from, to string) (string, string, error) {
	for _, v := range []string{from, to} {
		if v == "" {
			continue
		}
		if _, err := time.Parse("2006-01-02", v); err != nil {
			return "", "", err
		}
	}
	return from, to, nil
}

// SumMinutesByTicket totals time_entries per ticket for one customer over a
// work_date window. Exported for testing without HTTP, matching SumMinutes.
//
// The customer scope is a relation hop to the ticket, so a row can never leak
// across tenants even though time_entries itself carries no customer field.
func SumMinutesByTicket(app core.App, customerID, from, to string, billableOnly bool) (map[string]int, error) {
	filter := "1 = 1"
	params := dbx.Params{}
	if customerID != "" {
		filter += " && ticket.customer = {:c}"
		params["c"] = customerID
	}
	if from != "" {
		filter += " && work_date >= {:from}"
		params["from"] = from
	}
	if to != "" {
		// work_date is a date field; PocketBase stores it with a time part, so
		// bound with the end of the day rather than the bare date.
		filter += " && work_date <= {:to}"
		params["to"] = to + " 23:59:59.999Z"
	}

	entries, err := app.FindRecordsByFilter("time_entries", filter, "", 0, 0, params)
	if err != nil {
		return nil, err
	}
	out := map[string]int{}
	for _, r := range entries {
		if billableOnly && r.GetBool("non_billable") {
			continue
		}
		out[r.GetString("ticket")] += r.GetInt("minutes")
	}
	return out, nil
}
