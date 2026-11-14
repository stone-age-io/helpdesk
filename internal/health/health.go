// Package health answers "can this process do its job" at GET /api/ready,
// matching the shape the platform and the rest of the ecosystem use (see
// Health & Metrics in platform-docs). PocketBase already serves /api/health,
// which reports that the HTTP server is listening; it is listening through
// every failure below.
//
// The checks look for things that are SILENTLY wrong. A helpdesk with no SMTP
// settings serves every screen correctly and quietly sends no mail; one with no
// application URL puts dead links in the mail it does send; one whose NATS
// connection never came up keeps taking portal tickets while machine-filed ones
// stop arriving. None of that shows up on a dashboard, and all of it is a
// question this endpoint can answer in one request.
package health

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"ganigeorgiev.com/pocketbase/pocketbase/core"
)

// State is a check's verdict. The four values and their ranking are the
// ecosystem's contract, not ours to extend.
type State string

const (
	StateOK      State = "ok"
	StateWarn    State = "warn"
	StateFail    State = "fail"
	StateSkipped State = "skipped"
)

// severity orders the states worst-first for rolling up the report.
//
// `skipped` ranks BELOW `ok` deliberately: it means there is no answer, so a
// report where every check skipped must not read as healthy. Only `fail`
// clears `ready`, because a probe can do just two things, restart the process
// or take it out of rotation, and neither fixes a misconfiguration.
func severity(s State) int {
	switch s {
	case StateFail:
		return 3
	case StateWarn:
		return 2
	case StateSkipped:
		return 1
	default:
		return 0
	}
}

// Check is one line of the report.
type Check struct {
	Name   string `json:"name"`
	State  State  `json:"state"`
	Detail string `json:"detail"`
	Fix    string `json:"fix,omitempty"`
	Took   string `json:"took"`
}

// Report is the whole body of GET /api/ready.
type Report struct {
	Ready   bool    `json:"ready"`
	State   State   `json:"state"`
	Version string  `json:"version"`
	Uptime  string  `json:"uptime"`
	Took    string  `json:"took"`
	Checked string  `json:"checked"`
	Checks  []Check `json:"checks"`
}

// Func runs one check and returns its verdict, a human detail, and optionally
// the command or text to act on. It must not assume it is the only caller.
type Func func(ctx context.Context) (state State, detail, fix string)

type namedCheck struct {
	name string
	fn   Func
}

// Prober runs the checks on an interval and holds the last report.
//
// The endpoint never runs a check. A readiness probe is called often and by
// things with short timeouts, and a check that dials NATS or touches the disk
// would make the endpoint as slow as its slowest dependency, which is the
// opposite of what a probe is for.
type Prober struct {
	interval time.Duration
	timeout  time.Duration
	version  string
	started  time.Time
	checks   []namedCheck

	mu        sync.RWMutex
	last      *Report
	observers []func(*Report)
}

// OnProbe registers a callback run after every probe, with the report just
// stored. It exists so /metrics can publish the same numbers /api/ready
// serves: two independent check runs would let a scrape and a probe disagree
// about one process, and which answer you got would depend on who asked last.
//
// Callbacks run synchronously on the prober's goroutine and must not block.
func (p *Prober) OnProbe(fn func(*Report)) {
	p.observers = append(p.observers, fn)
}

// NewProber builds a prober. interval and timeout fall back to sane values so
// a zero config cannot turn the loop into a busy wait or hang it forever.
func NewProber(version string, interval, timeout time.Duration) *Prober {
	if interval <= 0 {
		interval = 30 * time.Second
	}
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	return &Prober{
		interval: interval,
		timeout:  timeout,
		version:  version,
		started:  time.Now(),
	}
}

// Add registers a check. Checks are reported sorted by name, so two probes can
// be diffed line for line.
func (p *Prober) Add(name string, fn Func) {
	p.checks = append(p.checks, namedCheck{name: name, fn: fn})
}

// Start probes once immediately, so a container is not answering 503 for a
// whole interval after it comes up, then every interval until ctx ends.
func (p *Prober) Start(ctx context.Context) {
	p.Probe(ctx)
	go func() {
		t := time.NewTicker(p.interval)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				p.Probe(ctx)
			}
		}
	}()
}

// Probe runs every check once and replaces the stored report.
func (p *Prober) Probe(ctx context.Context) *Report {
	start := time.Now()
	out := make([]Check, 0, len(p.checks))
	for _, c := range p.checks {
		out = append(out, p.runOne(ctx, c))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })

	worst := StateOK
	ready := true
	for _, c := range out {
		if severity(c.State) > severity(worst) {
			worst = c.State
		}
		if c.State == StateFail {
			ready = false
		}
	}

	rep := &Report{
		Ready:   ready,
		State:   worst,
		Version: p.version,
		Uptime:  time.Since(p.started).Truncate(time.Second).String(),
		Took:    time.Since(start).Truncate(time.Millisecond).String(),
		Checked: time.Now().UTC().Format(time.RFC3339),
		Checks:  out,
	}
	p.mu.Lock()
	p.last = rep
	p.mu.Unlock()

	for _, fn := range p.observers {
		fn(rep)
	}
	return rep
}

// runOne applies the timeout and contains a panicking check. A check that
// panics is a failure of that check, never of the probe loop: losing every
// other answer because one collector hit a nil map would be the wrong trade.
func (p *Prober) runOne(ctx context.Context, c namedCheck) (res Check) {
	start := time.Now()
	res = Check{Name: c.name, State: StateFail, Detail: "check did not complete"}
	defer func() {
		if r := recover(); r != nil {
			res = Check{
				Name:   c.name,
				State:  StateFail,
				Detail: fmt.Sprintf("check panicked: %v", r),
			}
		}
		res.Took = time.Since(start).Truncate(time.Millisecond).String()
	}()

	cctx, cancel := context.WithTimeout(ctx, p.timeout)
	defer cancel()

	state, detail, fix := c.fn(cctx)
	return Check{Name: c.name, State: state, Detail: detail, Fix: fix}
}

// Last returns the stored report, or nil before the first probe completes.
func (p *Prober) Last() *Report {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.last
}

// --- the helpdesk's checks -------------------------------------------------

// Database fails when SQLite does not answer. pb_data is not writable, or
// another instance holds it.
func Database(app core.App) Func {
	return func(ctx context.Context) (State, string, string) {
		var n int
		err := app.DB().NewQuery("SELECT 1").Row(&n)
		if err != nil {
			return StateFail, "not responding: " + err.Error(),
				"Check that pb_data is writable and no other helpdesk process holds it."
		}
		return StateOK, "responding", ""
	}
}

// Schema fails when the helpdesk's own collections are missing. That is a
// binary whose migrations never registered: PocketBase's own collections exist,
// the app serves, and nothing of ours is there.
func Schema(app core.App) Func {
	return func(ctx context.Context) (State, string, string) {
		for _, name := range []string{"tickets", "staff", "customers"} {
			if _, err := app.FindCollectionByNameOrId(name); err != nil {
				return StateFail, "collection " + name + " is missing",
					"Run: ./helpdesk migrate up"
			}
		}
		return StateOK, "helpdesk collections present", ""
	}
}

// SchemaVersion looks for a DOWNGRADE, not for pending migrations. `serve`
// applies every migration before it listens, so nothing can be pending while a
// probe is able to run. What it looks for is the database being AHEAD of the
// binary: migrations do not roll back, so an older binary on newer pb_data runs
// against columns and rules it does not know.
func SchemaVersion(app core.App) Func {
	return func(ctx context.Context) (State, string, string) {
		known := map[string]bool{}
		for _, m := range core.AppMigrations.Items() {
			known[m.File] = true
		}
		var applied []string
		err := app.DB().NewQuery("SELECT file FROM {{_migrations}}").Column(&applied)
		if err != nil {
			return StateSkipped, "could not read the migrations table: " + err.Error(), ""
		}
		var unknown []string
		for _, f := range applied {
			// System migrations are PocketBase's own and are not in AppMigrations.
			if !known[f] && isAppMigration(f) {
				unknown = append(unknown, f)
			}
		}
		if len(unknown) > 0 {
			return StateFail,
				fmt.Sprintf("the database has %d migration(s) this binary does not know, starting with %s", len(unknown), unknown[0]),
				"This pb_data was written by a newer helpdesk. Run that version instead; migrations do not roll back."
		}
		return StateOK, fmt.Sprintf("%d applied, none unknown", len(applied)), ""
	}
}

// isAppMigration tells our timestamped migrations from PocketBase's own, which
// live in the same table. Ours are named 18xxxxxxxx_*.go by convention.
func isAppMigration(file string) bool {
	return len(file) > 10 && file[0] == '1' && file[1] == '8'
}

// StaffAdmin warns, rather than fails, when nobody can administer the desk.
// Tickets still arrive and requesters still use the portal, so this is degraded
// and not dead, and neither restarting the process nor pulling it out of the
// load balancer would help.
func StaffAdmin(app core.App) Func {
	return func(ctx context.Context) (State, string, string) {
		var n int
		err := app.DB().NewQuery("SELECT COUNT(*) FROM {{staff}} WHERE role = 'admin'").Row(&n)
		if err != nil {
			return StateSkipped, "could not count admin staff: " + err.Error(), ""
		}
		if n == 0 {
			return StateWarn, "no staff member has the admin role",
				"Create one in the PocketBase dashboard at /_, or restore the bootstrap admin."
		}
		return StateOK, fmt.Sprintf("%d admin(s)", n), ""
	}
}

// SMTP warns when outbound mail is not configured. Every notification path in
// the app then becomes a clean no-op: no error, no retry, no sign on any
// screen that the customer was never told anything.
func SMTP(app core.App) Func {
	return func(ctx context.Context) (State, string, string) {
		s := app.Settings()
		if !s.SMTP.Enabled {
			return StateWarn, "disabled: no notification email is sent",
				"Enable SMTP in the PocketBase dashboard at /_ under Settings > Mail."
		}
		if s.SMTP.Host == "" {
			return StateWarn, "enabled but no host is set",
				"Set the SMTP host in the PocketBase dashboard at /_."
		}
		return StateOK, "enabled, host " + s.SMTP.Host, ""
	}
}

// ApplicationURL warns when the app URL is unset or still the default. Every
// notification deep-links /t/{id} against it, and the outbound NATS envelope
// carries it as ticket.url, so the default leaves customers with links to
// localhost and consumers with a useless field.
func ApplicationURL(app core.App) Func {
	return func(ctx context.Context) (State, string, string) {
		url := app.Settings().Meta.AppURL
		if url == "" {
			return StateWarn, "not set: notification links have no host",
				"Set the application URL in the PocketBase dashboard at /_ under Settings > Application."
		}
		if url == "http://localhost:8090" {
			return StateWarn, "still the default, so mailed links point at localhost",
				"Set the application URL in the PocketBase dashboard at /_ under Settings > Application."
		}
		return StateOK, url, ""
	}
}

// NATSIngest reports the machine-ticket intake. configured and connected are
// read at probe time rather than captured, because the connection is built
// during OnServe and may come and go afterwards.
//
// A broker that is down is a WARN and never a fail: the helpdesk is explicitly
// best-effort about NATS, and the portal, webhook and email intakes are all
// still serving. Taking the process out of rotation would stop those too.
func NATSIngest(configured func() bool, connected func() bool) Func {
	return func(ctx context.Context) (State, string, string) {
		if !configured() {
			return StateSkipped, "no nats.urls configured", ""
		}
		if !connected() {
			return StateWarn, "configured but not connected: machine-filed tickets are not arriving",
				"Check nats.urls and nats.creds_file, and that the hub account is reachable."
		}
		return StateOK, "connected", ""
	}
}

// InboundEmail reports whether the email intake route exists at all. It is
// registered only when inbound.secret is set, so an unset secret is a
// deliberate choice rather than a fault.
func InboundEmail(secret string) Func {
	return func(ctx context.Context) (State, string, string) {
		if secret == "" {
			return StateSkipped, "inbound.secret is not set, so the email route is not registered", ""
		}
		return StateOK, "route registered", ""
	}
}
