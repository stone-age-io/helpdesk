package migrations_test

import (
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase/core"

	"github.com/stone-age-io/helpdesk/internal/testutil"
)

// Exercises the 1815000000 estimated-effort migration: the new optional
// tickets.estimated_minutes field and the requester create guard that keeps it
// staff-only.
func TestEstimatedEffortSchema(t *testing.T) {
	app := testutil.SetupApp(t)

	tickets, err := app.FindCollectionByNameOrId("tickets")
	if err != nil {
		t.Fatalf("find tickets: %v", err)
	}

	f := tickets.Fields.GetByName("estimated_minutes")
	nf, ok := f.(*core.NumberField)
	if !ok {
		t.Fatalf("tickets.estimated_minutes should be a number field, got %T", f)
	}
	if !nf.OnlyInt {
		t.Error("tickets.estimated_minutes should be integer-only")
	}
	if nf.Required {
		t.Error("tickets.estimated_minutes should be optional (nil = unestimated)")
	}

	// The requester (portal) create rule keeps the estimate staff-only, and must
	// still carry the 1812 service-field guards it supersedes.
	if r := tickets.CreateRule; r == nil ||
		!strings.Contains(*r, "@request.body.estimated_minutes:isset = false") ||
		!strings.Contains(*r, "@request.body.type:isset = false") ||
		!strings.Contains(*r, "@request.body.location:isset = false") {
		t.Errorf("tickets create rule missing estimate/service guards: %v", tickets.CreateRule)
	}
}

// Staff may set an estimate; it round-trips.
func TestEstimatedEffortRoundTrip(t *testing.T) {
	app := testutil.SetupApp(t)

	customer := seed(t, app, "customers", map[string]any{"name": "Acme", "active": true})
	ticket := seed(t, app, "tickets", map[string]any{
		"customer": customer.Id, "title": "Install access control", "number": 1,
		"type": "install", "estimated_minutes": 240,
	})

	got, err := app.FindRecordById("tickets", ticket.Id)
	if err != nil {
		t.Fatalf("reload ticket: %v", err)
	}
	if got.GetInt("estimated_minutes") != 240 {
		t.Errorf("estimated_minutes: got %d, want 240", got.GetInt("estimated_minutes"))
	}
}
