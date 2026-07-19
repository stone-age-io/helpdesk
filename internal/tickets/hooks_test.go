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

	for _, status := range []string{"resolved", "closed"} {
		ticket := seedTicket(t, app, customer, status)
		addComment(t, app, ticket, map[string]any{"author_user": requester.Id})
		if got := statusOf(t, app, ticket.Id); got != "open" {
			t.Errorf("%s ticket: requester comment should reopen to open, got %q", status, got)
		}
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

func TestPublicStaffCommentSetsAwaitingRequester(t *testing.T) {
	app := testutil.SetupApp(t)
	Register(app)
	customer := seedCustomer(t, app, "Acme")
	agent := seedStaff(t, app)

	ticket := seedTicket(t, app, customer, "in_progress")
	if awaitingOf(t, app, ticket.Id) {
		t.Fatal("new ticket should not be awaiting the requester")
	}
	addComment(t, app, ticket, map[string]any{"author_staff": agent.Id})
	if !awaitingOf(t, app, ticket.Id) {
		t.Error("public staff comment should mark the ticket awaiting the requester")
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
	addComment(t, app, ticket, map[string]any{"author_staff": agent.Id})
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
	addComment(t, app, ticket, map[string]any{"author_staff": agent.Id})
	if !awaitingOf(t, app, ticket.Id) {
		t.Fatal("precondition: staff comment should set the flag")
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
	addComment(t, app, ticket, map[string]any{"author_staff": agent.Id})
	if !awaitingOf(t, app, ticket.Id) {
		t.Fatal("precondition: staff comment should set the flag")
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
