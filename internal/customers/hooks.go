// Package customers owns validation glue for the customers collection that
// can't live in collection rules — currently the email_domain guard that keeps
// domain-based email routing tenant-safe.
package customers

import (
	"strings"

	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"

	"github.com/stone-age-io/helpdesk/internal/inbound"
)

// Register binds the customers create/update guard. customers.email_domain
// drives the domain rung of email ingestion's customer resolution (a sender at
// acme.com resolves to Acme), so two things must hold on every write:
//
//   - it is normalized (lower-cased, trimmed) so matching against a sender's
//     domain is consistent, and
//   - it is never a shared/free provider (gmail.com, outlook.com, …) — mapping
//     one to a single customer would route every sender on that provider into
//     that tenant. The unique index already stops two customers claiming the
//     same domain; this stops the domains nobody may claim.
func Register(app *pocketbase.PocketBase) {
	guard := func(e *core.RecordEvent) error {
		domain := strings.ToLower(strings.TrimSpace(e.Record.GetString("email_domain")))
		if domain != "" && inbound.IsPublicEmailDomain(domain) {
			return apis.NewBadRequestError(
				"email_domain cannot be a shared email provider (e.g. gmail.com); leave it blank for such customers", nil)
		}
		e.Record.Set("email_domain", domain)
		return e.Next()
	}
	app.OnRecordCreate("customers").BindFunc(guard)
	app.OnRecordUpdate("customers").BindFunc(guard)
}
