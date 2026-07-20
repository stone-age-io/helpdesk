package tickets

import (
	"testing"

	"github.com/pocketbase/pocketbase/core"

	"github.com/stone-age-io/helpdesk/internal/testutil"
)

func seedCustomer(t *testing.T, app core.App, name string) *core.Record {
	t.Helper()
	col, err := app.FindCollectionByNameOrId("customers")
	if err != nil {
		t.Fatalf("find customers: %v", err)
	}
	rec := core.NewRecord(col)
	rec.Set("name", name)
	rec.Set("active", true)
	if err := app.Save(rec); err != nil {
		t.Fatalf("save customer: %v", err)
	}
	return rec
}

func seedRequester(t *testing.T, app core.App, customer *core.Record) *core.Record {
	t.Helper()
	col, _ := app.FindCollectionByNameOrId("users")
	rec := core.NewRecord(col)
	rec.Set("email", "rita@acme.example")
	rec.Set("name", "Rita")
	rec.Set("customer", customer.Id)
	rec.Set("active", true)
	rec.SetPassword("test-password-123")
	if err := app.Save(rec); err != nil {
		t.Fatalf("save requester: %v", err)
	}
	return rec
}

func seedTicket(t *testing.T, app core.App, customer *core.Record, status string) *core.Record {
	t.Helper()
	col, _ := app.FindCollectionByNameOrId("tickets")
	rec := core.NewRecord(col)
	rec.Set("customer", customer.Id)
	rec.Set("title", "pump fault")
	rec.Set("status", status)
	if err := app.Save(rec); err != nil {
		t.Fatalf("save ticket: %v", err)
	}
	return rec
}

func addComment(t *testing.T, app core.App, ticket *core.Record, set map[string]any) {
	t.Helper()
	col, _ := app.FindCollectionByNameOrId("ticket_comments")
	rec := core.NewRecord(col)
	rec.Set("ticket", ticket.Id)
	rec.Set("body", "still broken")
	for k, v := range set {
		rec.Set(k, v)
	}
	if err := app.Save(rec); err != nil {
		t.Fatalf("save comment: %v", err)
	}
}

func statusOf(t *testing.T, app core.App, id string) string {
	t.Helper()
	rec, err := app.FindRecordById("tickets", id)
	if err != nil {
		t.Fatalf("reload ticket: %v", err)
	}
	return rec.GetString("status")
}

func awaitingOf(t *testing.T, app core.App, id string) bool {
	t.Helper()
	rec, err := app.FindRecordById("tickets", id)
	if err != nil {
		t.Fatalf("reload ticket: %v", err)
	}
	return rec.GetBool("awaiting_requester")
}

func seedStaff(t *testing.T, app core.App) *core.Record {
	t.Helper()
	col, _ := app.FindCollectionByNameOrId("staff")
	rec := core.NewRecord(col)
	rec.Set("email", "sam@816tech.example")
	rec.Set("name", "Sam")
	rec.Set("role", "agent")
	rec.Set("active", true)
	rec.SetPassword("test-password-123")
	if err := app.Save(rec); err != nil {
		t.Fatalf("save staff: %v", err)
	}
	return rec
}

func TestRequesterCommentReopensResolvedTicket(t *testing.T) {
	app := testutil.SetupApp(t)
	Register(app)
	customer := seedCustomer(t, app, "Acme")
	requester := seedRequester(t, app, customer)

	ticket := seedTicket(t, app, customer, "resolved")
	addComment(t, app, ticket, map[string]any{"author_user": requester.Id})
	if got := statusOf(t, app, ticket.Id); got != "open" {
		t.Errorf("resolved ticket: requester comment should reopen to open, got %q", got)
	}
}

// A closed ticket is final: even a requester comment (the create rule normally
// blocks it in the API, but a direct save bypasses rules) must not reopen it.
func TestRequesterCommentDoesNotReopenClosedTicket(t *testing.T) {
	app := testutil.SetupApp(t)
	Register(app)
	customer := seedCustomer(t, app, "Acme")
	requester := seedRequester(t, app, customer)

	ticket := seedTicket(t, app, customer, "closed")
	addComment(t, app, ticket, map[string]any{"author_user": requester.Id})
	if got := statusOf(t, app, ticket.Id); got != "closed" {
		t.Errorf("closed ticket: requester comment must NOT reopen, got %q", got)
	}
}

func TestStaffCommentDoesNotReopen(t *testing.T) {
	app := testutil.SetupApp(t)
	Register(app)
	customer := seedCustomer(t, app, "Acme")
	staffCol, _ := app.FindCollectionByNameOrId("staff")
	agent := core.NewRecord(staffCol)
	agent.Set("email", "sam@816tech.example")
	agent.Set("name", "Sam")
	agent.Set("role", "agent")
	agent.Set("active", true)
	agent.SetPassword("test-password-123")
	if err := app.Save(agent); err != nil {
		t.Fatalf("save staff: %v", err)
	}

	ticket := seedTicket(t, app, customer, "resolved")
	addComment(t, app, ticket, map[string]any{"author_staff": agent.Id})
	if got := statusOf(t, app, ticket.Id); got != "resolved" {
		t.Errorf("staff comment should not reopen, got %q", got)
	}
}

func TestInternalNoteDoesNotReopen(t *testing.T) {
	app := testutil.SetupApp(t)
	Register(app)
	customer := seedCustomer(t, app, "Acme")
	requester := seedRequester(t, app, customer)

	ticket := seedTicket(t, app, customer, "resolved")
	addComment(t, app, ticket, map[string]any{"author_user": requester.Id, "internal": true})
	if got := statusOf(t, app, ticket.Id); got != "resolved" {
		t.Errorf("internal note should not reopen, got %q", got)
	}
}

func TestCommentOnOpenTicketStaysOpen(t *testing.T) {
	app := testutil.SetupApp(t)
	Register(app)
	customer := seedCustomer(t, app, "Acme")
	requester := seedRequester(t, app, customer)

	ticket := seedTicket(t, app, customer, "in_progress")
	addComment(t, app, ticket, map[string]any{"author_user": requester.Id})
	if got := statusOf(t, app, ticket.Id); got != "in_progress" {
		t.Errorf("comment on active ticket should not change status, got %q", got)
	}
}

func TestStaffReplyRequestingReplySetsAwaitingRequester(t *testing.T) {
	app := testutil.SetupApp(t)
	Register(app)
	customer := seedCustomer(t, app, "Acme")
	agent := seedStaff(t, app)

	ticket := seedTicket(t, app, customer, "in_progress")
	if awaitingOf(t, app, ticket.Id) {
		t.Fatal("new ticket should not be awaiting the requester")
	}
	addComment(t, app, ticket, map[string]any{"author_staff": agent.Id, "requests_reply": true})
	if !awaitingOf(t, app, ticket.Id) {
		t.Error("a staff comment that requests a reply should mark the ticket awaiting the requester")
	}
}

func TestPlainStaffCommentDoesNotAwaitRequester(t *testing.T) {
	app := testutil.SetupApp(t)
	Register(app)
	customer := seedCustomer(t, app, "Acme")
	agent := seedStaff(t, app)

	ticket := seedTicket(t, app, customer, "in_progress")
	// A staff update without ticking "request a reply" is just an FYI.
	addComment(t, app, ticket, map[string]any{"author_staff": agent.Id})
	if awaitingOf(t, app, ticket.Id) {
		t.Error("a plain staff update should not mark the ticket awaiting the requester")
	}
}

func TestInstallTicketNeverAwaitsRequester(t *testing.T) {
	app := testutil.SetupApp(t)
	Register(app)
	customer := seedCustomer(t, app, "Acme")
	agent := seedStaff(t, app)

	col, _ := app.FindCollectionByNameOrId("tickets")
	ticket := core.NewRecord(col)
	ticket.Set("customer", customer.Id)
	ticket.Set("title", "camera install")
	ticket.Set("type", "install")
	ticket.Set("status", "in_progress")
	if err := app.Save(ticket); err != nil {
		t.Fatalf("save install ticket: %v", err)
	}
	// Even an explicit request-a-reply doesn't flag proactive field work.
	addComment(t, app, ticket, map[string]any{"author_staff": agent.Id, "requests_reply": true})
	if awaitingOf(t, app, ticket.Id) {
		t.Error("install tickets are excluded from the needs-reply flag")
	}
}

func TestInternalStaffNoteDoesNotAwaitRequester(t *testing.T) {
	app := testutil.SetupApp(t)
	Register(app)
	customer := seedCustomer(t, app, "Acme")
	agent := seedStaff(t, app)

	ticket := seedTicket(t, app, customer, "in_progress")
	addComment(t, app, ticket, map[string]any{"author_staff": agent.Id, "internal": true})
	if awaitingOf(t, app, ticket.Id) {
		t.Error("an internal note is not a request for a reply")
	}
}

func TestStaffCommentOnResolvedDoesNotAwaitRequester(t *testing.T) {
	app := testutil.SetupApp(t)
	Register(app)
	customer := seedCustomer(t, app, "Acme")
	agent := seedStaff(t, app)

	ticket := seedTicket(t, app, customer, "resolved")
	addComment(t, app, ticket, map[string]any{"author_staff": agent.Id, "requests_reply": true})
	if awaitingOf(t, app, ticket.Id) {
		t.Error("a public note on a resolved ticket should not await a reply")
	}
}

func TestRequesterReplyClearsAwaitingRequester(t *testing.T) {
	app := testutil.SetupApp(t)
	Register(app)
	customer := seedCustomer(t, app, "Acme")
	requester := seedRequester(t, app, customer)
	agent := seedStaff(t, app)

	ticket := seedTicket(t, app, customer, "in_progress")
	addComment(t, app, ticket, map[string]any{"author_staff": agent.Id, "requests_reply": true})
	if !awaitingOf(t, app, ticket.Id) {
		t.Fatal("precondition: staff comment requesting a reply should set the flag")
	}
	addComment(t, app, ticket, map[string]any{"author_user": requester.Id})
	if awaitingOf(t, app, ticket.Id) {
		t.Error("requester reply should clear awaiting_requester")
	}
	if got := statusOf(t, app, ticket.Id); got != "in_progress" {
		t.Errorf("reply on an active ticket should not change status, got %q", got)
	}
}

func TestResolvingClearsAwaitingRequester(t *testing.T) {
	app := testutil.SetupApp(t)
	Register(app)
	customer := seedCustomer(t, app, "Acme")
	agent := seedStaff(t, app)

	ticket := seedTicket(t, app, customer, "in_progress")
	addComment(t, app, ticket, map[string]any{"author_staff": agent.Id, "requests_reply": true})
	if !awaitingOf(t, app, ticket.Id) {
		t.Fatal("precondition: staff comment requesting a reply should set the flag")
	}
	// Staff resolves the ticket — the pre-save hook should clear the flag.
	rec, err := app.FindRecordById("tickets", ticket.Id)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	rec.Set("status", "resolved")
	if err := app.Save(rec); err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if awaitingOf(t, app, ticket.Id) {
		t.Error("resolving a ticket should clear awaiting_requester")
	}
}

func TestTicketNumberAssignment(t *testing.T) {
	app := testutil.SetupApp(t)
	Register(app)

	customer := seedCustomer(t, app, "Acme")
	col, err := app.FindCollectionByNameOrId("tickets")
	if err != nil {
		t.Fatalf("find tickets: %v", err)
	}

	for want := 1; want <= 3; want++ {
		rec := core.NewRecord(col)
		rec.Set("customer", customer.Id)
		rec.Set("title", "test")
		if err := app.Save(rec); err != nil {
			t.Fatalf("save ticket %d: %v", want, err)
		}
		if got := rec.GetInt("number"); got != want {
			t.Errorf("ticket number: got %d, want %d", got, want)
		}
		if got := rec.GetString("status"); got != "open" {
			t.Errorf("default status: got %q, want open", got)
		}
		if got := rec.GetString("priority"); got != "normal" {
			t.Errorf("default priority: got %q, want normal", got)
		}
	}
}

func TestTicketNumberPreservedWhenSet(t *testing.T) {
	app := testutil.SetupApp(t)
	Register(app)

	customer := seedCustomer(t, app, "Acme")
	col, _ := app.FindCollectionByNameOrId("tickets")

	rec := core.NewRecord(col)
	rec.Set("customer", customer.Id)
	rec.Set("title", "explicit number")
	rec.Set("number", 42)
	rec.Set("status", "waiting")
	if err := app.Save(rec); err != nil {
		t.Fatalf("save: %v", err)
	}
	if got := rec.GetInt("number"); got != 42 {
		t.Errorf("number overwritten: got %d, want 42", got)
	}
	if got := rec.GetString("status"); got != "waiting" {
		t.Errorf("status overwritten: got %q, want waiting", got)
	}
}

func resolvedAtOf(t *testing.T, app core.App, id string) string {
	t.Helper()
	rec, err := app.FindRecordById("tickets", id)
	if err != nil {
		t.Fatalf("reload ticket: %v", err)
	}
	return rec.GetString("resolved_at")
}

func TestResolvedAtStampedAndCleared(t *testing.T) {
	app := testutil.SetupApp(t)
	Register(app)
	customer := seedCustomer(t, app, "Acme")

	// Open → no resolved_at.
	ticket := seedTicket(t, app, customer, "open")
	if got := resolvedAtOf(t, app, ticket.Id); got != "" {
		t.Errorf("open ticket should have no resolved_at, got %q", got)
	}

	// → resolved stamps it.
	ticket.Set("status", "resolved")
	if err := app.Save(ticket); err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if got := resolvedAtOf(t, app, ticket.Id); got == "" {
		t.Error("resolving should stamp resolved_at")
	}

	// → closed clears it (resolved_at only measures the current resolved spell).
	ticket.Set("status", "closed")
	if err := app.Save(ticket); err != nil {
		t.Fatalf("close: %v", err)
	}
	if got := resolvedAtOf(t, app, ticket.Id); got != "" {
		t.Errorf("closing should clear resolved_at, got %q", got)
	}
}

func TestAutoCloseResolved(t *testing.T) {
	app := testutil.SetupApp(t)
	Register(app)
	customer := seedCustomer(t, app, "Acme")

	// Stale: resolved long ago → should auto-close.
	stale := seedTicket(t, app, customer, "resolved")
	stale.Set("resolved_at", "2020-01-01 00:00:00.000Z")
	if err := app.Save(stale); err != nil {
		t.Fatalf("backdate stale: %v", err)
	}
	// Recent: resolved just now (create hook stamped it) → should stay resolved.
	recent := seedTicket(t, app, customer, "resolved")
	// Active and already-closed tickets are never touched.
	open := seedTicket(t, app, customer, "open")

	closed, err := AutoCloseResolved(app, 7)
	if err != nil {
		t.Fatalf("AutoCloseResolved: %v", err)
	}
	if closed != 1 {
		t.Errorf("expected 1 auto-closed, got %d", closed)
	}
	if got := statusOf(t, app, stale.Id); got != "closed" {
		t.Errorf("stale resolved ticket should be closed, got %q", got)
	}
	if got := resolvedAtOf(t, app, stale.Id); got != "" {
		t.Errorf("auto-closed ticket should have resolved_at cleared, got %q", got)
	}
	if got := statusOf(t, app, recent.Id); got != "resolved" {
		t.Errorf("recently resolved ticket should stay resolved, got %q", got)
	}
	if got := statusOf(t, app, open.Id); got != "open" {
		t.Errorf("open ticket must be untouched, got %q", got)
	}
}

func TestAutoCloseDisabledIsNoOp(t *testing.T) {
	app := testutil.SetupApp(t)
	Register(app)
	customer := seedCustomer(t, app, "Acme")

	stale := seedTicket(t, app, customer, "resolved")
	stale.Set("resolved_at", "2020-01-01 00:00:00.000Z")
	if err := app.Save(stale); err != nil {
		t.Fatalf("backdate: %v", err)
	}
	if n, err := AutoCloseResolved(app, 0); err != nil || n != 0 {
		t.Errorf("days=0 must be a no-op: got n=%d err=%v", n, err)
	}
	if got := statusOf(t, app, stale.Id); got != "resolved" {
		t.Errorf("days=0 must leave tickets resolved, got %q", got)
	}
}
