package inbound

// Postmark inbound adapter — the ONLY Postmark-aware code. It authenticates the
// provider webhook, decodes Postmark's inbound JSON, maps it to the
// provider-agnostic NormalizedInbound, and hands off to IngestEmail. A different
// provider (SES, CloudMailin) is a sibling file that produces the same struct;
// nothing else changes. See docs/email-ingestion.md.

import (
	"crypto/subtle"
	"net"
	"net/http"
	"strings"

	"github.com/pocketbase/pocketbase/core"
)

// RegisterEmail binds the Postmark inbound webhook. An empty secret disables the
// route entirely (email ingestion off), mirroring how NATS is skipped when
// unconfigured.
func RegisterEmail(e *core.ServeEvent, secret string, allowedIPs []string) {
	if secret == "" {
		return
	}
	nets := parseAllowedNets(allowedIPs)
	e.Router.POST("/api/helpdesk/inbound/email/postmark", func(re *core.RequestEvent) error {
		// IP allowlist first (cheap, and doesn't reveal whether the secret was
		// close): an empty allowlist means "secret alone gates it".
		if !ipInAllowlist(re.RealIP(), nets) {
			return re.ForbiddenError("forbidden", nil)
		}
		_, pass, ok := re.Request.BasicAuth()
		if !ok || !secretMatches(pass, secret) {
			re.Response.Header().Set("WWW-Authenticate", `Basic realm="helpdesk"`)
			return re.UnauthorizedError("unauthorized", nil)
		}

		var pm postmarkInbound
		if err := re.BindBody(&pm); err != nil {
			return re.BadRequestError("invalid JSON body", err)
		}

		res, err := IngestEmail(re.App, pm.normalize())
		if err != nil {
			// A genuine server fault — let Postmark retry (it does on non-2xx).
			return re.InternalServerError("ingest failed", err)
		}

		// Everything intentionally handled OR intentionally dropped is 2xx, so
		// Postmark stops retrying. The DB write already committed.
		out := map[string]any{"status": res.Outcome}
		if res.Ticket != nil {
			out["id"] = res.Ticket.Id
			out["number"] = res.Ticket.GetInt("number")
		}
		if res.Reason != "" {
			out["reason"] = res.Reason
		}
		return re.JSON(http.StatusOK, out)
	})
}

// postmarkInbound is the subset of Postmark's inbound JSON we consume.
type postmarkInbound struct {
	MessageID string `json:"MessageID"`
	From      string `json:"From"`
	FromFull  struct {
		Email string `json:"Email"`
		Name  string `json:"Name"`
	} `json:"FromFull"`
	Subject           string `json:"Subject"`
	TextBody          string `json:"TextBody"`
	StrippedTextReply string `json:"StrippedTextReply"`
	Headers           []struct {
		Name  string `json:"Name"`
		Value string `json:"Value"`
	} `json:"Headers"`
}

// normalize maps Postmark's shape onto NormalizedInbound. The core derives the
// [#N] thread token from Subject, so we don't set ReplyToken here.
func (p postmarkInbound) normalize() NormalizedInbound {
	headers := make(map[string]string, len(p.Headers))
	for _, h := range p.Headers {
		headers[strings.ToLower(h.Name)] = h.Value
	}

	// Prefer Postmark's quoted-history-stripped reply; fall back to full text.
	body := p.StrippedTextReply
	if strings.TrimSpace(body) == "" {
		body = p.TextBody
	}

	email := p.FromFull.Email
	if email == "" {
		email = p.From
	}

	// Postmark's MessageID is unique per inbound message and stable across
	// webhook retries; fall back to the original header if absent.
	msgID := p.MessageID
	if msgID == "" {
		msgID = headers["message-id"]
	}

	// DKIM is log-only in v1: default pass, flag only an explicit failure so we
	// never warn on a provider that omits Authentication-Results.
	authResults := strings.ToLower(headers["authentication-results"])
	spam := strings.HasPrefix(strings.ToLower(strings.TrimSpace(headers["x-spam-status"])), "yes")

	return NormalizedInbound{
		MessageID: strings.TrimSpace(msgID),
		From:      Addr{Email: strings.TrimSpace(email), Name: strings.TrimSpace(p.FromFull.Name)},
		Subject:   p.Subject,
		Body:      body,
		Headers:   headers,
		DKIMPass:  !strings.Contains(authResults, "dkim=fail"),
		SpamFlag:  spam,
	}
}

// secretMatches compares in constant time so the webhook isn't a timing oracle.
func secretMatches(provided, secret string) bool {
	return subtle.ConstantTimeCompare([]byte(provided), []byte(secret)) == 1
}

// parseAllowedNets turns config strings (single IPs or CIDRs) into networks. A
// bare IP becomes a /32 (or /128 for IPv6); unparseable entries are dropped.
func parseAllowedNets(entries []string) []*net.IPNet {
	var out []*net.IPNet
	for _, c := range entries {
		c = strings.TrimSpace(c)
		if c == "" {
			continue
		}
		if !strings.Contains(c, "/") {
			if strings.Contains(c, ":") {
				c += "/128"
			} else {
				c += "/32"
			}
		}
		if _, n, err := net.ParseCIDR(c); err == nil {
			out = append(out, n)
		}
	}
	return out
}

// ipInAllowlist reports whether ipStr falls in any allowed network. An empty
// allowlist allows everything (the secret is then the only gate).
func ipInAllowlist(ipStr string, nets []*net.IPNet) bool {
	if len(nets) == 0 {
		return true
	}
	ip := net.ParseIP(ipStr)
	if ip == nil {
		return false
	}
	for _, n := range nets {
		if n.Contains(ip) {
			return true
		}
	}
	return false
}
