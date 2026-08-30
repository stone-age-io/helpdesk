package migrations_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase/core"

	"github.com/stone-age-io/helpdesk/internal/authz"
	"github.com/stone-age-io/helpdesk/internal/testutil"
	"github.com/stone-age-io/helpdesk/internal/tickets"
)

// These exercise 1829000000: the maintenance_plans collection, tickets.due_at
// and tickets.maintenance_plan, and the create-rule guards that keep both
// staff-only.

func TestMaintenancePlansSchema(t *testing.T) {
	app := testutil.SetupApp(t)

	plans, err := app.FindCollectionByNameOrId("maintenance_plans")
	if err != nil {
		t.Fatalf("find maintenance_plans: %v", err)
	}

	for _, name := range []string{
		"customer", "title", "body", "thing", "location", "project",
		"category", "assignee", "priority", "estimated_minutes",
		"interval_days", "anchor", "lead_time_days", "next_due", "paused",
	} {
		if plans.Fields.GetByName(name) == nil {
			t.Errorf("maintenance_plans.%s missing", name)
		}
	}

	// interval_days is the one required schedule field: a plan without one has
	// no recurrence and the generator would skip it forever.
	iv, ok := plans.Fields.GetByName("interval_days").(*core.NumberField)
	if !ok {
		t.Fatalf("interval_days should be a number field, got %T", plans.Fields.GetByName("interval_days"))
	}
	if !iv.Required || !iv.OnlyInt {
		t.Error("interval_days should be a required integer")
	}

	// next_due must stay OPTIONAL — empty is the "parked" state a
	// completion-anchored plan sits in while its ticket is open.
	nd, ok := plans.Fields.GetByName("next_due").(*core.DateField)
	if !ok {
		t.Fatalf("next_due should be a date field, got %T", plans.Fields.GetByName("next_due"))
	}
	if nd.Required {
		t.Error("next_due must be optional — empty means parked")
	}

	// `paused`, not `active`: a Go bool's zero value is false, so an `active`
	// field would make every hand-created plan arrive inactive.
	if plans.Fields.GetByName("active") != nil {
		t.Error("maintenance_plans should carry `paused`, not `active` — see 1829000000")
	}
	if _, ok := plans.Fields.GetByName("paused").(*core.BoolField); !ok {
		t.Error("paused should be a bool field")
	}

	anchor, ok := plans.Fields.GetByName("anchor").(*core.SelectField)
	if !ok {
		t.Fatalf("anchor should be a select field, got %T", plans.Fields.GetByName("anchor"))
	}
	if len(anchor.Values) != 2 || anchor.Values[0] != "schedule" || anchor.Values[1] != "completion" {
		t.Errorf("anchor values = %v, want [schedule completion]", anchor.Values)
	}
}

// TestMaintenancePlansRules pins the locations split (1813000000): any staff
// member creates and edits, delete stays admin, and requesters see nothing —
// a plan carries `assignee`, the MSP roster the portal hides everywhere else.
func TestMaintenancePlansRules(t *testing.T) {
	app := testutil.SetupApp(t)

	plans, err := app.FindCollectionByNameOrId("maintenance_plans")
	if err != nil {
		t.Fatalf("find maintenance_plans: %v", err)
	}

	for name, rule := range map[string]*string{
		"list": plans.ListRule, "view": plans.ViewRule,
		"create": plans.CreateRule, "update": plans.UpdateRule,
	} {
		if rule == nil || *rule != authz.StaffRule {
			t.Errorf("%s rule = %v, want StaffRule", name, rule)
		}
		if rule != nil && strings.Contains(*rule, "@request.auth.customer") {
			t.Errorf("%s rule should not be requester-readable — it exposes the staff roster", name)
		}
	}
	if plans.DeleteRule == nil || *plans.DeleteRule != authz.AdminRule {
		t.Errorf("delete rule = %v, want AdminRule", plans.DeleteRule)
	}
}

func TestTicketsGainDueAtAndPlan(t *testing.T) {
	app := testutil.SetupApp(t)

	ticketsCol, err := app.FindCollectionByNameOrId("tickets")
	if err != nil {
		t.Fatalf("find tickets: %v", err)
	}

	due, ok := ticketsCol.Fields.GetByName("due_at").(*core.DateField)
	if !ok {
		t.Fatalf("tickets.due_at should be a date field, got %T", ticketsCol.Fields.GetByName("due_at"))
	}
	if due.Required {
		t.Error("due_at should be optional — most tickets never get one")
	}

	rel, ok := ticketsCol.Fields.GetByName("maintenance_plan").(*core.RelationField)
	if !ok {
		t.Fatalf("tickets.maintenance_plan should be a relation, got %T", ticketsCol.Fields.GetByName("maintenance_plan"))
	}
	if rel.CascadeDelete {
		t.Error("maintenance_plan must not cascade-delete — deleting a plan would erase its history")
	}

	// A cron-generated ticket needs an honest provenance value; reusing `agent`
	// would make the scheduler indistinguishable from a human at the desk.
	src, ok := ticketsCol.Fields.GetByName("source").(*core.SelectField)
	if !ok {
		t.Fatalf("tickets.source should be a select field, got %T", ticketsCol.Fields.GetByName("source"))
	}
	found := false
	for _, v := range src.Values {
		if v == "maintenance" {
			found = true
		}
	}
	if !found {
		t.Errorf("tickets.source values = %v, want a `maintenance` option", src.Values)
	}
}

// TestMaintenanceCreateRuleIntact checks the new guards landed without dropping
// any inherited one. This rule is restated verbatim by each migration that
// touches it, so a copy-paste slip here is a silent portal privilege change.
func TestMaintenanceCreateRuleIntact(t *testing.T) {
	app := testutil.SetupApp(t)

	ticketsCol, err := app.FindCollectionByNameOrId("tickets")
	if err != nil {
		t.Fatalf("find tickets: %v", err)
	}
	if ticketsCol.CreateRule == nil {
		t.Fatal("tickets create rule is nil")
	}
	rule := *ticketsCol.CreateRule

	for _, clause := range []string{
		"@request.body.assignee:isset = false",
		"@request.body.category:isset = false",
		"@request.body.project:isset = false",
		"@request.body.type:isset = false",
		"@request.body.estimated_minutes:isset = false",
		"@request.body.due_at:isset = false",
		"@request.body.maintenance_plan:isset = false",
		"@request.body.source = 'portal'",
	} {
		if !strings.Contains(rule, clause) {
			t.Errorf("tickets create rule missing %q", clause)
		}
	}

	// 1825000000's tenant hops must survive — restoring an older builder by
	// mistake would re-close the portal's site and device pickers.
	for _, field := range []string{"location", "thing"} {
		hop := "@request.body." + field + ".customer = @request.auth.customer"
		if !strings.Contains(rule, hop) {
			t.Errorf("tickets create rule lost the %s tenant hop", field)
		}
		if strings.Contains(rule, "@request.body."+field+":isset = false") {
			t.Errorf("%s should not be :isset-guarded — requesters set it since 1825000000", field)
		}
	}
}

// TestPortalCannotSetDueAtOrPlan runs the guard for real, over HTTP. A string
// assertion proves the clause is present; only this proves it bites.
func TestPortalCannotSetDueAtOrPlan(t *testing.T) {
	app := testutil.SetupApp(t)
	// Without the create hook every ticket gets number 0 and the second create
	// fails on the unique index — which looks exactly like a rule rejection.
	tickets.Register(app)

	customer := seed(t, app, "customers", map[string]any{"name": "Acme", "active": true})
	plan := seed(t, app, "maintenance_plans", map[string]any{
		"customer": customer.Id, "title": "Quarterly service", "interval_days": 90,
	})

	users, err := app.FindCollectionByNameOrId("users")
	if err != nil {
		t.Fatalf("find users: %v", err)
	}
	u := core.NewRecord(users)
	u.Set("email", "requester@acme.test")
	u.Set("password", "password12345")
	u.Set("customer", customer.Id)
	u.Set("active", true)
	if err := app.Save(u); err != nil {
		t.Fatalf("save requester: %v", err)
	}

	mux := testutil.Handler(t, app)
	token := testutil.AuthToken(t, u)

	post := func(extra map[string]any) int {
		body := map[string]any{
			"customer":  customer.Id,
			"requester": u.Id,
			"title":     "Badge reader is dead",
			"source":    "portal",
		}
		for k, v := range extra {
			body[k] = v
		}
		raw, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		req := httptest.NewRequest("POST", "/api/collections/tickets/records", bytes.NewReader(raw))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", token)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		return rec.Code
	}

	// The baseline still works — otherwise the rejections below prove nothing.
	if code := post(nil); code != http.StatusOK {
		t.Fatalf("plain portal ticket: got %d, want 200", code)
	}

	for name, body := range map[string]map[string]any{
		"due date":         {"due_at": "2026-12-31"},
		"maintenance plan": {"maintenance_plan": plan.Id},
		"both":             {"due_at": "2026-12-31", "maintenance_plan": plan.Id},
	} {
		if code := post(body); code == http.StatusOK {
			t.Errorf("requester set %s: got 200, want rejection", name)
		}
	}
}
