// Package metrics builds the Prometheus registry served at GET /metrics (the
// route is the kit's pbx.ServeMetrics), matching
// the shape the platform and the agent use (see Health & Metrics in
// platform-docs): the readiness report as a state set, build info, and a small
// number of gauges about what is in the database.
//
// Two rules are carried over from the platform deliberately.
//
// NO METRIC CARRIES A PER-CUSTOMER LABEL. /metrics is open unless a token is
// set, and a customer label would put a tenant's name next to a ticket count
// for anyone who can reach the port. The queue shape is exported by status,
// which is the number anyone actually alerts on, and per-customer reporting
// lives behind the authenticated report routes where it belongs.
//
// A COLLECTOR THAT FAILS EMITS NOTHING, NOT ZERO. Zero is a legitimate value
// here (an install with no projects yet), so publishing zero on failure would
// give a wrong answer and an alert on `== 0` would fire for the wrong reason.
// A failed collector raises helpdesk_collector_errors and leaves its series
// absent, so pair any alert with absent() if a gap matters to you.
package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"

	"github.com/stone-age-io/kit/health"
)

// allStates is every value the state set publishes a series for, so a check
// that moves from ok to fail leaves a 0 behind rather than a gap.
var allStates = []health.State{
	health.StateOK,
	health.StateWarn,
	health.StateFail,
	health.StateSkipped,
}

// Namespace prefixes every series this app exports.
const Namespace = "helpdesk"

// Set is the registry plus the gauges we own.
type Set struct {
	Registry *prometheus.Registry

	ready      prometheus.Gauge
	checkState *prometheus.GaugeVec
	checkTime  prometheus.Gauge
}

// New builds the set and registers the Go runtime and process collectors
// beside it, so a scrape also answers "how is this process itself doing".
func New(version string) *Set {
	reg := prometheus.NewRegistry()

	reg.MustRegister(
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
	)

	// The conventional build-info pattern: a constant 1 whose labels carry the
	// facts, so a dashboard can group by version without a second source.
	buildInfo := prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: Namespace,
		Name:      "build_info",
		Help:      "Build information. Always 1; read the labels.",
	}, []string{"version"})
	buildInfo.WithLabelValues(version).Set(1)
	reg.MustRegister(buildInfo)

	s := &Set{
		Registry: reg,
		ready: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: Namespace,
			Name:      "ready",
			Help:      "1 if the last readiness probe passed, 0 if any check failed. Warnings do not clear this.",
		}),
		checkState: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: Namespace,
			Name:      "check_state",
			Help:      "Readiness check state set: 1 on the check's current state, 0 on the others.",
		}, []string{"name", "state"}),
		checkTime: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: Namespace,
			Name:      "check_timestamp_seconds",
			Help:      "Unix time of the last readiness probe. Alert on this going stale: it means the prober itself is wedged, and no other series shows that because they all keep their last values.",
		}),
	}
	reg.MustRegister(s.ready, s.checkState, s.checkTime)
	return s
}

// Observe copies a readiness report into the gauges. A nil report sets ready to
// 0, because "never probed" must read as not ready.
func (s *Set) Observe(rep *health.Report) {
	if rep == nil {
		s.ready.Set(0)
		return
	}
	if rep.Ready {
		s.ready.Set(1)
	} else {
		s.ready.Set(0)
	}
	s.checkTime.Set(float64(nowUnix()))

	for _, c := range rep.Checks {
		for _, st := range allStates {
			v := 0.0
			if c.State == st {
				v = 1.0
			}
			s.checkState.WithLabelValues(c.Name, string(st)).Set(v)
		}
	}
}
