// Package testutil provides the shared PocketBase test harness: a real app
// against a throwaway data dir with the full migration set applied, following
// the kiosk convention.
package testutil

import (
	"net/http"
	"testing"

	"github.com/pocketbase/pocketbase/apis"

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

// Handler builds the real PocketBase HTTP mux for the app, so a test can drive
// the record API end to end — the only way to exercise a COLLECTION RULE.
//
// Rules are this app's security boundary (see CLAUDE.md), but until now every
// rule test asserted on the rule STRING. That catches a dropped clause and
// nothing else: it cannot tell you whether a clause means what you think, which
// is how 1825000000's `= ”` vs `:isset = false` distinction was found. Prefer a
// string assertion for "this guard is still here" and this for "this guard
// actually blocks that request".
//
// Note that record hooks are NOT bound by SetupApp — a test that creates
// tickets through this handler must call tickets.Register(app) first or every
// create after the first collides on the unique `number` index.
func Handler(t *testing.T, app core.App) http.Handler {
	t.Helper()

	r, err := apis.NewRouter(app)
	if err != nil {
		t.Fatalf("build router: %v", err)
	}
	mux, err := r.BuildMux()
	if err != nil {
		t.Fatalf("build mux: %v", err)
	}
	return mux
}

// AuthToken mints a record auth token for use as an Authorization header.
func AuthToken(t *testing.T, rec *core.Record) string {
	t.Helper()

	tok, err := rec.NewAuthToken()
	if err != nil {
		t.Fatalf("auth token for %s: %v", rec.Id, err)
	}
	return tok
}
