package natsx

import (
	"log/slog"
	"os"

	"github.com/nats-io/nats.go/micro"

	"github.com/stone-age-io/helpdesk/internal/version"
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
	ver := version.Semver(version.Build())
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
