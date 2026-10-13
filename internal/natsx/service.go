package natsx

import (
	"log/slog"
	"os"
	"regexp"
	"runtime/debug"
	"strings"

	"github.com/nats-io/nats.go/micro"
)

// ServiceName is what the helpdesk registers as on NATS service discovery.
const ServiceName = "helpdesk"

// RegisterService announces the helpdesk on the NATS micro discovery protocol
// ($SRV.PING, $SRV.INFO, $SRV.STATS), so `nats micro ls` in the hub account
// shows it beside the agents, rule-routers and access-control binaries: its
// version, host and start time.
//
// It is identity, not an API. The service has no endpoints, so there is
// nothing to call, read or control through it, and it is not the request/reply
// service this app keeps out of scope. Like access-control's, it is never
// fatal: a failure is logged and the app carries on. A credential without
// subscribe on $SRV.> is refused discovery only, and nats.go logs the refusal.
// Close stops the service.
func (c *Conn) RegisterService() {
	meta := map[string]string{}
	if host, err := os.Hostname(); err == nil {
		meta["host"] = host
	}
	ver := serviceVersion(buildVersion())
	svc, err := micro.AddService(c.NC, micro.Config{
		Name:        ServiceName,
		Version:     ver,
		Description: "Stone Age helpdesk",
		Metadata:    meta,
	})
	if err != nil {
		slog.Warn("nats service discovery registration failed; continuing without it", "err", err)
		return
	}
	c.svc = svc
	slog.Info("registered for nats service discovery", "service", ServiceName, "version", ver)
}

// buildVersion is the module version `go build` stamped from the checkout: a
// tag, a pseudo-version between tags, or "(devel)" without VCS info.
func buildVersion() string {
	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" {
		return info.Main.Version
	}
	return "(devel)"
}

// semverPattern is the regular expression semver.org publishes, which is also
// the one micro validates a service version against.
var semverPattern = regexp.MustCompile(`^(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)(?:-((?:0|[1-9]\d*|\d*[a-zA-Z-][0-9a-zA-Z-]*)(?:\.(?:0|[1-9]\d*|\d*[a-zA-Z-][0-9a-zA-Z-]*))*))?(?:\+([0-9a-zA-Z-]+(?:\.[0-9a-zA-Z-]+)*))?$`)

// serviceVersion is the build version in the form micro accepts: a leading
// "v" is dropped, and anything still not semver reports as 0.0.0-dev.
func serviceVersion(v string) string {
	v = strings.TrimPrefix(v, "v")
	if semverPattern.MatchString(v) {
		return v
	}
	return "0.0.0-dev"
}
