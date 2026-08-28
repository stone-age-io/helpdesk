package migrations

import (
	"fmt"

	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"

	"github.com/stone-age-io/helpdesk/internal/authz"
)

// Portal site + device: let a requester name the site and the device on intake.
//
// Every classification field has been staff-only since 1806000000, for a good
// reason: category/type/project/estimated_minutes are triage decisions, and a
// requester guessing at them makes the queue worse. `location` and `thing` are
// different in kind — they are not judgements about the work, they are facts
// about where it is and what it is on, and the requester is the ONLY person who
// knows them at intake. Staff have been reconstructing both by hand from the
// free-text notes ever since.
//
// So the two relations come off the guard list, and `location_note` /
// `thing_note` stay as the fallback for anything not in the catalog (which is
// most devices — the things catalog is a hand-curated superset, never complete).
//
// # The guard that replaces the guard
//
// Dropping `:isset = false` is NOT sufficient. PocketBase validates that a
// relation id EXISTS, not that it belongs to the caller's tenant, so a bare
// removal would let a requester attach another customer's site or device to
// their own ticket — a cross-tenant write, and a read primitive besides (the
// ticket detail expands `location` and `thing` and would render the other
// tenant's names straight back). The replacement is the `@request.body.<rel>.<field>`
// relation hop already proven by ticket_comments in 1800000000 and 1822000000:
//
//	@request.body.location.customer = @request.auth.customer
//
// # Why `= ''` and not `:isset = false`
//
// Every prior hop in this schema is on ticket_comments.ticket, which is
// REQUIRED — the relation is always present, so the empty case has never been
// exercised. These two are optional, and that changes the answer. Measured
// against a real PocketBase (v0.38.1), for a requester POSTing to
// /api/collections/tickets/records:
//
//	rule clause                    absent  ""    own   other's
//	`:isset = false`                 ok    400   400     400
//	`:isset = false || <hop>`        ok    400    ok     400
//	`= '' || <hop>`                  ok     ok    ok     400
//
// The middle row is the trap. A picker that hasn't been touched submits
// `location: ""`, not nothing — so the `:isset` form rejects every ticket filed
// without a site, which is most of them. `= ''` covers BOTH the empty and the
// absent case (PocketBase resolves a missing body key to the zero value), which
// is why the clause is two terms and not three; adding `:isset = false` back as
// a third term changes no outcome. Also verified to hold for `null`, for an
// array-wrapped id (`["<id>"]`, the multi-relation wire shape), and for a
// syntactically bogus id.
//
// Keep the `= ''` term FIRST. Reversed, a null/empty value would fall into the
// hop, and a hop off an empty relation resolves to nothing rather than to the
// caller's customer.
//
// # Why `=` and not `?=`
//
// `?=` ("at least one related record matches") is the usual advice for relation
// hops, but it is the WRONG operator for a tenant guard. Both fields are
// maxSelect=1, and PocketBase accepts the multi-relation wire shape `["<id>"]`
// for them regardless; with `?=`, a body carrying both a legitimate id and
// another tenant's would satisfy the clause on the strength of the legitimate
// one. Plain `=` requires every submitted value to match, which is what a tenant
// boundary means. Verified: `["<other tenant's id>"]` is rejected.

func init() {
	m.Register(portalSiteDeviceUp, portalSiteDeviceDown)
}

// portalTenantHop builds the "unset, or belongs to my customer" clause for one
// optional relation on the requester branch. See the table above.
func portalTenantHop(field string) string {
	return " && (@request.body." + field + " = ''" +
		" || @request.body." + field + ".customer = @request.auth.customer)"
}

// portalSiteDeviceTicketsCreateRule supersedes 1824000000's
// thingsTicketsCreateRule by trading the `location` and `thing` :isset guards
// for tenant-scoped hops. The other five guards are unchanged — this is the
// sixth verbatim restatement of this rule and nothing composes it, so diff it
// against thingsTicketsCreateRule before editing either.
func portalSiteDeviceTicketsCreateRule() string {
	return "(" + authz.StaffRule + ")" +
		" || (" + authz.RequesterRule +
		" && @request.body.customer = @request.auth.customer" +
		" && @request.body.requester = @request.auth.id" +
		" && @request.body.assignee:isset = false" +
		" && @request.body.category:isset = false" +
		" && @request.body.project:isset = false" +
		" && @request.body.type:isset = false" +
		" && @request.body.estimated_minutes:isset = false" +
		portalTenantHop("location") +
		portalTenantHop("thing") +
		" && @request.body.source = 'portal')"
}

func portalSiteDeviceUp(app core.App) error {
	tickets, err := app.FindCollectionByNameOrId("tickets")
	if err != nil {
		return fmt.Errorf("find tickets: %w", err)
	}
	rule := portalSiteDeviceTicketsCreateRule()
	tickets.CreateRule = &rule
	if err := app.Save(tickets); err != nil {
		return fmt.Errorf("save tickets: %w", err)
	}
	return nil
}

// portalSiteDeviceDown restores 1824000000's rule — the one with BOTH :isset
// guards. Restoring an earlier builder would silently drop the `thing` guard
// (1815's) or the `estimated_minutes` guard (1812's).
func portalSiteDeviceDown(app core.App) error {
	tickets, err := app.FindCollectionByNameOrId("tickets")
	if err != nil {
		return nil // collection already gone; nothing to restore
	}
	rule := thingsTicketsCreateRule()
	tickets.CreateRule = &rule
	if err := app.Save(tickets); err != nil {
		return fmt.Errorf("save tickets: %w", err)
	}
	return nil
}
