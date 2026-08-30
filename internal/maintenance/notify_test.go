package maintenance

import (
	"sync"
	"testing"
	"time"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/mailer"

	"github.com/stone-age-io/helpdesk/internal/notifications"
	"github.com/stone-age-io/helpdesk/internal/testutil"
	"github.com/stone-age-io/helpdesk/internal/tickets"
)

// A generated maintenance ticket deliberately does NOT call
// notifications.Suppress. That is the opposite call from
// tickets.AutoCloseResolved, and this test is here so that adding a Suppress —
// which looks like tidiness — fails loudly instead of silently going quiet.
//
// It also pins the second half of the reasoning: a plan has no requester, so
// ticket.created's requester recipient resolves to nothing and only staff are
// mailed, exactly as for a machine ticket.
func TestGeneratedTicketMailsStaffOnly(t *testing.T) {
	app := testutil.SetupApp(t)
	tickets.Register(app)
	Register(app)

	n := notifications.New(app)
	t.Cleanup(func() { n.WaitInFlight(5 * time.Second) })
	notifications.RegisterHooks(app, n)

	var (
		mu   sync.Mutex
		sent []*mailer.Message
	)
	app.OnMailerSend().BindFunc(func(e *core.MailerEvent) error {
		mu.Lock()
		defer mu.Unlock()
		sent = append(sent, e.Message)
		return nil // swallow: no real delivery
	})

	custCol, _ := app.FindCollectionByNameOrId("customers")
	cust := core.NewRecord(custCol)
	cust.Set("name", "Acme Corp")
	cust.Set("code", "acme")
	cust.Set("active", true)
	if err := app.Save(cust); err != nil {
		t.Fatalf("save customer: %v", err)
	}

	staffCol, _ := app.FindCollectionByNameOrId("staff")
	agent := core.NewRecord(staffCol)
	agent.Set("name", "Sam Staff")
	agent.Set("email", "sam@msp.example")
	agent.Set("role", "agent")
	agent.Set("active", true)
	agent.SetPassword("test-password-123")
	if err := app.Save(agent); err != nil {
		t.Fatalf("save staff: %v", err)
	}

	// A requester exists but is NOT on the plan — a plan has no requester field
	// at all, so nothing should reach them.
	userCol, _ := app.FindCollectionByNameOrId("users")
	req := core.NewRecord(userCol)
	req.Set("name", "Rita Requester")
	req.Set("email", "rita@acme.example")
	req.Set("customer", cust.Id)
	req.Set("active", true)
	req.SetPassword("test-password-123")
	if err := app.Save(req); err != nil {
		t.Fatalf("save requester: %v", err)
	}

	seedPlan(t, app, cust.Id, map[string]any{"next_due": day(0), "interval_days": 90})

	if created, _, err := Generate(app, time.Now().UTC()); err != nil || created != 1 {
		t.Fatalf("generate: created=%d err=%v", created, err)
	}
	if !n.WaitInFlight(5 * time.Second) {
		t.Fatal("notifier goroutines did not finish")
	}

	mu.Lock()
	defer mu.Unlock()

	var to []string
	for _, m := range sent {
		for _, addr := range m.To {
			to = append(to, addr.Address)
		}
	}
	if len(to) == 0 {
		t.Fatal("a generated maintenance ticket sent no mail — was Suppress added? See createTicket")
	}
	for _, addr := range to {
		if addr == "rita@acme.example" {
			t.Error("the requester was mailed about a maintenance ticket they did not file")
		}
	}
}
