package notifications

import (
	"strings"
	"testing"
	"time"
)

// sampleContext returns a fully-populated TicketContext so every default
// template has the fields it references.
func sampleContext() TicketContext {
	return TicketContext{
		Ticket: TicketInfo{
			ID:        "t1",
			Number:    42,
			Title:     "Pump fault on line 3",
			Body:      "Vibration sensor reports repeated overcurrent.",
			Status:    "in_progress",
			OldStatus: "open",
			Priority:  "high",
			Source:    "nats",
			URL:       "https://helpdesk.example.com/t/t1",
		},
		Customer:  "Acme Corp",
		Requester: PersonInfo{Name: "Rita Requester", Email: "rita@acme.example"},
		Assignee:  PersonInfo{Name: "Sam Staff", Email: "sam@816tech.example"},
		Comment:   &CommentInfo{AuthorName: "Sam Staff", Body: "Heading out tomorrow.", ByStaff: true},
		Visit:     &VisitInfo{ScheduledAt: "2026-07-14 14:00:00.000Z", AssigneeName: "Sam Staff", Notes: "Bring spare motor"},
	}
}

// TestDefaultsRenderAgainstTicketContext is the regression test that
// catches "operator field reference broke" — every seeded default must
// parse and render cleanly against a representative TicketContext.
func TestDefaultsRenderAgainstTicketContext(t *testing.T) {
	ctx := sampleContext()
	for _, et := range SeededEventTypes() {
		subjectSrc, bodySrc, ok := Defaults(et)
		if !ok {
			t.Fatalf("no defaults for seeded event type %q", et)
		}
		subject, body, err := Render(subjectSrc, bodySrc, ctx)
		if err != nil {
			t.Fatalf("%s: render failed: %v", et, err)
		}
		if !strings.Contains(subject, "42") {
			t.Errorf("%s: subject missing ticket number: %q", et, subject)
		}
		if !strings.Contains(body, ctx.Ticket.URL) {
			t.Errorf("%s: body missing ticket URL: %q", et, body)
		}
	}
}

func TestStatusChangedMentionsBothStates(t *testing.T) {
	_, body, err := Render(DefaultTicketStatusChangedSubject, DefaultTicketStatusChangedBody, sampleContext())
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if !strings.Contains(body, "in progress") || !strings.Contains(body, "was open") {
		t.Errorf("status change body missing transition: %q", body)
	}
}

func TestVisitScheduledFormatsTimestamp(t *testing.T) {
	subject, _, err := Render(DefaultVisitScheduledSubject, DefaultVisitScheduledBody, sampleContext())
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if strings.Contains(subject, "2026-07-14 14:00") {
		t.Errorf("visit subject shows raw datetime, formatTime not applied: %q", subject)
	}
	if !strings.Contains(subject, "2026") {
		t.Errorf("visit subject missing formatted date: %q", subject)
	}
}

func TestValidateTemplates(t *testing.T) {
	if err := ValidateTemplates(DefaultTicketCreatedSubject, DefaultTicketCreatedBody); err != nil {
		t.Errorf("defaults rejected: %v", err)
	}
	if err := ValidateTemplates("{{ .Bogus ", "body"); err == nil {
		t.Error("expected parse error for unclosed action in subject")
	}
	if err := ValidateTemplates("ok", "{{range .X}}"); err == nil {
		t.Error("expected parse error for unterminated range in body")
	}
}

func TestSuppressionBlanksProviderEmails(t *testing.T) {
	ctx := sampleContext()
	if ctx.RequesterEmail() == "" || ctx.AssigneeEmail() == "" {
		t.Fatal("unsuppressed context should expose both emails")
	}
	ctx.suppressRequester = true
	ctx.suppressAssignee = true
	if ctx.RequesterEmail() != "" || ctx.AssigneeEmail() != "" {
		t.Error("suppressed context leaked an email")
	}
}

func TestNilNotifierIsNoOp(t *testing.T) {
	var n *Notifier
	n.Send(EventTypeTicketCreated, sampleContext())
	n.SendIfFirst(EventTypeTicketCreated, "x", sampleContext())
	if !n.WaitInFlight(time.Second) {
		t.Error("nil notifier WaitInFlight should return true")
	}
	if _, err := n.PruneSendLog("2026-01-01"); err != nil {
		t.Errorf("nil notifier prune: %v", err)
	}
}
