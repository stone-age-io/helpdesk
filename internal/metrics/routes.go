package metrics

import (
	"crypto/subtle"
	"encoding/base64"
	"net/http"
	"strings"

	"ganigeorgiev.com/pocketbase/pocketbase/core"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// RegisterRoutes mounts GET /metrics when enabled.
//
// Note /api/ready is always on and only this follows `enabled`: readiness is a
// contract with whatever orchestrates the process, where metrics are something
// an install may legitimately not want exposed.
//
// The handler does NOT use PocketBase auth. PocketBase tokens are JWTs that
// expire and scrapers cannot refresh them, so a token here is a static shared
// secret, accepted in the two forms every common scraper can send.
func RegisterRoutes(e *core.ServeEvent, s *Set, enabled bool, token string) {
	if !enabled {
		return
	}
	h := promhttp.HandlerFor(s.Registry, promhttp.HandlerOpts{})
	e.Router.GET("/metrics", func(re *core.RequestEvent) error {
		if token != "" && !authorized(re.Request.Header.Get("Authorization"), token) {
			// The realm makes a browser prompt and a misconfigured scrape fail
			// clearly, rather than looking like an empty metrics endpoint.
			re.Response.Header().Set("WWW-Authenticate", `Basic realm="metrics"`)
			return re.JSON(http.StatusUnauthorized, map[string]string{"error": "metrics token required"})
		}
		h.ServeHTTP(re.Response, re.Request)
		return nil
	})
}

// authorized accepts either form:
//
//	Authorization: Bearer <token>        // Prometheus bearer_token
//	Authorization: Basic <any>:<token>   // basic_auth, username ignored
//
// The username is ignored on purpose: a scraper's basic_auth block always has
// one, it is not a second secret, and requiring a particular value would only
// produce confusing 401s.
func authorized(header, token string) bool {
	if header == "" {
		return false
	}
	scheme, value, ok := strings.Cut(header, " ")
	if !ok {
		return false
	}
	switch strings.ToLower(scheme) {
	case "bearer":
		return constantTimeEqual(value, token)
	case "basic":
		raw, err := base64.StdEncoding.DecodeString(value)
		if err != nil {
			return false
		}
		_, pass, ok := strings.Cut(string(raw), ":")
		return ok && constantTimeEqual(pass, token)
	}
	return false
}

func constantTimeEqual(a, b string) bool {
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}
