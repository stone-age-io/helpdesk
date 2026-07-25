package customers

import (
	"testing"

	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/core"

	"github.com/stone-age-io/helpdesk/internal/testutil"
)

func setup(t *testing.T) *pocketbase.PocketBase {
	t.Helper()
	app := testutil.SetupApp(t)
	Register(app)
	return app
}

func newCustomer(app *pocketbase.PocketBase, name, domain string) *core.Record {
	col, _ := app.FindCollectionByNameOrId("customers")
	c := core.NewRecord(col)
	c.Set("name", name)
	c.Set("active", true)
	c.Set("email_domain", domain)
	return c
}

func TestEmailDomainNormalized(t *testing.T) {
	app := setup(t)
	c := newCustomer(app, "Acme", "  Acme.Example  ")
	if err := app.Save(c); err != nil {
		t.Fatalf("save: %v", err)
	}
	if got := c.GetString("email_domain"); got != "acme.example" {
		t.Errorf("email_domain not normalized: got %q want acme.example", got)
	}
}

func TestPublicDomainRejected(t *testing.T) {
	app := setup(t)
	for _, d := range []string{"gmail.com", "Outlook.com", " yahoo.com "} {
		c := newCustomer(app, "Solo "+d, d)
		if err := app.Save(c); err == nil {
			t.Errorf("public domain %q was accepted", d)
		}
	}
}

func TestBlankDomainAllowed(t *testing.T) {
	app := setup(t)
	c := newCustomer(app, "Solo Gmail Contact", "")
	if err := app.Save(c); err != nil {
		t.Errorf("blank email_domain should be allowed: %v", err)
	}
}

func TestDomainUniquePerTenant(t *testing.T) {
	app := setup(t)
	if err := app.Save(newCustomer(app, "Acme", "acme.example")); err != nil {
		t.Fatalf("first customer: %v", err)
	}
	if err := app.Save(newCustomer(app, "Acme Clone", "acme.example")); err == nil {
		t.Error("two customers claimed the same email_domain (unique index missing?)")
	}
}
