// Package testutil provides the shared PocketBase test harness: a real app
// against a throwaway data dir with the full migration set applied, following
// the kiosk convention.
package testutil

import (
	"testing"

	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/core"

	// Side-effect import: registers the schema migrations into the test binary.
	_ "github.com/stone-age-io/helpdesk/migrations"
)

// SetupApp boots a real PocketBase against t.TempDir() and applies all
// registered migrations. The returned app is ready for record CRUD.
func SetupApp(t *testing.T) *pocketbase.PocketBase {
	t.Helper()

	app := pocketbase.NewWithConfig(pocketbase.Config{
		DefaultDataDir:  t.TempDir(),
		HideStartBanner: true,
	})
	if err := app.Bootstrap(); err != nil {
		t.Fatalf("bootstrap: %v", err)
	}
	runner := core.NewMigrationsRunner(app, core.AppMigrations)
	if _, err := runner.Up(); err != nil {
		t.Fatalf("migrations up: %v", err)
	}
	t.Cleanup(func() { _ = app.ResetBootstrapState() })
	return app
}
