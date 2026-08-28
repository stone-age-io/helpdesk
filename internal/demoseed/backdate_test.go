package demoseed_test

import (
	"testing"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"

	"github.com/stone-age-io/helpdesk/internal/testutil"
)

// Probe: PocketBase's autodate field overwrites `created` on every save, in
// both create and update. Confirm a raw UPDATE through app.DB() can backdate it
// and that the value survives a normal record read — the whole reason the
// seeder is in-process Go rather than an HTTP client.
func TestBackdateCreatedViaRawSQL(t *testing.T) {
	app := testutil.SetupApp(t)

	col, err := app.FindCollectionByNameOrId("customers")
	if err != nil {
		t.Fatalf("find customers: %v", err)
	}
	rec := core.NewRecord(col)
	rec.Set("name", "Probe")
	rec.Set("active", true)
	if err := app.Save(rec); err != nil {
		t.Fatalf("save: %v", err)
	}
	if got := rec.GetString("created"); got == "" {
		t.Fatal("expected an autodate created value")
	}

	const want = "2026-01-15 10:00:00.000Z"
	if _, err := app.DB().NewQuery(
		"UPDATE customers SET created = {:c} WHERE id = {:id}",
	).Bind(dbx.Params{"c": want, "id": rec.Id}).Execute(); err != nil {
		t.Fatalf("raw update: %v", err)
	}

	reloaded, err := app.FindRecordById("customers", rec.Id)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if got := reloaded.GetString("created"); got != want {
		t.Fatalf("created = %q, want %q", got, want)
	}
}
