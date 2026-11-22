package readiness

import (
	"testing"
	"time"

	"github.com/stone-age-io/kit/health"
	"github.com/stone-age-io/kit/health/healthtest"

	"github.com/stone-age-io/helpdesk/internal/testutil"
)

// A fresh install: migrations applied, no SMTP, no NATS, no inbound secret.
// Nothing fails, so it is ready; the silent misconfigurations warn; and the
// intakes that are off say skipped rather than ok.
func TestFreshInstall(t *testing.T) {
	app := testutil.SetupApp(t)
	var reg health.Registry
	Register(&reg, app, Options{
		NATSConfigured: func() bool { return false },
		NATSConnected:  func() bool { return false },
	})
	p := health.NewProber(&reg, "v0.0.0-test", time.Hour, 5*time.Second)
	healthtest.Conform(t, p.Handler()) // before the first probe
	p.Start(t.Context())
	healthtest.Conform(t, p.Handler())

	rep := p.Snapshot()
	if !rep.Ready {
		t.Fatalf("a fresh install should be ready: %+v", rep.Checks)
	}
	want := map[string]health.State{
		"database":        health.StateOK,
		"schema":          health.StateOK,
		"schema_version":  health.StateOK,
		"staff_admin":     health.StateOK, // the migrations seed a bootstrap admin
		"smtp":            health.StateWarn,
		"application_url": health.StateWarn,
		"nats_ingest":     health.StateSkipped,
		"inbound_email":   health.StateSkipped,
	}
	if len(rep.Checks) != len(want) {
		t.Fatalf("got %d checks, want %d: %+v", len(rep.Checks), len(want), rep.Checks)
	}
	for _, c := range rep.Checks {
		if c.State != want[c.Name] {
			t.Errorf("%s = %s (%s), want %s", c.Name, c.State, c.Detail, want[c.Name])
		}
	}
}

// NATS that is configured but down warns and never fails: the portal, webhook
// and email intakes still serve, and pulling the process would stop them.
func TestNATSDownWarns(t *testing.T) {
	res := NATSIngest(func() bool { return true }, func() bool { return false })(t.Context())
	if res.State != health.StateWarn || res.Fix == "" {
		t.Errorf("NATS down = %+v, want a warning with a fix", res)
	}
}
