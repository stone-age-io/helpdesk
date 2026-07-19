package notifications_test

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"

	"github.com/pocketbase/dbx"

	"github.com/stone-age-io/helpdesk/internal/notifications"
)

// fakePublisher implements notifications.Publisher, capturing every publish so
// the tests can assert subject + envelope without a broker (sibling
// convention: no NATS in tests).
type fakePublisher struct {
	mu   sync.Mutex
	msgs []publishedMsg
	err  error // returned from Publish when set, to exercise the failure path
}

type publishedMsg struct {
	subject string
	data    []byte
	msgID   string
}

func (f *fakePublisher) Publish(_ context.Context, subject string, data []byte, msgID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.msgs = append(f.msgs, publishedMsg{subject: subject, data: data, msgID: msgID})
	return f.err
}

func (f *fakePublisher) captured() []publishedMsg {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]publishedMsg(nil), f.msgs...)
}

// enablePublish flips a template row's flags (publish_nats, enabled) for a test.
func (h *harness) enablePublish(t *testing.T, eventType string, set map[string]any) {
	t.Helper()
	rec, err := h.app.FindFirstRecordByFilter(
		"notification_templates", "event_type = {:t}", dbx.Params{"t": eventType})
	if err != nil {
		t.Fatalf("find template %q: %v", eventType, err)
	}
	for k, v := range set {
		rec.Set(k, v)
	}
	if err := h.app.Save(rec); err != nil {
		t.Fatalf("save template %q: %v", eventType, err)
	}
}

func TestPublishTicketCreatedEnvelope(t *testing.T) {
	h := setupHarness(t)
	fake := &fakePublisher{}
	h.notifier.SetPublisher(fake)
	h.enablePublish(t, "ticket.created", map[string]any{"publish_nats": true})

	h.createTicket(t, map[string]any{"requester": h.requester.Id})
	h.drain(t)

	msgs := fake.captured()
	if len(msgs) != 1 {
		t.Fatalf("expected 1 publish, got %d", len(msgs))
	}
	m := msgs[0]

	wantSubject := "helpdesk." + h.customer.Id + ".events.ticket.created"
	if m.subject != wantSubject {
		t.Errorf("subject = %q, want %q", m.subject, wantSubject)
	}
	if !strings.HasPrefix(m.msgID, "ticket.created:") {
		t.Errorf("msgID = %q, want ticket.created: prefix", m.msgID)
	}

	var env notifications.EventEnvelope
	if err := json.Unmarshal(m.data, &env); err != nil {
		t.Fatalf("envelope is not valid JSON: %v", err)
	}
	if env.Schema != notifications.EnvelopeSchema || env.Version != notifications.EnvelopeVersion {
		t.Errorf("schema/version = %q/%d", env.Schema, env.Version)
	}
	if env.EventType != "ticket.created" {
		t.Errorf("event_type = %q", env.EventType)
	}
	if env.Customer.ID != h.customer.Id || env.Customer.Name != "Acme Corp" {
		t.Errorf("customer = %+v", env.Customer)
	}
	if env.Ticket.Number == 0 || env.Ticket.Title != "Pump fault" {
		t.Errorf("ticket = %+v", env.Ticket)
	}
	if env.OccurredAt == "" {
		t.Error("occurred_at is empty")
	}
}

func TestPublishDisabledByDefault(t *testing.T) {
	h := setupHarness(t)
	fake := &fakePublisher{}
	h.notifier.SetPublisher(fake)
	// No enablePublish — publish_nats defaults false on every seeded template.

	h.createTicket(t, map[string]any{"requester": h.requester.Id})
	h.drain(t)

	if got := fake.captured(); len(got) != 0 {
		t.Fatalf("expected no publishes when publish_nats is off, got %d", len(got))
	}
}

// TestChannelsIndependent proves the two channels are gated separately: email
// off + NATS on publishes the event but sends no mail.
func TestChannelsIndependent(t *testing.T) {
	h := setupHarness(t)
	fake := &fakePublisher{}
	h.notifier.SetPublisher(fake)
	h.enablePublish(t, "ticket.created", map[string]any{"enabled": false, "publish_nats": true})

	h.createTicket(t, map[string]any{"requester": h.requester.Id})
	mail := h.drain(t)

	if len(mail) != 0 {
		t.Errorf("email disabled but %d recipients mailed: %v", len(mail), mail)
	}
	if got := fake.captured(); len(got) != 1 {
		t.Fatalf("expected 1 publish with NATS on, got %d", len(got))
	}
}

func TestPublishStatusChangedCarriesChange(t *testing.T) {
	h := setupHarness(t)
	fake := &fakePublisher{}
	h.notifier.SetPublisher(fake)
	h.enablePublish(t, "ticket.status_changed", map[string]any{"publish_nats": true})

	ticket := h.createTicket(t, map[string]any{"requester": h.requester.Id})
	h.drain(t) // discard the (unpublished) create

	rec, err := h.app.FindRecordById("tickets", ticket.Id)
	if err != nil {
		t.Fatalf("refetch ticket: %v", err)
	}
	rec.Set("status", "in_progress")
	if err := h.app.Save(rec); err != nil {
		t.Fatalf("update status: %v", err)
	}
	h.drain(t)

	msgs := fake.captured()
	if len(msgs) != 1 {
		t.Fatalf("expected 1 status_changed publish, got %d", len(msgs))
	}
	var env notifications.EventEnvelope
	if err := json.Unmarshal(msgs[0].data, &env); err != nil {
		t.Fatalf("bad envelope: %v", err)
	}
	if env.Change == nil {
		t.Fatal("status_changed envelope missing change block")
	}
	if env.Change.Field != "status" || env.Change.From != "open" || env.Change.To != "in_progress" {
		t.Errorf("change = %+v, want status open→in_progress", env.Change)
	}
}

// TestVisitCompletedSeededNATSOnly proves migration 1817 lands the template in
// its NATS-only state: email off, wire channel on.
func TestVisitCompletedSeededNATSOnly(t *testing.T) {
	h := setupHarness(t)
	rec, err := h.app.FindFirstRecordByFilter(
		"notification_templates", "event_type = {:t}",
		dbx.Params{"t": notifications.EventTypeVisitCompleted})
	if err != nil {
		t.Fatalf("visit.completed template not seeded: %v", err)
	}
	if rec.GetBool("enabled") {
		t.Error("visit.completed should ship email-disabled")
	}
	if !rec.GetBool("publish_nats") {
		t.Error("visit.completed should ship with publish_nats enabled")
	}
}

// TestVisitCompletedPublishesNATSOnly drives a visit to completed and asserts it
// publishes a rich envelope (with completed_at + technician) while mailing
// nobody — the whole point of the NATS-only channel. No enablePublish call: the
// seeded template already carries publish_nats=true.
func TestVisitCompletedPublishesNATSOnly(t *testing.T) {
	h := setupHarness(t)
	fake := &fakePublisher{}
	h.notifier.SetPublisher(fake)

	ticket := h.createTicket(t, map[string]any{"requester": h.requester.Id})
	visit := h.createVisit(t, ticket, map[string]any{
		"status": "scheduled", "assignee": h.agent.Id,
		"scheduled_at": "2026-07-14 14:00:00.000Z",
	})
	h.drain(t) // discard the scheduled event (visit.scheduled has publish_nats off)

	if got := fake.captured(); len(got) != 0 {
		t.Fatalf("scheduling published %d messages; expected 0 before completion", len(got))
	}

	h.updateVisit(t, visit.Id, map[string]any{
		"status":       "completed",
		"completed_at": "2026-07-14 15:30:00.000Z",
	})
	mail := h.drain(t)

	if len(mail) != 0 {
		t.Errorf("visit.completed is NATS-only but mailed %v", mail)
	}
	msgs := fake.captured()
	if len(msgs) != 1 {
		t.Fatalf("expected 1 visit.completed publish, got %d", len(msgs))
	}
	msg := msgs[0]
	if want := "helpdesk." + h.customer.Id + ".events.visit.completed"; msg.subject != want {
		t.Errorf("subject = %q, want %q", msg.subject, want)
	}

	var env notifications.EventEnvelope
	if err := json.Unmarshal(msg.data, &env); err != nil {
		t.Fatalf("envelope is not valid JSON: %v", err)
	}
	if env.EventType != notifications.EventTypeVisitCompleted {
		t.Errorf("event_type = %q", env.EventType)
	}
	if env.Visit == nil {
		t.Fatal("visit.completed envelope missing visit block")
	}
	if env.Visit.CompletedAt == "" {
		t.Errorf("envelope missing completed_at: %+v", env.Visit)
	}
	if env.Visit.AssigneeName == "" {
		t.Errorf("envelope missing technician name: %+v", env.Visit)
	}
}

// TestPublishFailureDoesNotBlockEmail confirms a publish error is contained:
// the email on the same event still goes out.
func TestPublishFailureDoesNotBlockEmail(t *testing.T) {
	h := setupHarness(t)
	fake := &fakePublisher{err: context.DeadlineExceeded}
	h.notifier.SetPublisher(fake)
	h.enablePublish(t, "ticket.created", map[string]any{"publish_nats": true})

	h.createTicket(t, map[string]any{"requester": h.requester.Id})
	mail := h.drain(t)

	if len(mail) == 0 {
		t.Error("publish failed but email was suppressed too — channels not independent")
	}
	if got := fake.captured(); len(got) != 1 {
		t.Errorf("expected 1 publish attempt, got %d", len(got))
	}
}
