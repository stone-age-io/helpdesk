package inbound

import (
	"testing"

	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/core"

	"github.com/stone-age-io/helpdesk/internal/testutil"
	"github.com/stone-age-io/helpdesk/internal/tickets"
)

func setup(t *testing.T) (*pocketbase.PocketBase, *core.Record) {
	t.Helper()
	app := testutil.SetupApp(t)
	tickets.Register(app)

	col, _ := app.FindCollectionByNameOrId("customers")
	customer := core.NewRecord(col)
	customer.Set("name", "Acme Corp")
	customer.Set("active", true)
	customer.Set("webhook_token", "tok-acme")
	if err := app.Save(customer); err != nil {
		t.Fatalf("save customer: %v", err)
	}
	return app, customer
}

func TestCreateTicketBasics(t *testing.T) {
	app, customer := setup(t)
	rec, created, err := CreateTicket(app, customer, Payload{Title: "printer on fire", Body: "3rd floor", Priority: "urgent"})
	if err != nil {
		t.Fatalf("CreateTicket: %v", err)
	}
	if !created {
		t.Fatal("created = false on first ticket")
	}
	if got := rec.GetString("source"); got != "webhook" {
		t.Errorf("source: got %q", got)
	}
	if got := rec.GetString("priority"); got != "urgent" {
		t.Errorf("priority: got %q", got)
	}
	if got := rec.GetInt("number"); got != 1 {
		t.Errorf("number hook: got %d", got)
	}
}

func TestCreateTicketRequiresTitle(t *testing.T) {
	app, customer := setup(t)
	if _, _, err := CreateTicket(app, customer, Payload{Body: "no title"}); err == nil {
		t.Fatal("missing title accepted")
	}
}

func TestCreateTicketDedupes(t *testing.T) {
	app, customer := setup(t)
	p := Payload{Title: "pump fault", DedupeKey: "pump-7"}
	first, created, err := CreateTicket(app, customer, p)
	if err != nil || !created {
		t.Fatalf("first: created=%v err=%v", created, err)
	}
	second, created, err := CreateTicket(app, customer, p)
	if err != nil {
		t.Fatalf("second: %v", err)
	}
	if created {
		t.Error("duplicate dedupe_key created a second ticket")
	}
	if second.Id != first.Id {
		t.Errorf("duplicate returned different ticket: %s vs %s", second.Id, first.Id)
	}
}

func TestRequesterMatchIsCustomerScoped(t *testing.T) {
	app, customer := setup(t)

	// A requester at ANOTHER customer with the same email must not match.
	custCol, _ := app.FindCollectionByNameOrId("customers")
	other := core.NewRecord(custCol)
	other.Set("name", "Globex")
	other.Set("active", true)
	if err := app.Save(other); err != nil {
		t.Fatalf("save other customer: %v", err)
	}
	userCol, _ := app.FindCollectionByNameOrId("users")
	stranger := core.NewRecord(userCol)
	stranger.Set("name", "Stray")
	stranger.Set("email", "shared@example.com")
	stranger.Set("customer", other.Id)
	stranger.Set("active", true)
	stranger.SetPassword("test-password-123")
	if err := app.Save(stranger); err != nil {
		t.Fatalf("save stranger: %v", err)
	}

	rec, _, err := CreateTicket(app, customer, Payload{Title: "x", RequesterEmail: "shared@example.com"})
	if err != nil {
		t.Fatalf("CreateTicket: %v", err)
	}
	if got := rec.GetString("requester"); got != "" {
		t.Errorf("cross-tenant requester linked: %q", got)
	}

	// The same email at THIS customer does match.
	local := core.NewRecord(userCol)
	local.Set("name", "Rita")
	local.Set("email", "rita@acme.example")
	local.Set("customer", customer.Id)
	local.Set("active", true)
	local.SetPassword("test-password-123")
	if err := app.Save(local); err != nil {
		t.Fatalf("save local requester: %v", err)
	}
	rec2, _, err := CreateTicket(app, customer, Payload{Title: "y", RequesterEmail: "rita@acme.example"})
	if err != nil {
		t.Fatalf("CreateTicket: %v", err)
	}
	if got := rec2.GetString("requester"); got != local.Id {
		t.Errorf("same-customer requester not linked: got %q, want %q", got, local.Id)
	}
}
