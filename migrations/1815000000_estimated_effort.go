package migrations

import (
	"fmt"

	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"

	"github.com/stone-age-io/helpdesk/internal/authz"
)

// Estimated effort: an optional `tickets.estimated_minutes` — the staff guess at
// how much work a ticket is, compared against the logged `time_entries` total
// per ticket and rolled up (summed) per project in the SPA. It is deliberately
// one nullable number: no baselines, no re-estimate history, no schedule math.
// The project rollup is derived at read time (like crew and total logged time),
// so nothing is stored server-side beyond this column.
//
// This is distinct from `visits.duration_minutes` (1809000000): that is the
// *calendar block* reserved for a field visit, entered at dispatch; this is the
// *effort estimate*, entered at intake before any visit exists. Different
// questions, different fields.
//
// Like `category`/`type`/`project`, the estimate is staff-set: the requester
// (portal) create rule gains an `estimated_minutes:isset = false` guard,
// superseding 1812000000's rule (kept in sync with it).

func init() {
	m.Register(estimatedEffortUp, estimatedEffortDown)
}

// estimatedEffortTicketsCreateRule supersedes 1812000000's
// serviceTicketsCreateRule by adding the estimated_minutes guard to the
// requester branch. Kept in sync with serviceTicketsCreateRule.
func estimatedEffortTicketsCreateRule() string {
	return "(" + authz.StaffRule + ")" +
		" || (" + authz.RequesterRule +
		" && @request.body.customer = @request.auth.customer" +
		" && @request.body.requester = @request.auth.id" +
		" && @request.body.assignee:isset = false" +
		" && @request.body.category:isset = false" +
		" && @request.body.project:isset = false" +
		" && @request.body.type:isset = false" +
		" && @request.body.location:isset = false" +
		" && @request.body.estimated_minutes:isset = false" +
		" && @request.body.source = 'portal')"
}

func estimatedEffortUp(app core.App) error {
	tickets, err := app.FindCollectionByNameOrId("tickets")
	if err != nil {
		return fmt.Errorf("find tickets: %w", err)
	}

	if tickets.Fields.GetByName("estimated_minutes") == nil {
		min := 1.0
		tickets.Fields.Add(&core.NumberField{Name: "estimated_minutes", OnlyInt: true, Min: &min})
	}

	createRule := estimatedEffortTicketsCreateRule()
	tickets.CreateRule = &createRule

	if err := app.Save(tickets); err != nil {
		return fmt.Errorf("save tickets: %w", err)
	}
	return nil
}

// estimatedEffortDown is dev-loop only, like the other down paths: drop the
// field and restore 1812000000's create rule.
func estimatedEffortDown(app core.App) error {
	tickets, err := app.FindCollectionByNameOrId("tickets")
	if err != nil {
		return fmt.Errorf("find tickets: %w", err)
	}
	tickets.Fields.RemoveByName("estimated_minutes")
	createRule := serviceTicketsCreateRule()
	tickets.CreateRule = &createRule
	if err := app.Save(tickets); err != nil {
		return fmt.Errorf("save tickets: %w", err)
	}
	return nil
}
