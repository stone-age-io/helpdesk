package migrations_test

import (
	"testing"

	"github.com/pocketbase/pocketbase/core"

	"github.com/stone-age-io/helpdesk/internal/testutil"
)

// The 1821000000 migration adds the optional tickets.resolved_at datetime that
// the auto-close cron reads.
func TestTicketResolvedAtSchema(t *testing.T) {
	app := testutil.SetupApp(t)

	tickets, err := app.FindCollectionByNameOrId("tickets")
	if err != nil {
		t.Fatalf("find tickets: %v", err)
	}
	f := tickets.Fields.GetByName("resolved_at")
	df, ok := f.(*core.DateField)
	if !ok {
		t.Fatalf("tickets.resolved_at should be a date field, got %T", f)
	}
	if df.Required {
		t.Error("tickets.resolved_at should be optional (nil until resolved)")
	}
}
