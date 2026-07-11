// Package subjects is the single source of truth for the helpdesk's NATS
// subject grammar, for both sides of the platform boundary:
//
// Customer side (inside a customer org's NATS account), apps publish:
//
//	helpdesk.tickets.create
//
// The platform's managed-org export/import (platform commit 45ca1e3)
// delivers those into the operator hub account with the org id injected as
// token 2:
//
//	helpdesk.{platformOrgId}.tickets.create
//
// That injection is the provenance mechanism: the subject is rewritten by
// the operator-signed import, so a customer cannot spoof another org's id —
// which is why ingestion parses the org from the subject and NEVER from the
// payload.
//
// Rooting at the literal app token ("helpdesk") keeps the HELPDESK_EVENTS
// stream's subjects disjoint from every sibling app's stream on the shared
// hub account (JetStream forbids overlapping stream subjects) and matches
// the platform export's subject token for this app.
//
// The {verb} position ends the grammar deliberately open: v1 consumes only
// "create", but "comment"/"resolve" can ride the same stream later without
// a subject migration.
package subjects

import (
	"fmt"
	"strings"
)

// DefaultApp is the app-discriminator token. It must match the platform's
// managed-org export for the helpdesk app; override only if the platform
// export changes.
const DefaultApp = "helpdesk"

// VerbCreate is the only ticket verb v1 consumes.
const VerbCreate = "create"

// Subjects builds and parses subjects for one app namespace. The zero value
// is usable and behaves as the default app.
type Subjects struct {
	app string
}

// New returns a Subjects for the given app token; an empty token falls back
// to DefaultApp.
func New(app string) Subjects { return Subjects{app: app} }

// Default returns a Subjects for DefaultApp.
func Default() Subjects { return Subjects{} }

// App is the discriminator token every subject leads with.
func (s Subjects) App() string {
	if s.app == "" {
		return DefaultApp
	}
	return s.app
}

// TicketCreate is the customer-side publish subject (inside the customer
// org's own account, before the export injects the org token).
func (s Subjects) TicketCreate() string {
	return fmt.Sprintf("%s.tickets.%s", s.App(), VerbCreate)
}

// StreamWildcards is the HELPDESK_EVENTS stream's subject set and the
// durable consumer's filter: every ticket verb from every org, hub-side.
func (s Subjects) StreamWildcards() []string {
	return []string{fmt.Sprintf("%s.*.tickets.>", s.App())}
}

// ParseTicketEvent splits a hub-side subject into its org id and verb:
//
//	{app}.{platformOrgId}.tickets.{verb} -> orgID, verb, true
//
// ok is false for anything else (wrong app, wrong shape). The org id comes
// exclusively from here — it is the signed, unforgeable part of the event.
func (s Subjects) ParseTicketEvent(subject string) (orgID, verb string, ok bool) {
	parts := strings.Split(subject, ".")
	if len(parts) != 4 || parts[0] != s.App() || parts[2] != "tickets" || parts[1] == "" || parts[3] == "" {
		return "", "", false
	}
	return parts[1], parts[3], true
}
