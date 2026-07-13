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

func TestCreateTicketClassification(t *testing.T) {
	app, customer := setup(t)

	catCol, _ := app.FindCollectionByNameOrId("ticket_categories")
	cat := core.NewRecord(catCol)
	cat.Set("name", "VoIP")
	cat.Set("key", "voip")
	cat.Set("active", true)
	if err := app.Save(cat); err != nil {
		t.Fatalf("seed category: %v", err)
	}

	// Known key + free-text asset/location all land on the ticket.
	rec, _, err := CreateTicket(app, customer, Payload{
		Title: "printer down", Category: "voip", Asset: "printer-3f", Location: "copy room",
	})
	if err != nil {
		t.Fatalf("CreateTicket: %v", err)
	}
	if got := rec.GetString("category"); got != cat.Id {
		t.Errorf("category: got %q, want %q", got, cat.Id)
	}
	if got := rec.GetString("asset"); got != "printer-3f" {
		t.Errorf("asset: got %q", got)
	}
	if got := rec.GetString("location_note"); got != "copy room" {
		t.Errorf("location_note: got %q", got)
	}

	// Unknown key → created, unclassified.
	rec2, _, err := CreateTicket(app, customer, Payload{Title: "y", Category: "nope"})
	if err != nil {
		t.Fatalf("CreateTicket unknown category: %v", err)
	}
	if got := rec2.GetString("category"); got != "" {
		t.Errorf("unknown category should not classify, got %q", got)
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

func TestCreateTicketResolvesLocationByCode(t *testing.T) {
	app, customer := setup(t)

	locCol, _ := app.FindCollectionByNameOrId("locations")
	loc := core.NewRecord(locCol)
	loc.Set("customer", customer.Id)
	loc.Set("code", "HQ")
	loc.Set("name", "Acme HQ")
	if err := app.Save(loc); err != nil {
		t.Fatalf("seed location: %v", err)
	}

	// A location with the SAME code at another customer must not match — the
	// resolver is customer-scoped.
	custCol, _ := app.FindCollectionByNameOrId("customers")
	other := core.NewRecord(custCol)
	other.Set("name", "Globex")
	other.Set("active", true)
	if err := app.Save(other); err != nil {
		t.Fatalf("save other customer: %v", err)
	}
	otherLoc := core.NewRecord(locCol)
	otherLoc.Set("customer", other.Id)
	otherLoc.Set("code", "HQ")
	otherLoc.Set("name", "Globex HQ")
	if err := app.Save(otherLoc); err != nil {
		t.Fatalf("seed other location: %v", err)
	}

	rec, _, err := CreateTicket(app, customer, Payload{Title: "x", LocationCode: "HQ"})
	if err != nil {
		t.Fatalf("CreateTicket: %v", err)
	}
	if got := rec.GetString("location"); got != loc.Id {
		t.Errorf("location: got %q, want %q (this customer's HQ, not Globex's)", got, loc.Id)
	}

	// Unknown code → breadcrumb in location_note, no relation.
	rec2, _, err := CreateTicket(app, customer, Payload{Title: "y", LocationCode: "ZZZ"})
	if err != nil {
		t.Fatalf("CreateTicket: %v", err)
	}
	if got := rec2.GetString("location"); got != "" {
		t.Errorf("unknown code linked a location: %q", got)
	}
	if got := rec2.GetString("location_note"); got != "ZZZ" {
		t.Errorf("breadcrumb: got %q, want ZZZ", got)
	}
}
