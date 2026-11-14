package health

import (
	"net/http"

	"ganigeorgiev.com/pocketbase/pocketbase/core"
)

// RegisterRoutes mounts GET /api/ready.
//
// Unauthenticated, with one body for everyone. Its callers are container
// orchestrators, load balancers and uptime checkers, and none of them holds a
// PocketBase session. Put it behind a reverse proxy to restrict it.
//
// It is deliberately NOT under /api/helpdesk like this app's other routes: the
// ecosystem's contract is that every binary answers readiness at the same
// path, so a probe written for one can be pointed at another.
func RegisterRoutes(e *core.ServeEvent, p *Prober) {
	e.Router.GET("/api/ready", func(re *core.RequestEvent) error {
		// no-store, because a cached readiness answer is worse than none: a
		// proxy could keep serving "ok" for a process that has since failed.
		re.Response.Header().Set("Cache-Control", "no-store")

		rep := p.Last()
		if rep == nil {
			// Before the first probe completes there is no answer, and
			// "no answer" must not read as ready.
			return re.JSON(http.StatusServiceUnavailable, map[string]any{
				"ready":  false,
				"state":  StateFail,
				"detail": "readiness has not been probed yet",
			})
		}

		status := http.StatusOK
		if !rep.Ready {
			status = http.StatusServiceUnavailable
		}
		return re.JSON(status, rep)
	})
}
