// DB-backed tests live in an external test package: the schema migrations
// import notifications (for defaults + seeding), so an in-package test that
// pulled in testutil→migrations would form an import cycle.
package notifications_test

import (
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/mailer"

	"github.com/pocketbase/dbx"

	"github.com/stone-age-io/helpdesk/internal/notifications"
	"github.com/stone-age-io/helpdesk/internal/testutil"
	"github.com/stone-age-io/helpdesk/internal/tickets"
	"github.com/stone-age-io/helpdesk/internal/visits"
)

// mailCapture binds OnMailerSend and swallows sends so no SMTP/sendmail is
// ever attempted; messages are recorded for assertions.
type mailCapture struct {
	mu   sync.Mutex
	msgs []*mailer.Message
}

func captureMail(app *pocketbase.PocketBase) *mailCapture {
	c := &mailCapture{}
	app.OnMailerSend().BindFunc(func(e *core.MailerEvent) error {
		c.mu.Lock()
		defer c.mu.Unlock()
		c.msgs = append(c.msgs, e.Message)
		return nil // swallow: do not call e.Next(), no real delivery
	})
	return c
}

func (c *mailCapture) recipients() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	var out []string
	for _, m := range c.msgs {
		for _, to := range m.To {
			out = append(out, to.Address)
		}
	}
	return out
}

func (c *mailCapture) reset() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.msgs = nil
}

// harness boots the full stack: real app + migrations, ticket hooks,
// notification hooks, mail capture. Seeded records: one customer, one
// requester, one agent (plus the migration's bootstrap admin).
type harness struct {
	app       *pocketbase.PocketBase
	notifier  *notifications.Notifier
	capture   *mailCapture
	customer  *core.Record
	requester *core.Record
	agent     *core.Record
}

func setupHarness(t *testing.T) *harness {
	t.Helper()
	app := testutil.SetupApp(t)
	tickets.Register(app)
	visits.Register(app)
	n := notifications.New(app)
	t.Cleanup(func() { n.WaitInFlight(5 * time.Second) })
	notifications.RegisterHooks(app, n)
	h := &harness{app: app, notifier: n, capture: captureMail(app)}

	custCol, _ := app.FindCollectionByNameOrId("customers")
	h.customer = core.NewRecord(custCol)
	h.customer.Set("name", "Acme Corp")
	h.customer.Set("active", true)
	if err := app.Save(h.customer); err != nil {
		t.Fatalf("save customer: %v", err)
	}

	userCol, _ := app.FindCollectionByNameOrId("users")
	h.requester = core.NewRecord(userCol)
	h.requester.Set("name", "Rita Requester")
	h.requester.Set("email", "rita@acme.example")
	h.requester.Set("customer", h.customer.Id)
	h.requester.Set("active", true)
	h.requester.SetPassword("test-password-123")
	if err := app.Save(h.requester); err != nil {
		t.Fatalf("save requester: %v", err)
	}

	staffCol, _ := app.FindCollectionByNameOrId("staff")
	h.agent = core.NewRecord(staffCol)
	h.agent.Set("name", "Sam Staff")
	h.agent.Set("email", "sam@816tech.example")
	h.agent.Set("role", "agent")
	h.agent.Set("active", true)
	h.agent.SetPassword("test-password-123")
	if err := app.Save(h.agent); err != nil {
		t.Fatalf("save agent: %v", err)
	}
	return h
}

// drain waits for async sends and returns the recipients seen since the
// last drain.
func (h *harness) drain(t *testing.T) []string {
	t.Helper()
	if !h.notifier.WaitInFlight(5 * time.Second) {
		t.Fatal("notifier goroutines did not finish")
	}
	got := h.capture.recipients()
	h.capture.reset()
	return got
}

func (h *harness) createTicket(t *testing.T, set map[string]any) *core.Record {
	t.Helper()
	col, _ := h.app.FindCollectionByNameOrId("tickets")
	rec := core.NewRecord(col)
	rec.Set("customer", h.customer.Id)
	rec.Set("title", "Pump fault")
	for k, v := range set {
		rec.Set(k, v)
	}
	if err := h.app.Save(rec); err != nil {
		t.Fatalf("save ticket: %v", err)
	}
	return rec
}

func (h *harness) createVisit(t *testing.T, ticket *core.Record, set map[string]any) *core.Record {
	t.Helper()
	col, _ := h.app.FindCollectionByNameOrId("visits")
	rec := core.NewRecord(col)
	rec.Set("ticket", ticket.Id)
	for k, v := range set {
		rec.Set(k, v)
	}
	if err := h.app.Save(rec); err != nil {
		t.Fatalf("save visit: %v", err)
	}
	return rec
}

// updateVisit re-fetches before mutating so Record.Original() reflects the
// committed DB state (same pattern as the ticket update tests).
func (h *harness) updateVisit(t *testing.T, id string, set map[string]any) {
	t.Helper()
	rec, err := h.app.FindRecordById("visits", id)
	if err != nil {
		t.Fatalf("find visit: %v", err)
	}
	for k, v := range set {
		rec.Set(k, v)
	}
	if err := h.app.Save(rec); err != nil {
		t.Fatalf("update visit: %v", err)
	}
}

func (h *harness) createComment(t *testing.T, ticket *core.Record, set map[string]any) {
	t.Helper()
	col, _ := h.app.FindCollectionByNameOrId("ticket_comments")
	rec := core.NewRecord(col)
	rec.Set("ticket", ticket.Id)
	rec.Set("body", "comment body")
	for k, v := range set {
		rec.Set(k, v)
	}
	if err := h.app.Save(rec); err != nil {
		t.Fatalf("save comment: %v", err)
	}
}

func TestTicketCreatedNotifiesRequesterAndStaff(t *testing.T) {
	h := setupHarness(t)
	h.createTicket(t, map[string]any{"requester": h.requester.Id})
	got := h.drain(t)
	for _, want := range []string{"rita@acme.example", "sam@816tech.example", "admin@helpdesk.local"} {
		if !slices.Contains(got, want) {
			t.Errorf("ticket.created missing %s (got %v)", want, got)
		}
	}

	logs, err := h.app.FindRecordsByFilter(notifications.SendLogCollectionName, "event_type = 'ticket.created'", "", 0, 0)
	if err != nil {
		t.Fatalf("read send log: %v", err)
	}
	if len(logs) != len(got) {
		t.Errorf("send log rows: got %d, want %d (one per recipient)", len(logs), len(got))
	}
	for _, l := range logs {
		if l.GetString("status") != notifications.SendStatusSent {
			t.Errorf("send log status: got %q, want sent (err=%q)", l.GetString("status"), l.GetString("error"))
		}
		if l.GetString("payload_summary") != "#1 · Acme Corp" {
			t.Errorf("payload summary: got %q", l.GetString("payload_summary"))
		}
	}
}

func TestMachineTicketWithoutRequesterStillNotifiesStaff(t *testing.T) {
	h := setupHarness(t)
	h.createTicket(t, map[string]any{"source": "nats"})
	got := h.drain(t)
	if slices.Contains(got, "rita@acme.example") {
		t.Errorf("requester mailed on machine ticket: %v", got)
	}
	if !slices.Contains(got, "sam@816tech.example") {
		t.Errorf("staff not mailed on machine ticket: %v", got)
	}
}

func TestStatusChangeNotifiesRequester(t *testing.T) {
	h := setupHarness(t)
	ticket := h.createTicket(t, map[string]any{"requester": h.requester.Id})
	h.drain(t)

	loaded, _ := h.app.FindRecordById("tickets", ticket.Id)
	loaded.Set("status", "resolved")
	if err := h.app.Save(loaded); err != nil {
		t.Fatalf("update: %v", err)
	}
	got := h.drain(t)
	if !slices.Contains(got, "rita@acme.example") {
		t.Errorf("status change did not mail requester: %v", got)
	}
}

// A Suppress'd update is silent — the same status change that mails the
// requester above sends nothing when the quiet flag is set.
func TestSuppressedUpdateSendsNoEmail(t *testing.T) {
	h := setupHarness(t)
	ticket := h.createTicket(t, map[string]any{"requester": h.requester.Id})
	h.drain(t)

	loaded, _ := h.app.FindRecordById("tickets", ticket.Id)
	loaded.Set("status", "resolved")
	notifications.Suppress(loaded)
	if err := h.app.Save(loaded); err != nil {
		t.Fatalf("update: %v", err)
	}
	if got := h.drain(t); len(got) != 0 {
		t.Errorf("suppressed update should send no mail, got %v", got)
	}
}

// A requester replying on a resolved ticket reopens it, but the reopen is
// silent (the comment already carried the news): the requester is not mailed
// a redundant status-change, and the ticket ends up open.
func TestRequesterCommentReopenIsSilent(t *testing.T) {
	h := setupHarness(t)
	ticket := h.createTicket(t, map[string]any{"requester": h.requester.Id})
	h.drain(t)

	resolved, _ := h.app.FindRecordById("tickets", ticket.Id)
	resolved.Set("status", "resolved")
	if err := h.app.Save(resolved); err != nil {
		t.Fatalf("resolve: %v", err)
	}
	h.drain(t)

	h.createComment(t, ticket, map[string]any{"author_user": h.requester.Id})
	got := h.drain(t)
	if slices.Contains(got, "rita@acme.example") {
		t.Errorf("reopen should not mail the requester who commented: %v", got)
	}
	if s := statusOfTicket(t, h.app, ticket.Id); s != "open" {
		t.Errorf("requester comment should reopen resolved ticket, got %q", s)
	}
}

func statusOfTicket(t *testing.T, app *pocketbase.PocketBase, id string) string {
	t.Helper()
	rec, err := app.FindRecordById("tickets", id)
	if err != nil {
		t.Fatalf("reload ticket: %v", err)
	}
	return rec.GetString("status")
}

func TestNoOpUpdateSendsNothing(t *testing.T) {
	h := setupHarness(t)
	ticket := h.createTicket(t, map[string]any{"requester": h.requester.Id})
	h.drain(t)

	loaded, _ := h.app.FindRecordById("tickets", ticket.Id)
	loaded.Set("body", "extra detail, same status")
	if err := h.app.Save(loaded); err != nil {
		t.Fatalf("update: %v", err)
	}
	if got := h.drain(t); len(got) != 0 {
		t.Errorf("cosmetic update fired notifications: %v", got)
	}
}

func TestAssignmentNotifiesAssignee(t *testing.T) {
	h := setupHarness(t)
	ticket := h.createTicket(t, nil)
	h.drain(t)

	loaded, _ := h.app.FindRecordById("tickets", ticket.Id)
	loaded.Set("assignee", h.agent.Id)
	if err := h.app.Save(loaded); err != nil {
		t.Fatalf("assign: %v", err)
	}
	got := h.drain(t)
	if !slices.Contains(got, "sam@816tech.example") {
		t.Errorf("assignment did not mail assignee: %v", got)
	}
	if slices.Contains(got, "admin@helpdesk.local") {
		t.Errorf("assignment mailed all staff: %v", got)
	}
}

func TestStaffCommentNotifiesRequesterOnly(t *testing.T) {
	h := setupHarness(t)
	ticket := h.createTicket(t, map[string]any{"requester": h.requester.Id, "assignee": h.agent.Id})
	h.drain(t)

	h.createComment(t, ticket, map[string]any{"author_staff": h.agent.Id})
	got := h.drain(t)
	if !slices.Contains(got, "rita@acme.example") {
		t.Errorf("staff comment did not mail requester: %v", got)
	}
	if slices.Contains(got, "sam@816tech.example") {
		t.Errorf("staff comment mailed the assignee side: %v", got)
	}
}

func TestRequesterCommentNotifiesAssigneeOnly(t *testing.T) {
	h := setupHarness(t)
	ticket := h.createTicket(t, map[string]any{"requester": h.requester.Id, "assignee": h.agent.Id})
	h.drain(t)

	h.createComment(t, ticket, map[string]any{"author_user": h.requester.Id})
	got := h.drain(t)
	if !slices.Contains(got, "sam@816tech.example") {
		t.Errorf("requester comment did not mail assignee: %v", got)
	}
	if slices.Contains(got, "rita@acme.example") {
		t.Errorf("requester comment mailed its own author: %v", got)
	}
}

func TestInternalCommentSendsNothing(t *testing.T) {
	h := setupHarness(t)
	ticket := h.createTicket(t, map[string]any{"requester": h.requester.Id, "assignee": h.agent.Id})
	h.drain(t)

	h.createComment(t, ticket, map[string]any{"author_staff": h.agent.Id, "internal": true})
	if got := h.drain(t); len(got) != 0 {
		t.Errorf("internal comment fired notifications: %v", got)
	}
}

func TestVisitScheduledNotifiesRequesterAndTechnician(t *testing.T) {
	h := setupHarness(t)
	ticket := h.createTicket(t, map[string]any{"requester": h.requester.Id})
	h.drain(t)

	col, _ := h.app.FindCollectionByNameOrId("visits")
	rec := core.NewRecord(col)
	rec.Set("ticket", ticket.Id)
	rec.Set("assignee", h.agent.Id)
	rec.Set("scheduled_at", "2026-07-14 14:00:00.000Z")
	rec.Set("status", "scheduled")
	if err := h.app.Save(rec); err != nil {
		t.Fatalf("save visit: %v", err)
	}
	got := h.drain(t)
	if !slices.Contains(got, "rita@acme.example") {
		t.Errorf("visit did not mail requester: %v", got)
	}
	if !slices.Contains(got, "sam@816tech.example") {
		t.Errorf("visit did not mail technician: %v", got)
	}
}

func TestRequestedVisitSendsNothing(t *testing.T) {
	h := setupHarness(t)
	ticket := h.createTicket(t, map[string]any{"requester": h.requester.Id})
	h.drain(t)

	h.createVisit(t, ticket, map[string]any{"status": "requested"})
	if got := h.drain(t); len(got) != 0 {
		t.Errorf("requested visit sent mail to %v", got)
	}
}

func TestRequestedToScheduledSendsVisitScheduled(t *testing.T) {
	h := setupHarness(t)
	ticket := h.createTicket(t, map[string]any{"requester": h.requester.Id})
	visit := h.createVisit(t, ticket, map[string]any{"status": "requested"})
	h.drain(t)

	h.updateVisit(t, visit.Id, map[string]any{
		"status":       "scheduled",
		"assignee":     h.agent.Id,
		"scheduled_at": "2026-07-14 14:00:00.000Z",
	})
	got := h.drain(t)
	if !slices.Contains(got, "rita@acme.example") {
		t.Errorf("scheduling did not mail requester: %v", got)
	}
	if !slices.Contains(got, "sam@816tech.example") {
		t.Errorf("scheduling did not mail technician: %v", got)
	}
	assertSendLog(t, h, notifications.EventTypeVisitScheduled)
}

func TestRescheduleSendsVisitRescheduled(t *testing.T) {
	h := setupHarness(t)
	ticket := h.createTicket(t, map[string]any{"requester": h.requester.Id})
	visit := h.createVisit(t, ticket, map[string]any{
		"status": "scheduled", "assignee": h.agent.Id,
		"scheduled_at": "2026-07-14 14:00:00.000Z",
	})
	h.drain(t)

	h.updateVisit(t, visit.Id, map[string]any{"scheduled_at": "2026-07-16 09:00:00.000Z"})
	got := h.drain(t)
	if !slices.Contains(got, "rita@acme.example") || !slices.Contains(got, "sam@816tech.example") {
		t.Errorf("reschedule did not mail both sides: %v", got)
	}
	assertSendLog(t, h, notifications.EventTypeVisitRescheduled)
}

func TestScheduledToCanceledSendsVisitCanceled(t *testing.T) {
	h := setupHarness(t)
	ticket := h.createTicket(t, map[string]any{"requester": h.requester.Id})
	visit := h.createVisit(t, ticket, map[string]any{
		"status": "scheduled", "assignee": h.agent.Id,
		"scheduled_at": "2026-07-14 14:00:00.000Z",
	})
	h.drain(t)

	h.updateVisit(t, visit.Id, map[string]any{"status": "canceled"})
	got := h.drain(t)
	if !slices.Contains(got, "rita@acme.example") {
		t.Errorf("cancelation did not mail requester: %v", got)
	}
	assertSendLog(t, h, notifications.EventTypeVisitCanceled)
}

func TestRequestedToCanceledSendsNothing(t *testing.T) {
	h := setupHarness(t)
	ticket := h.createTicket(t, map[string]any{"requester": h.requester.Id})
	visit := h.createVisit(t, ticket, map[string]any{"status": "requested"})
	h.drain(t)

	h.updateVisit(t, visit.Id, map[string]any{"status": "canceled"})
	if got := h.drain(t); len(got) != 0 {
		t.Errorf("canceling a never-scheduled request sent mail to %v", got)
	}
}

func TestCompletedVisitSendsNothing(t *testing.T) {
	h := setupHarness(t)
	ticket := h.createTicket(t, map[string]any{"requester": h.requester.Id})
	visit := h.createVisit(t, ticket, map[string]any{
		"status": "scheduled", "assignee": h.agent.Id,
		"scheduled_at": "2026-07-14 14:00:00.000Z",
	})
	h.drain(t)

	h.updateVisit(t, visit.Id, map[string]any{"status": "completed"})
	if got := h.drain(t); len(got) != 0 {
		t.Errorf("completing a visit sent mail to %v", got)
	}
}

// assertSendLog verifies at least one send-log row exists for the event —
// the notifier's audit trail, independent of the captured SMTP messages.
func assertSendLog(t *testing.T, h *harness, eventType string) {
	t.Helper()
	rows, err := h.app.FindRecordsByFilter(
		notifications.SendLogCollectionName,
		"event_type = {:t}", "", 0, 0, dbx.Params{"t": eventType})
	if err != nil {
		t.Fatalf("query send log: %v", err)
	}
	if len(rows) == 0 {
		t.Errorf("no send-log rows for %s", eventType)
	}
}

func TestDisabledTemplateSkipsSend(t *testing.T) {
	h := setupHarness(t)
	tmpl, err := h.app.FindFirstRecordByFilter(notifications.CollectionName, "event_type = 'ticket.created'")
	if err != nil {
		t.Fatalf("find template: %v", err)
	}
	tmpl.Set("enabled", false)
	if err := h.app.Save(tmpl); err != nil {
		t.Fatalf("disable template: %v", err)
	}

	h.createTicket(t, map[string]any{"requester": h.requester.Id})
	if got := h.drain(t); len(got) != 0 {
		t.Errorf("disabled template still sent to %v", got)
	}
}

func TestSendIfFirstDedupes(t *testing.T) {
	h := setupHarness(t)
	ctx := notifications.TicketContext{
		Ticket:   notifications.TicketInfo{Number: 7, Title: "flap"},
		Customer: "Acme Corp",
	}

	h.notifier.SendIfFirst(notifications.EventTypeTicketCreated, "ref-1", ctx)
	first := len(h.drain(t))
	if first == 0 {
		t.Fatal("first SendIfFirst delivered nothing")
	}

	h.notifier.SendIfFirst(notifications.EventTypeTicketCreated, "ref-1", ctx)
	if got := len(h.drain(t)); got != 0 {
		t.Errorf("duplicate SendIfFirst delivered again to %d recipients", got)
	}

	// A different ref key fires normally.
	h.notifier.SendIfFirst(notifications.EventTypeTicketCreated, "ref-2", ctx)
	if got := len(h.drain(t)); got == 0 {
		t.Error("different ref key was deduped")
	}
}
