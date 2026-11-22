// Command helpdesk is the Stone-Age.io service-ticket application: an embedded
// PocketBase (system of record + REST API + auth for both identity classes)
// serving the compiled SPA, driven by PocketBase's CLI (`helpdesk serve`).
//
// Configuration comes from helpdesk.yaml (path from $HELPDESK_CONFIG) plus
// HELPDESK_-prefixed environment overrides; see config/config.go. NATS
// ingestion and the inbound webhook are wired in the serve lifecycle.
package main

import (
	"context"
	"fmt"
	"io/fs"
	"log"
	"log/slog"
	"os"
	"time"

	"ganigeorgiev.com/pocketbase/pocketbase"
	"ganigeorgiev.com/pocketbase/pocketbase/core"
	"ganigeorgiev.com/pocketbase/pocketbase/plugins/migratecmd"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/nats-io/nats.go/micro"
	"github.com/spf13/cobra"
	"github.com/stone-age-io/kit/health"
	"github.com/stone-age-io/kit/natsx"
	"github.com/stone-age-io/kit/pbx"
	"github.com/stone-age-io/kit/version"

	"github.com/stone-age-io/helpdesk/config"
	"github.com/stone-age-io/helpdesk/internal/activity"
	"github.com/stone-age-io/helpdesk/internal/customers"
	"github.com/stone-age-io/helpdesk/internal/demoseed"
	"github.com/stone-age-io/helpdesk/internal/inbound"
	"github.com/stone-age-io/helpdesk/internal/ingest"
	"github.com/stone-age-io/helpdesk/internal/maintenance"
	"github.com/stone-age-io/helpdesk/internal/metrics"
	"github.com/stone-age-io/helpdesk/internal/notifications"
	"github.com/stone-age-io/helpdesk/internal/projects"
	"github.com/stone-age-io/helpdesk/internal/readiness"
	"github.com/stone-age-io/helpdesk/internal/streams"
	"github.com/stone-age-io/helpdesk/internal/subjects"
	"github.com/stone-age-io/helpdesk/internal/tickets"
	"github.com/stone-age-io/helpdesk/internal/timeentries"
	"github.com/stone-age-io/helpdesk/internal/timers"
	"github.com/stone-age-io/helpdesk/internal/visits"
	"github.com/stone-age-io/helpdesk/internal/webui"

	// Side-effect import: registers the schema migrations.
	_ "github.com/stone-age-io/helpdesk/migrations"
)

// sendLogRetentionDays bounds the notification_send_log + dedupe tables.
// 90 days matches the kiosk convention: long enough to debug "did the
// customer get that email", short enough to keep SQLite lean.
const sendLogRetentionDays = 90

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	app := pocketbase.NewWithConfig(pocketbase.Config{
		DefaultDataDir: cfg.DataDir,
	})

	// migratecmd exposes `helpdesk migrate ...` and, with Automigrate,
	// snapshots dashboard collection edits into Go files beside ours.
	migratecmd.MustRegister(app, app.RootCmd, migratecmd.Config{
		Dir:          "migrations",
		Automigrate:  true,
		TemplateLang: migratecmd.TemplateLangGo,
	})

	// `helpdesk seed-demo --confirm` fills a showcase instance. Registered here
	// rather than in OnServe because it is a one-shot subcommand, and it relies
	// on the record hooks below being bound the same way — a seeded ticket gets
	// its sequential number and audit trail exactly as a real one does.
	demoseed.RegisterCommand(app)

	// `helpdesk maintenance-run` performs the daily generation sweep on demand —
	// the catch-up path after downtime, since PB's cron is process-local.
	maintenance.RegisterCommand(app)

	// `helpdesk version` reports the same build string NATS service discovery
	// publishes, so `nats micro ls` and the box you are standing at can be
	// compared without guessing. The version comes from the kit's version
	// package, the same one natsx.RegisterService reads.
	//
	// RootCmd.Version is set from the same source on purpose. PocketBase seeds
	// it with its own string ("(untracked)" for a build with no ldflags), so
	// `helpdesk --version` and `helpdesk version` otherwise answer the same
	// question differently — and the wrong one of the two is the one an
	// operator reaches for first.
	app.RootCmd.Version = version.Build()
	app.RootCmd.AddCommand(&cobra.Command{
		Use:   "version",
		Short: "Print the helpdesk version",
		Run: func(cmd *cobra.Command, _ []string) {
			fmt.Printf("helpdesk %s\n", version.Build())
		},
	})

	tickets.Register(app)
	projects.Register(app)
	maintenance.Register(app)
	visits.Register(app)
	timers.Register(app)
	activity.Register(app)
	customers.Register(app)
	pbx.EnforceEmailVisibility(app, "users", "staff")

	// Outbound email: ticket lifecycle hooks → templated sends. The notifier
	// no-ops cleanly when SMTP isn't configured (PocketBase mail settings).
	notifier := notifications.New(app)
	notifications.RegisterHooks(app, notifier)

	// Drain in-flight notification goroutines on shutdown before PB tears
	// the DB down — a deliver() waking after the DB closes would panic
	// inside FindCollectionByNameOrId. Bounded best-effort.
	app.OnTerminate().BindFunc(func(e *core.TerminateEvent) error {
		notifier.WaitInFlight(2 * time.Second)
		return e.Next()
	})

	// Daily retention pass on the send log + dedupe table, well outside
	// business hours. PB's Cron is process-local; if the app is down at
	// fire time, the next live tick handles the backlog.
	app.Cron().Add("notifications_retention", "15 3 * * *", func() {
		cutoff := time.Now().UTC().AddDate(0, 0, -sendLogRetentionDays).Format("2006-01-02 15:04:05.000Z")
		if deleted, err := notifier.PruneSendLog(cutoff); err != nil {
			log.Printf("send log prune: %v", err)
		} else if deleted > 0 {
			log.Printf("send log prune: removed %d rows older than %d days", deleted, sendLogRetentionDays)
		}
		if deleted, err := notifier.PruneDedupe(cutoff); err != nil {
			log.Printf("dedupe prune: %v", err)
		} else if deleted > 0 {
			log.Printf("dedupe prune: removed %d rows older than %d days", deleted, sendLogRetentionDays)
		}
	})

	// Daily auto-close: promote tickets left `resolved` (the grace window)
	// untouched past the configured horizon to `closed`. Disabled when the
	// config is 0. Staggered a few minutes off the retention pass; PB's Cron is
	// process-local, so a missed fire is caught by the next live tick.
	if days := cfg.AutoCloseResolvedDays; days > 0 {
		app.Cron().Add("auto_close_resolved", "30 3 * * *", func() {
			if closed, err := tickets.AutoCloseResolved(app, days); err != nil {
				log.Printf("auto-close resolved: %v", err)
			} else if closed > 0 {
				log.Printf("auto-close resolved: closed %d ticket(s) resolved > %d days", closed, days)
			}
		})
	}

	// Daily preventive-maintenance sweep: materialize a ticket for every plan
	// that has come due. Unconditional — an install with no plans has nothing to
	// generate and a single plan is paused with one toggle, so a config flag
	// would be a second disable mechanism for the same thing.
	//
	// Ordering matters: this runs AFTER auto_close_resolved so a
	// completion-anchored plan whose ticket was auto-closed at 03:30 restarts its
	// clock the same night instead of waiting a day. A fire missed to downtime is
	// caught up with `helpdesk maintenance-run`.
	app.Cron().Add("maintenance_generate", "45 3 * * *", func() {
		created, skipped, err := maintenance.Generate(app, time.Now().UTC())
		if err != nil {
			log.Printf("maintenance generate: %v", err)
		} else if created > 0 || skipped > 0 {
			log.Printf("maintenance generate: created %d ticket(s), skipped %d plan(s) still open", created, skipped)
		}
	})

	// NATS resources brought up only when actually serving (not for
	// migrate/superuser subcommands) and torn down on terminate.
	var (
		nc       *nats.Conn
		svc      micro.Service
		consumer *ingest.Consumer
	)

	// The readiness prober's loop lives for as long as the server does.
	proberCtx, stopProber := context.WithCancel(context.Background())

	app.OnServe().BindFunc(func(e *core.ServeEvent) error {
		// Readiness. The NATS state is read through closures rather than
		// captured, because the connection below is built later in this same
		// hook and can come and go afterwards.
		var reg health.Registry
		readiness.Register(&reg, app, readiness.Options{
			NATSConfigured: func() bool { return cfg.NATS.Enabled() },
			NATSConnected:  func() bool { return nc != nil && nc.IsConnected() },
			InboundSecret:  cfg.Inbound.Secret,
		})
		prober := health.NewProber(&reg, version.Build(), cfg.Readiness.Interval(), cfg.Readiness.Timeout())
		pbx.ServeReady(e, prober)

		// Metrics share the prober's work rather than running their own checks:
		// a scrape and a probe would otherwise disagree about the same process,
		// and the disagreement would be a function of who asked last. Readiness
		// is always on; /metrics follows metrics.enabled, since an install may
		// legitimately not want it exposed.
		mset := metrics.New(version.Build())
		mset.RegisterDB(app)
		prober.OnRefresh(mset.Observe)
		if cfg.Metrics.Enabled {
			pbx.ServeMetrics(e, mset.Registry, cfg.Metrics.Token)
		}

		notifications.RegisterRoutes(e)
		inbound.Register(e)
		inbound.RegisterEmail(e, cfg.Inbound.Secret, cfg.Inbound.AllowedIPs)
		timeentries.RegisterRoutes(e)
		timers.RegisterRoutes(e)

		// NATS ingestion is best-effort: the helpdesk boots and serves
		// portal/agent/webhook traffic without a broker; machine tickets
		// resume when connectivity returns and the durable consumer picks
		// up where it left off.
		if cfg.NATS.Enabled() {
			var (
				js  jetstream.JetStream
				err error
			)
			nc, err = natsx.Connect(natsx.Options{
				URLs:  cfg.NATS.URLs,
				Creds: cfg.NATS.CredsFile,
				Name:  "helpdesk",
				Log:   slog.Default(),
				Extra: []nats.Option{
					nats.ReconnectJitter(100*time.Millisecond, time.Second),
					nats.DrainTimeout(10 * time.Second),
				},
			})
			if err == nil {
				js, err = jetstream.New(nc)
			}
			if err != nil {
				log.Printf("nats connect failed (tickets arrive via portal/webhook only): %v", err)
			} else {
				// Identity only, never fatal: `nats micro ls` lists the helpdesk
				// with its Thing code, which is how the console finds it.
				if cfg.NATS.Code == "" {
					log.Printf("nats.code is not set: skipping NATS service discovery registration")
				} else if svc, err = natsx.RegisterService(nc, natsx.Service{
					Name:        "helpdesk",
					Description: "Stone Age helpdesk",
					Code:        cfg.NATS.Code,
					Metadata:    hostMetadata(),
				}); err != nil {
					log.Printf("nats service discovery registration failed; continuing without it: %v", err)
				}

				subj := subjects.Default()
				ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
				defer cancel()
				if _, err := streams.EnsureIngest(ctx, js, cfg.NATS.Stream, subj.StreamWildcards()); err != nil {
					log.Printf("nats stream setup failed: %v", err)
				} else {
					consumer = ingest.New(app, js, cfg.NATS.Stream, cfg.NATS.Durable, subj)
					if err := consumer.Start(ctx); err != nil {
						log.Printf("nats consumer start failed: %v", err)
						consumer = nil
					}
				}

				// Outbound notification channel: helpdesk owns its egress stream
				// (helpdesk.*.evt.>), symmetric with the inbox. Best-effort —
				// if the creds lack publish/stream-management or the stream can't
				// be ensured, we log once and leave the publisher unset, so email
				// keeps working and NATS publishes are silent no-ops.
				if _, err := streams.EnsureNotify(ctx, js, cfg.NATS.NotifyStream, subj.EventStreamWildcards()); err != nil {
					log.Printf("nats notify stream setup failed (email still sends): %v", err)
				} else {
					notifier.SetPublisher(streams.NewPublisher(js))
				}
			}
		}

		// Started last, so the first probe sees the NATS connection above
		// rather than reporting it down for one interval on every boot. It
		// probes once synchronously, so /api/ready has a real answer as soon
		// as the server is listening.
		prober.Start(proberCtx)

		pbx.Branding(e, cfg.BrandingDir, slog.Default())
		uiFS, err := fs.Sub(webui.FS, "public")
		if err != nil {
			return err
		}
		pbx.ServeSPA(e, uiFS)
		log.Printf("helpdesk serving (dataDir=%s, nats=%v)", cfg.DataDir, cfg.NATS.Enabled())
		return e.Next()
	})

	app.OnTerminate().BindFunc(func(e *core.TerminateEvent) error {
		stopProber()
		consumer.Stop()
		if svc != nil {
			_ = svc.Stop()
		}
		if nc != nil {
			_ = nc.Drain()
		}
		return e.Next()
	})

	if err := app.Start(); err != nil {
		log.Fatalf("pocketbase exited with error: %v", err)
	}
}

// hostMetadata adds the host name to the discovery metadata, so `nats micro
// info helpdesk` says which box an instance runs on.
func hostMetadata() map[string]string {
	if host, err := os.Hostname(); err == nil {
		return map[string]string{"host": host}
	}
	return nil
}
