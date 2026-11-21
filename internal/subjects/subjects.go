// Package subjects is the single source of truth for the helpdesk's NATS
// subject grammar, for both sides of the platform boundary. It follows ADR 0008
// in platform-docs: {app}.{scope}.{channel}.{rest}, where the helpdesk's scope
// is the organization at the hub and absent inside a tenant account.
//
// Customer side (inside a customer org's NATS account), apps publish a command:
//
//	helpdesk.cmd.tickets.create
//
// The platform's managed-org export/import delivers those into the operator
// hub account with the org code injected as token 2:
//
//	helpdesk.{orgCode}.cmd.tickets.create
//
// That injection is the provenance mechanism: the subject is rewritten by
// the operator-signed import, so a customer cannot spoof another org's code,
// which is why ingestion parses the org from the subject and NEVER from the
// payload.
//
// Rooting at the literal app token ("helpdesk") keeps the helpdesk's streams
// disjoint from every sibling app's stream on the shared hub account
// (JetStream forbids overlapping stream subjects) and matches the platform
// export's subject token for this app.
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

// The channels (ADR 0008). Token 3 at the hub is the whole disjointness
// guarantee between the two streams: the ingest filter helpdesk.*.cmd.tickets.>
// and the outbound filter helpdesk.*.evt.> match any org at token 2, so only
// token 3 can tell them apart. Commands come in, events go out, and an outbound
// event can never be re-ingested as a ticket. See the TestStreamsDisjoint
// tripwire.
const (
	ChannelCmd = "cmd"
	ChannelEvt = "evt"
)

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
	return fmt.Sprintf("%s.%s.tickets.%s", s.App(), ChannelCmd, VerbCreate)
}

// StreamWildcards is the HELPDESK_EVENTS stream's subject set and the
// durable consumer's filter: every ticket verb from every org, hub-side.
func (s Subjects) StreamWildcards() []string {
	return []string{fmt.Sprintf("%s.*.%s.tickets.>", s.App(), ChannelCmd)}
}

// EventSubject builds the hub-side outbound subject for one notification
// event:
//
//	helpdesk.{customerCode}.evt.{event_type}
//
// customerCode is customers.code, the ecosystem's tenant token: the same
// handle token 2 carries on the way IN and the same one the platform stamps on
// its operator-signed rewrite (ADR 0002 in platform-docs). Both directions of
// the boundary name a tenant the same way.
//
// The code is optional, so a customer without one cannot be published for; the
// notifier skips and logs rather than falling back to the record id, because a
// token that is sometimes a shared code and sometimes a local key is not a
// token.
//
// eventType is the notification event type ("ticket.created",
// "visit.scheduled"); its embedded dot supplies the trailing domain.verb
// tokens, so no separate mapping is needed. platform_org_id is deliberately NOT
// in the subject; it rides the payload.
func (s Subjects) EventSubject(customerCode, eventType string) string {
	return fmt.Sprintf("%s.%s.%s.%s", s.App(), customerCode, ChannelEvt, eventType)
}

// EventStreamWildcards is the HELPDESK_NOTIFICATIONS stream's subject set: every
// outbound event for every customer, hub-side. Disjoint from StreamWildcards()
// at token 3 (evt vs cmd).
func (s Subjects) EventStreamWildcards() []string {
	return []string{fmt.Sprintf("%s.*.%s.>", s.App(), ChannelEvt)}
}

// ParseTicketEvent splits a hub-side subject into its org code and verb:
//
//	{app}.{orgCode}.cmd.tickets.{verb} -> orgCode, verb, true
//
// ok is false for anything else (wrong app, wrong shape). The org code comes
// exclusively from here: it is the signed, unforgeable part of the command.
func (s Subjects) ParseTicketEvent(subject string) (orgID, verb string, ok bool) {
	parts := strings.Split(subject, ".")
	if len(parts) != 5 || parts[0] != s.App() || parts[1] == "" ||
		parts[2] != ChannelCmd || parts[3] != "tickets" || parts[4] == "" {
		return "", "", false
	}
	return parts[1], parts[4], true
}
