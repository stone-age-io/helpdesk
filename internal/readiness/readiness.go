// Package readiness holds the helpdesk's own checks for GET /api/ready. The
// engine, the wire format and the database checks come from stone-age-io/kit
// (health and pbx); what is here is what only the helpdesk can know.
//
// The checks look for things that are SILENTLY wrong. A helpdesk with no SMTP
// settings serves every screen correctly and quietly sends no mail; one with no
// application URL puts dead links in the mail it does send; one whose NATS
// connection never came up keeps taking portal tickets while machine-filed ones
// stop arriving. None of that shows up on a dashboard, and all of it is a
// question this endpoint can answer in one request.
package readiness

import (
	"context"
	"fmt"

	"ganigeorgiev.com/pocketbase/pocketbase/core"
	"github.com/stone-age-io/kit/health"
	"github.com/stone-age-io/kit/pbx"
)

// Collections are the helpdesk's own collections. When they are missing, the
// binary's migrations never registered: PocketBase's own collections exist, the
// app serves, and nothing of ours is there.
var Collections = []string{"tickets", "staff", "customers"}

// Options carries what the checks read from outside the database.
type Options struct {
	// NATSConfigured and NATSConnected are read at probe time rather than
	// captured, because the connection is built during OnServe and may come
	// and go afterwards; a captured nil would report "never connected" forever.
	NATSConfigured func() bool
	NATSConnected  func() bool
	// InboundSecret is inbound.secret; the email route exists only when it is set.
	InboundSecret string
}

// Register adds every helpdesk check to reg.
func Register(reg *health.Registry, app core.App, o Options) {
	reg.Register("database", pbx.Database(app))
	reg.Register("schema", pbx.Schema(app, Collections...))
	reg.Register("schema_version", pbx.SchemaVersion(app))
	reg.Register("staff_admin", StaffAdmin(app))
	reg.Register("smtp", SMTP(app))
	reg.Register("application_url", ApplicationURL(app))
	reg.Register("nats_ingest", NATSIngest(o.NATSConfigured, o.NATSConnected))
	reg.Register("inbound_email", InboundEmail(o.InboundSecret))
}

// StaffAdmin warns, rather than fails, when nobody can administer the desk.
// Tickets still arrive and requesters still use the portal, so this is degraded
// and not dead, and neither restarting the process nor pulling it out of the
// load balancer would help.
func StaffAdmin(app core.App) health.Func {
	return func(ctx context.Context) health.Result {
		var n int
		err := app.DB().NewQuery("SELECT COUNT(*) FROM {{staff}} WHERE role = 'admin'").WithContext(ctx).Row(&n)
		if err != nil {
			return health.Skip("could not count admin staff: " + err.Error())
		}
		if n == 0 {
			return health.Warn("no staff member has the admin role",
				"Create one in the PocketBase dashboard at /_, or restore the bootstrap admin.")
		}
		return health.OK(fmt.Sprintf("%d admin(s)", n))
	}
}

// SMTP warns when outbound mail is not configured. Every notification path in
// the app then becomes a clean no-op: no error, no retry, no sign on any
// screen that the customer was never told anything.
func SMTP(app core.App) health.Func {
	return func(context.Context) health.Result {
		s := app.Settings()
		if !s.SMTP.Enabled {
			return health.Warn("disabled: no notification email is sent",
				"Enable SMTP in the PocketBase dashboard at /_ under Settings > Mail.")
		}
		if s.SMTP.Host == "" {
			return health.Warn("enabled but no host is set",
				"Set the SMTP host in the PocketBase dashboard at /_.")
		}
		return health.OK("enabled, host " + s.SMTP.Host)
	}
}

// ApplicationURL warns when the app URL is unset or still the default. Every
// notification deep-links /t/{id} against it, and the outbound NATS envelope
// carries it as ticket.url, so the default leaves customers with links to
// localhost and consumers with a useless field.
func ApplicationURL(app core.App) health.Func {
	const fix = "Set the application URL in the PocketBase dashboard at /_ under Settings > Application."
	return func(context.Context) health.Result {
		url := app.Settings().Meta.AppURL
		switch url {
		case "":
			return health.Warn("not set: notification links have no host", fix)
		case "http://localhost:8090":
			return health.Warn("still the default, so mailed links point at localhost", fix)
		}
		return health.OK(url)
	}
}

// NATSIngest reports the machine-ticket intake.
//
// A broker that is down is a WARN and never a fail: the helpdesk is explicitly
// best-effort about NATS, and the portal, webhook and email intakes are all
// still serving. Taking the process out of rotation would stop those too. That
// is why this is not the kit's natsx.Ready, which fails.
func NATSIngest(configured, connected func() bool) health.Func {
	return func(context.Context) health.Result {
		if !configured() {
			return health.Skip("no nats.urls configured")
		}
		if !connected() {
			return health.Warn("configured but not connected: machine-filed tickets are not arriving",
				"Check nats.urls and nats.creds_file, and that the hub account is reachable.")
		}
		return health.OK("connected")
	}
}

// InboundEmail reports whether the email intake route exists at all. It is
// registered only when inbound.secret is set, so an unset secret is a
// deliberate choice rather than a fault.
func InboundEmail(secret string) health.Func {
	return func(context.Context) health.Result {
		if secret == "" {
			return health.Skip("inbound.secret is not set, so the email route is not registered")
		}
		return health.OK("route registered")
	}
}
