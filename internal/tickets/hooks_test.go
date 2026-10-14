package tickets

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
	rec.Set("email", "sam@msp.example")
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
	agent.Set("email", "sam@msp.example")
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
	ticket.Set("type", "planned")
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

// The create hook's default and the tickets.type enum have to agree: the hook
// stamps a value on every ticket that arrives without one (machine intake,
// seeds, the portal form), so a default outside the enum would make every such
// insert invalid. Pinned here rather than in the migrations package because
// SetupApp does not bind record hooks — without Register the field just stays
// empty and the assertion would be vacuous.
func TestTypeDefaultsToReactive(t *testing.T) {
	app := testutil.SetupApp(t)
	Register(app)
	customer := seedCustomer(t, app, "Acme")

	col, err := app.FindCollectionByNameOrId("tickets")
	if err != nil {
		t.Fatalf("find tickets: %v", err)
	}
	rec := core.NewRecord(col)
	rec.Set("customer", customer.Id)
	rec.Set("title", "no type given")
	if err := app.Save(rec); err != nil {
		t.Fatalf("save ticket: %v", err)
	}
	if got := rec.GetString("type"); got != "reactive" {
		t.Errorf("default type: got %q, want reactive", got)
	}
}

// TestResolveReleasesMachineDedupeKey: a NATS or webhook key means "this
// problem", so resolving or closing gives it back and the next occurrence opens
// a new ticket. Email (Message-ID) and maintenance (pm:{plan}:{date}) keys are
// idempotency tokens that must keep deduping after closure, so they stay.
func TestResolveReleasesMachineDedupeKey(t *testing.T) {
	cases := []struct {
		source string
		keep   bool
	}{
		{"nats", false},
		{"webhook", false},
		{"email", true},
		{"maintenance", true},
	}
	for _, tc := range cases {
		for _, status := range []string{"resolved", "closed"} {
			t.Run(tc.source+"/"+status, func(t *testing.T) {
				app := testutil.SetupApp(t)
				Register(app)
				customer := seedCustomer(t, app, "Acme")

				ticket := seedTicket(t, app, customer, "open")
				ticket.Set("source", tc.source)
				ticket.Set("dedupe_key", "key-1")
				if err := app.Save(ticket); err != nil {
					t.Fatalf("set source: %v", err)
				}
				// An unrelated edit while open must not release it.
				ticket.Set("priority", "high")
				if err := app.Save(ticket); err != nil {
					t.Fatalf("edit: %v", err)
				}
				ticket.Set("status", status)
				if err := app.Save(ticket); err != nil {
					t.Fatalf("set %s: %v", status, err)
				}

				rec, err := app.FindRecordById("tickets", ticket.Id)
				if err != nil {
					t.Fatalf("reload: %v", err)
				}
				want := ""
				if tc.keep {
					want = "key-1"
				}
				if got := rec.GetString("dedupe_key"); got != want {
					t.Errorf("dedupe_key after %s: got %q, want %q", status, got, want)
				}
			})
		}
	}
}

// TestCountRepeat: one more, stamped now, on the same ticket.
func TestCountRepeat(t *testing.T) {
	app := testutil.SetupApp(t)
	Register(app)
	customer := seedCustomer(t, app, "Acme")
	ticket := seedTicket(t, app, customer, "open")

	for want := 1; want <= 2; want++ {
		if err := CountRepeat(app, ticket); err != nil {
			t.Fatalf("CountRepeat: %v", err)
		}
		rec, err := app.FindRecordById("tickets", ticket.Id)
		if err != nil {
			t.Fatalf("reload: %v", err)
		}
		if got := rec.GetInt("repeat_count"); got != want {
			t.Errorf("repeat_count: got %d, want %d", got, want)
		}
		if rec.GetDateTime("last_repeat_at").IsZero() {
			t.Error("last_repeat_at not stamped")
		}
	}
}

// TestRepeatFieldsAreServerOnly drives the record API: a staff PATCH can't set
// the count and a requester's create can't seed one. Only CountRepeat writes it.
func TestRepeatFieldsAreServerOnly(t *testing.T) {
	app := testutil.SetupApp(t)
	Register(app)
	customer := seedCustomer(t, app, "Acme")
	requester := seedRequester(t, app, customer)
	agent := seedStaff(t, app)
	mux := testutil.Handler(t, app)

	send := func(method, path, token string, body map[string]any) map[string]any {
		t.Helper()
		raw, _ := json.Marshal(body)
		req := httptest.NewRequest(method, path, bytes.NewReader(raw))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", token)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s %s: got %d: %s", method, path, rec.Code, rec.Body.String())
		}
		var out map[string]any
		_ = json.Unmarshal(rec.Body.Bytes(), &out)
		return out
	}

	created := send("POST", "/api/collections/tickets/records", testutil.AuthToken(t, requester), map[string]any{
		"customer": customer.Id, "requester": requester.Id, "title": "door stuck", "source": "portal",
		"repeat_count": 50, "last_repeat_at": "2026-01-01 00:00:00.000Z",
	})
	id, _ := created["id"].(string)

	ticket, err := app.FindRecordById("tickets", id)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if ticket.GetInt("repeat_count") != 0 || !ticket.GetDateTime("last_repeat_at").IsZero() {
		t.Errorf("requester create set repeat fields: count=%d at=%q",
			ticket.GetInt("repeat_count"), ticket.GetString("last_repeat_at"))
	}

	if err := CountRepeat(app, ticket); err != nil {
		t.Fatalf("CountRepeat: %v", err)
	}
	stamped := ticket.GetString("last_repeat_at")

	send("PATCH", "/api/collections/tickets/records/"+id, testutil.AuthToken(t, agent), map[string]any{
		"priority": "high", "repeat_count": 0, "last_repeat_at": "",
	})
	ticket, err = app.FindRecordById("tickets", id)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if ticket.GetString("priority") != "high" {
		t.Error("the rest of the staff PATCH should still apply")
	}
	if ticket.GetInt("repeat_count") != 1 || ticket.GetString("last_repeat_at") != stamped {
		t.Errorf("staff PATCH rolled back repeat fields: count=%d at=%q",
			ticket.GetInt("repeat_count"), ticket.GetString("last_repeat_at"))
	}
}
