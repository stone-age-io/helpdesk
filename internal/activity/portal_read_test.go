package activity

import (
	"testing"

	"github.com/pocketbase/pocketbase/core"

	"github.com/stone-age-io/helpdesk/internal/testutil"
)

// The portal progress timeline reads ticket_events, so migration 1808000000
// opens a narrow slice of the (otherwise staff-only) audit trail to
// requesters. This locks the boundary: a requester sees STATUS events on
// their own company's tickets and nothing else — never priority/assignee
// events (whose values are staff names) and never another customer's trail.
func TestRequesterReadsOnlyOwnStatusEvents(t *testing.T) {
	app := testutil.SetupApp(t)

	custA := seed(t, app, "customers", map[string]any{"name": "Acme", "active": true})
	custB := seed(t, app, "customers", map[string]any{"name": "Globex", "active": true})

	reqA := seed(t, app, "users", map[string]any{
		"email": "rita@acme.example", "password": "secret123456",
		"name": "Rita", "customer": custA.Id, "active": true,
	})
	reqB := seed(t, app, "users", map[string]any{
		"email": "gus@globex.example", "password": "secret123456",
		"name": "Gus", "customer": custB.Id, "active": true,
	})
	agent := seed(t, app, "staff", map[string]any{
		"email": "sam@816tech.example", "password": "secret123456",
		"name": "Sam", "role": "agent", "active": true,
	})

	ticketA := seed(t, app, "tickets", map[string]any{
		"customer": custA.Id, "title": "pump", "number": 1, "status": "open", "priority": "normal",
	})

	statusEv := seed(t, app, "ticket_events", map[string]any{
		"ticket": ticketA.Id, "field": "status", "old_value": "open", "new_value": "in_progress", "actor_staff": agent.Id,
	})
	priorityEv := seed(t, app, "ticket_events", map[string]any{
		"ticket": ticketA.Id, "field": "priority", "old_value": "normal", "new_value": "high", "actor_staff": agent.Id,
	})
	assigneeEv := seed(t, app, "ticket_events", map[string]any{
		"ticket": ticketA.Id, "field": "assignee", "old_value": "", "new_value": "Sam Staff", "actor_staff": agent.Id,
	})

	col, err := app.FindCollectionByNameOrId("ticket_events")
	if err != nil {
		t.Fatalf("find ticket_events: %v", err)
	}
	if col.ViewRule == nil {
		t.Fatal("ticket_events ViewRule is nil")
	}

	can := func(auth, rec *core.Record) bool {
		ok, err := app.CanAccessRecord(rec, &core.RequestInfo{Auth: auth}, col.ViewRule)
		if err != nil {
			t.Fatalf("CanAccessRecord: %v", err)
		}
		return ok
	}

	// Requester of the ticket's customer: status events yes, others no.
	if !can(reqA, statusEv) {
		t.Error("requester should read a status event on their own ticket")
	}
	if can(reqA, priorityEv) {
		t.Error("requester must NOT read priority events")
	}
	if can(reqA, assigneeEv) {
		t.Error("requester must NOT read assignee events (would leak the staff roster)")
	}

	// Another customer's requester: nothing, not even the status event.
	if can(reqB, statusEv) {
		t.Error("requester of another customer must not read the status event")
	}

	// Staff still see the whole trail.
	if !can(agent, statusEv) || !can(agent, priorityEv) || !can(agent, assigneeEv) {
		t.Error("staff must read the full audit trail")
	}
}
