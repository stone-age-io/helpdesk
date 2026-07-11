package subjects

import "testing"

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
		{"helpdesk.tickets.create", "", "", false},            // customer-side shape, no org
		{"helpdesk.org123.tickets", "", "", false},            // missing verb
		{"kiosk.org123.tickets.create", "", "", false},        // wrong app
		{"helpdesk.org123.events.create", "", "", false},      // wrong subtree
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
