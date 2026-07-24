package inbound

import (
	"strconv"
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/core"

	"github.com/stone-age-io/helpdesk/internal/testutil"
	"github.com/stone-age-io/helpdesk/internal/tickets"
)

// emailSetup returns an app with ticket lifecycle hooks (so the reply→reopen
// path is exercised for real) and one active customer with a mapped domain.
func emailSetup(t *testing.T) (*pocketbase.PocketBase, *core.Record) {
	t.Helper()
	app := testutil.SetupApp(t)
	tickets.Register(app)

	col, _ := app.FindCollectionByNameOrId("customers")
	customer := core.NewRecord(col)
	customer.Set("name", "Acme Corp")
	customer.Set("active", true)
	customer.Set("email_domain", "acme.example")
	if err := app.Save(customer); err != nil {
		t.Fatalf("save customer: %v", err)
	}
	return app, customer
}

func seedUser(t *testing.T, app *pocketbase.PocketBase, customer *core.Record, name, email string) *core.Record {
	t.Helper()
	col, _ := app.FindCollectionByNameOrId("users")
	u := core.NewRecord(col)
	u.Set("name", name)
	u.Set("email", email)
	u.Set("customer", customer.Id)
	u.Set("active", true)
	u.SetPassword("test-password-123")
	if err := app.Save(u); err != nil {
		t.Fatalf("save user %s: %v", email, err)
	}
	return u
}

func msg(from, subject, body string) NormalizedInbound {
	return NormalizedInbound{
		MessageID: "<" + subject + "@mail.example>",
		From:      Addr{Email: from, Name: "Sender"},
		Subject:   subject,
		Body:      body,
		DKIMPass:  true,
	}
}

func TestIngestNewTicketByUser(t *testing.T) {
	app, customer := emailSetup(t)
	rita := seedUser(t, app, customer, "Rita", "rita@acme.example")

	res, err := IngestEmail(app, msg("rita@acme.example", "printer on fire", "3rd floor"))
	if err != nil {
		t.Fatalf("IngestEmail: %v", err)
	}
	if res.Outcome != OutcomeCreated {
		t.Fatalf("outcome: got %q want created", res.Outcome)
	}
	if got := res.Ticket.GetString("source"); got != "email" {
		t.Errorf("source: got %q want email", got)
	}
	if got := res.Ticket.GetString("customer"); got != customer.Id {
		t.Errorf("customer: got %q want %q", got, customer.Id)
	}
	if got := res.Ticket.GetString("requester"); got != rita.Id {
		t.Errorf("requester: got %q want %q (matched user)", got, rita.Id)
	}
}

func TestIngestNewTicketByDomain(t *testing.T) {
	app, customer := emailSetup(t)

	// A sender at the mapped domain with NO registered user → customer via rung 2,
	// no requester.
	res, err := IngestEmail(app, msg("newguy@acme.example", "vpn down", "since noon"))
	if err != nil {
		t.Fatalf("IngestEmail: %v", err)
	}
	if res.Outcome != OutcomeCreated {
		t.Fatalf("outcome: got %q want created", res.Outcome)
	}
	if got := res.Ticket.GetString("customer"); got != customer.Id {
		t.Errorf("customer: got %q want %q", got, customer.Id)
	}
	if got := res.Ticket.GetString("requester"); got != "" {
		t.Errorf("requester should be empty for an unregistered domain sender, got %q", got)
	}
}

func TestIngestRejectsUnresolvedSender(t *testing.T) {
	app, _ := emailSetup(t)

	// Unknown domain, no user → rejected (no catch-all customer).
	res, err := IngestEmail(app, msg("stranger@unknown.test", "hello", "anyone there"))
	if err != nil {
		t.Fatalf("IngestEmail: %v", err)
	}
	if res.Outcome != OutcomeIgnored {
		t.Fatalf("outcome: got %q want ignored", res.Outcome)
	}

	// A public-provider sender is likewise unresolvable (no customer maps a
	// shared domain) unless registered as a user.
	res2, _ := IngestEmail(app, msg("someone@gmail.com", "hi", "test"))
	if res2.Outcome != OutcomeIgnored {
		t.Errorf("public-domain sender: got %q want ignored", res2.Outcome)
	}
}

func TestIngestReplyThreadsAndReopens(t *testing.T) {
	app, customer := emailSetup(t)
	rita := seedUser(t, app, customer, "Rita", "rita@acme.example")

	// A resolved ticket owned by Rita.
	created, err := IngestEmail(app, msg("rita@acme.example", "laptop slow", "very slow"))
	if err != nil {
		t.Fatalf("seed ticket: %v", err)
	}
	ticket := created.Ticket
	ticket.Set("status", "resolved")
	if err := app.Save(ticket); err != nil {
		t.Fatalf("resolve ticket: %v", err)
	}
	num := ticket.GetInt("number")

	// Rita replies (subject carries the [#N] token from the notification).
	reply := msg("rita@acme.example", "Re: [#"+strconv.Itoa(num)+"] laptop slow", "still slow!")
	res, err := IngestEmail(app, reply)
	if err != nil {
		t.Fatalf("IngestEmail reply: %v", err)
	}
	if res.Outcome != OutcomeCommented {
		t.Fatalf("outcome: got %q want commented", res.Outcome)
	}

	// The comment exists, is public, and attributed to Rita.
	comments, _ := app.FindRecordsByFilter("ticket_comments",
		"ticket = {:t}", "-created", 1, 0, map[string]any{"t": ticket.Id})
	if len(comments) == 0 {
		t.Fatal("no comment written")
	}
	if comments[0].GetString("author_user") != rita.Id {
		t.Errorf("comment author: got %q want %q", comments[0].GetString("author_user"), rita.Id)
	}
	if comments[0].GetBool("internal") {
		t.Error("email reply should be a public comment, not internal")
	}

	// The resolved ticket reopened (existing tickets hook).
	fresh, _ := app.FindRecordById("tickets", ticket.Id)
	if got := fresh.GetString("status"); got != "open" {
		t.Errorf("resolved ticket should reopen on requester reply, got %q", got)
	}
}

func TestIngestReplyToClosedMakesNewTicket(t *testing.T) {
	app, customer := emailSetup(t)
	seedUser(t, app, customer, "Rita", "rita@acme.example")

	created, _ := IngestEmail(app, msg("rita@acme.example", "door sensor", "offline"))
	ticket := created.Ticket
	ticket.Set("status", "closed")
	if err := app.Save(ticket); err != nil {
		t.Fatalf("close ticket: %v", err)
	}
	num := ticket.GetInt("number")

	reply := msg("rita@acme.example", "Re: [#"+strconv.Itoa(num)+"] door sensor", "it is broken again")
	res, err := IngestEmail(app, reply)
	if err != nil {
		t.Fatalf("IngestEmail: %v", err)
	}
	if res.Outcome != OutcomeCreated {
		t.Fatalf("reply to CLOSED ticket should create a new ticket, got %q", res.Outcome)
	}
	if res.Ticket.Id == ticket.Id {
		t.Error("new ticket should be distinct from the closed one")
	}
	if body := res.Ticket.GetString("body"); !strings.Contains(body, "Reply to closed ticket #"+strconv.Itoa(num)) {
		t.Errorf("new ticket body missing breadcrumb, got %q", body)
	}
}

func TestIngestLoopGuard(t *testing.T) {
	app, customer := emailSetup(t)
	seedUser(t, app, customer, "Rita", "rita@acme.example")

	cases := []struct {
		name string
		mut  func(*NormalizedInbound)
	}{
		{"auto-submitted", func(m *NormalizedInbound) { m.Headers = map[string]string{"auto-submitted": "auto-replied"} }},
		{"bulk precedence", func(m *NormalizedInbound) { m.Headers = map[string]string{"precedence": "bulk"} }},
		{"mailer-daemon", func(m *NormalizedInbound) { m.From = Addr{Email: "mailer-daemon@acme.example"} }},
		{"spam flag", func(m *NormalizedInbound) { m.SpamFlag = true }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := msg("rita@acme.example", "out of office", "away until monday")
			tc.mut(&m)
			res, err := IngestEmail(app, m)
			if err != nil {
				t.Fatalf("IngestEmail: %v", err)
			}
			if res.Outcome != OutcomeIgnored {
				t.Errorf("outcome: got %q want ignored", res.Outcome)
			}
		})
	}
}

func TestIngestReplyIdempotent(t *testing.T) {
	app, customer := emailSetup(t)
	seedUser(t, app, customer, "Rita", "rita@acme.example")

	created, _ := IngestEmail(app, msg("rita@acme.example", "printer jam", "again"))
	num := created.Ticket.GetInt("number")

	reply := msg("rita@acme.example", "Re: [#"+strconv.Itoa(num)+"] printer jam", "still jammed")
	reply.MessageID = "<dup-reply@mail.example>"

	if res, _ := IngestEmail(app, reply); res.Outcome != OutcomeCommented {
		t.Fatalf("first delivery: got %q want commented", res.Outcome)
	}
	// Redelivery of the same Message-ID must not create a second comment.
	res, err := IngestEmail(app, reply)
	if err != nil {
		t.Fatalf("redelivery: %v", err)
	}
	if res.Outcome != OutcomeDuplicate {
		t.Errorf("redelivery outcome: got %q want duplicate", res.Outcome)
	}
	comments, _ := app.FindRecordsByFilter("ticket_comments",
		"ticket = {:t}", "-created", 0, 0, map[string]any{"t": created.Ticket.Id})
	if len(comments) != 1 {
		t.Errorf("expected exactly 1 comment after redelivery, got %d", len(comments))
	}
}

func TestParseTicketToken(t *testing.T) {
	cases := map[string]string{
		"Re: [#42] printer on fire":  "42",
		"[#7] new":                   "7",
		"no token here":              "",
		"Fwd: RE: [#1234] something": "1234",
		"bracket [#] empty":          "",
	}
	for subject, want := range cases {
		if got := ParseTicketToken(subject); got != want {
			t.Errorf("ParseTicketToken(%q): got %q want %q", subject, got, want)
		}
	}
}
