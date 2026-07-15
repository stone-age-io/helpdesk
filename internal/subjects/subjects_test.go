package subjects

import (
	"strings"
	"testing"
)

func TestParseTicketEvent(t *testing.T) {
	s := Default()
	cases := []struct {
		subject string
		org     string
		verb    string
		ok      bool
	}{
		{"helpdesk.org123.tickets.create", "org123", "create", true},
		{"helpdesk.org123.tickets.comment", "org123", "comment", true},
		{"helpdesk.tickets.create", "", "", false},              // customer-side shape, no org
		{"helpdesk.org123.tickets", "", "", false},              // missing verb
		{"kiosk.org123.tickets.create", "", "", false},          // wrong app
		{"helpdesk.org123.events.create", "", "", false},        // wrong subtree
		{"helpdesk.org123.tickets.create.extra", "", "", false}, // too many tokens
	}
	for _, c := range cases {
		org, verb, ok := s.ParseTicketEvent(c.subject)
		if org != c.org || verb != c.verb || ok != c.ok {
			t.Errorf("ParseTicketEvent(%q) = (%q,%q,%v), want (%q,%q,%v)",
				c.subject, org, verb, ok, c.org, c.verb, c.ok)
		}
	}
}

func TestCustomerSidePublishSubject(t *testing.T) {
	if got := Default().TicketCreate(); got != "helpdesk.tickets.create" {
		t.Errorf("TicketCreate() = %q", got)
	}
}

func TestStreamWildcardsCoverHubSubjects(t *testing.T) {
	got := Default().StreamWildcards()
	if len(got) != 1 || got[0] != "helpdesk.*.tickets.>" {
		t.Errorf("StreamWildcards() = %v", got)
	}
}

func TestEventSubject(t *testing.T) {
	s := Default()
	cases := []struct {
		customerID string
		eventType  string
		want       string
	}{
		{"rec8f3x", "ticket.created", "helpdesk.rec8f3x.events.ticket.created"},
		{"rec8f3x", "ticket.status_changed", "helpdesk.rec8f3x.events.ticket.status_changed"},
		{"custABC", "visit.scheduled", "helpdesk.custABC.events.visit.scheduled"},
	}
	for _, c := range cases {
		if got := s.EventSubject(c.customerID, c.eventType); got != c.want {
			t.Errorf("EventSubject(%q,%q) = %q, want %q", c.customerID, c.eventType, got, c.want)
		}
	}
}

func TestEventStreamWildcards(t *testing.T) {
	got := Default().EventStreamWildcards()
	if len(got) != 1 || got[0] != "helpdesk.*.events.>" {
		t.Errorf("EventStreamWildcards() = %v", got)
	}
}

// TestStreamsDisjoint is the tripwire against reintroducing a JetStream subject
// overlap between the ingest and notification streams. Overlap is a property of
// the filters, not of the concrete subjects we intend to publish: NATS rejects
// a second stream whose subject set shares ANY concrete subject with an
// existing one. The ingest filter is helpdesk.*.tickets.> (token 2 is a
// wildcard that swallows anything), so the streams can only be proven disjoint
// at token 3. This asserts that discriminator differs, and that a real
// outbound subject is not matched by the ingest parser (so it can never loop).
func TestStreamsDisjoint(t *testing.T) {
	s := Default()
	ingest := s.StreamWildcards()[0]      // helpdesk.*.tickets.>
	events := s.EventStreamWildcards()[0] // helpdesk.*.events.>

	ingestTok := strings.Split(ingest, ".")
	eventsTok := strings.Split(events, ".")
	if ingestTok[0] != eventsTok[0] {
		t.Fatalf("streams should share the app root token: %q vs %q", ingestTok[0], eventsTok[0])
	}
	if ingestTok[2] == eventsTok[2] {
		t.Fatalf("token 3 must discriminate the streams; both are %q — overlap", ingestTok[2])
	}

	// A concrete outbound subject must not parse as a ticket event (belt-and-
	// suspenders loop guard: even if it reached the ingest consumer, it would
	// be ignored rather than projected as a ticket).
	if _, _, ok := s.ParseTicketEvent(s.EventSubject("rec123", "ticket.created")); ok {
		t.Error("an outbound event subject was parsed as a ticket event — loop risk")
	}
}
