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

// TestIngestReplyFromDomainIsPublic: a colleague at the customer's own domain,
// with no portal account, still adds to the conversation — publicly, but
// unattributed, so it doesn't reopen a resolved ticket on their say-so.
func TestIngestReplyFromDomainIsPublic(t *testing.T) {
	app, customer := emailSetup(t)
	seedUser(t, app, customer, "Rita", "rita@acme.example")

	created, _ := IngestEmail(app, msg("rita@acme.example", "badge reader", "dead"))
	num := created.Ticket.GetInt("number")

	reply := msg("colleague@acme.example", "Re: [#"+strconv.Itoa(num)+"] badge reader", "mine too")
	res, err := IngestEmail(app, reply)
	if err != nil || res.Outcome != OutcomeCommented {
		t.Fatalf("outcome: got %q err %v, want commented", res.Outcome, err)
	}
	c, err := app.FindFirstRecordByFilter("ticket_comments", "ticket = {:t}",
		map[string]any{"t": created.Ticket.Id})
	if err != nil {
		t.Fatalf("no comment: %v", err)
	}
	if c.GetBool("internal") {
		t.Error("a sender at the customer's domain should comment publicly")
	}
	if c.GetString("author_user") != "" {
		t.Errorf("unregistered sender should be unattributed, got %q", c.GetString("author_user"))
	}
}

// TestIngestReplyFromOutsiderIsHeldInternal: ticket numbers are sequential, so
// [#N] is guessable. A sender who isn't on the ticket's customer — another
// tenant's user, or a stranger — gets an internal comment staff can review,
// never a public one the customer's requesters would read.
func TestIngestReplyFromOutsiderIsHeldInternal(t *testing.T) {
	app, acme := emailSetup(t)
	seedUser(t, app, acme, "Rita", "rita@acme.example")

	col, _ := app.FindCollectionByNameOrId("customers")
	globex := core.NewRecord(col)
	globex.Set("name", "Globex")
	globex.Set("active", true)
	globex.Set("email_domain", "globex.example")
	if err := app.Save(globex); err != nil {
		t.Fatalf("save globex: %v", err)
	}
	seedUser(t, app, globex, "Gus", "gus@globex.example")

	created, _ := IngestEmail(app, msg("rita@acme.example", "door sensor", "offline"))
	ticket := created.Ticket
	ticket.Set("status", "resolved")
	if err := app.Save(ticket); err != nil {
		t.Fatalf("resolve: %v", err)
	}
	num := strconv.Itoa(ticket.GetInt("number"))

	for _, from := range []string{"gus@globex.example", "stranger@unknown.test"} {
		res, err := IngestEmail(app, msg(from, "Re: [#"+num+"] door sensor from "+from, "click here"))
		if err != nil || res.Outcome != OutcomeCommented {
			t.Fatalf("%s: outcome %q err %v, want commented", from, res.Outcome, err)
		}
	}

	comments, _ := app.FindRecordsByFilter("ticket_comments", "ticket = {:t}", "", 0, 0,
		map[string]any{"t": ticket.Id})
	if len(comments) != 2 {
		t.Fatalf("want 2 held comments, got %d", len(comments))
	}
	for _, c := range comments {
		if !c.GetBool("internal") {
			t.Errorf("outsider reply was published to the portal: %q", c.GetString("body"))
		}
		if c.GetString("author_user") != "" {
			t.Errorf("outsider reply was attributed to %q", c.GetString("author_user"))
		}
	}
	if fresh, _ := app.FindRecordById("tickets", ticket.Id); fresh.GetString("status") != "resolved" {
		t.Errorf("outsider reply changed status to %q", fresh.GetString("status"))
	}
}

// TestIngestReplyToOtherTenantsClosedTicket: replying to a closed ticket that
// belongs to someone else opens a ticket for the sender's own customer, and
// does not name the other tenant's ticket in it.
func TestIngestReplyToOtherTenantsClosedTicket(t *testing.T) {
	app, acme := emailSetup(t)
	seedUser(t, app, acme, "Rita", "rita@acme.example")

	col, _ := app.FindCollectionByNameOrId("customers")
	globex := core.NewRecord(col)
	globex.Set("name", "Globex")
	globex.Set("active", true)
	if err := app.Save(globex); err != nil {
		t.Fatalf("save globex: %v", err)
	}
	seedUser(t, app, globex, "Gus", "gus@globex.example")

	created, _ := IngestEmail(app, msg("rita@acme.example", "door sensor", "offline"))
	ticket := created.Ticket
	ticket.Set("status", "closed")
	if err := app.Save(ticket); err != nil {
		t.Fatalf("close: %v", err)
	}
	num := strconv.Itoa(ticket.GetInt("number"))

	res, err := IngestEmail(app, msg("gus@globex.example", "Re: [#"+num+"] door sensor", "hello"))
	if err != nil || res.Outcome != OutcomeCreated {
		t.Fatalf("outcome %q err %v, want created", res.Outcome, err)
	}
	if got := res.Ticket.GetString("customer"); got != globex.Id {
		t.Errorf("new ticket customer: got %q want globex", got)
	}
	if body := res.Ticket.GetString("body"); strings.Contains(body, "#"+num) {
		t.Errorf("new ticket names another tenant's ticket: %q", body)
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
