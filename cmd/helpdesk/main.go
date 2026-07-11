// Command helpdesk is the 816tech service-ticket application: an embedded
// PocketBase (system of record + REST API + auth for both identity classes)
// serving the compiled SPA, driven by PocketBase's CLI (`helpdesk serve`).
//
// Configuration comes from helpdesk.yaml (path from $HELPDESK_CONFIG) plus
// HELPDESK_-prefixed environment overrides; see config/config.go. NATS
// ingestion and the inbound webhook are wired in the serve lifecycle.
package main

import (
	"io/fs"
	"log"
	"strings"
	"time"

	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/plugins/migratecmd"

	"github.com/stone-age-io/helpdesk/config"
	"github.com/stone-age-io/helpdesk/internal/notifications"
	"github.com/stone-age-io/helpdesk/internal/tickets"
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

	tickets.Register(app)

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

	app.OnServe().BindFunc(func(e *core.ServeEvent) error {
		notifications.RegisterRoutes(e)
		if err := serveSPA(e); err != nil {
			return err
		}
		log.Printf("helpdesk serving (dataDir=%s, nats=%v)", cfg.DataDir, cfg.NATS.Enabled())
		return e.Next()
	})

	if err := app.Start(); err != nil {
		log.Fatalf("pocketbase exited with error: %v", err)
	}
}

// serveSPA registers the embedded SPA at "/" with history-mode fallback.
// PocketBase does not serve static assets in framework mode, so we register
// the catch-all ourselves; the more specific /api and /_ routes PocketBase
// registers take precedence over /{path...}.
func serveSPA(e *core.ServeEvent) error {
	uiFS, err := fs.Sub(webui.FS, "public")
	if err != nil {
		return err
	}
	e.Router.GET("/{path...}", func(re *core.RequestEvent) error {
		p := re.Request.PathValue("path")
		if p == "" || p == "/" {
			return re.FileFS(uiFS, "index.html")
		}
		if f, openErr := uiFS.Open(p); openErr == nil {
			_ = f.Close()
			return re.FileFS(uiFS, p)
		}
		// A missing asset (has an extension) is a real 404; anything else is
		// a client-side route → hand back index.html so vue-router resolves it.
		if strings.Contains(p, ".") {
			return re.NotFoundError("File not found", nil)
		}
		return re.FileFS(uiFS, "index.html")
	})
	return nil
}
