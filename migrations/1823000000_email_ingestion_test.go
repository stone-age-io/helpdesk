package migrations_test

import (
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase/core"

	"github.com/stone-age-io/helpdesk/internal/testutil"
)

// The 1823000000 migration wires the email-ingestion schema: the "email"
// source value, customers.email_domain (unique when set), and the hidden
// ticket_comments.source_message_id idempotency key.
func TestEmailIngestionSchema(t *testing.T) {
	app := testutil.SetupApp(t)

	tickets, err := app.FindCollectionByNameOrId("tickets")
	if err != nil {
		t.Fatalf("find tickets: %v", err)
	}
	source, ok := tickets.Fields.GetByName("source").(*core.SelectField)
	if !ok {
		t.Fatalf("tickets.source should be a select field")
	}
	if !containsStr(source.Values, "email") {
		t.Errorf("tickets.source should include \"email\", got %v", source.Values)
	}

	customers, err := app.FindCollectionByNameOrId("customers")
	if err != nil {
		t.Fatalf("find customers: %v", err)
	}
	dom, ok := customers.Fields.GetByName("email_domain").(*core.TextField)
	if !ok {
		t.Fatalf("customers.email_domain should be a text field")
	}
	if dom.Required {
		t.Error("customers.email_domain should be optional (shared-provider contacts have none)")
	}
	if !hasIndex(customers, "idx_customers_email_domain") {
		t.Error("customers.email_domain should have a unique index")
	}

	comments, err := app.FindCollectionByNameOrId("ticket_comments")
	if err != nil {
		t.Fatalf("find ticket_comments: %v", err)
	}
	smid, ok := comments.Fields.GetByName("source_message_id").(*core.TextField)
	if !ok {
		t.Fatalf("ticket_comments.source_message_id should be a text field")
	}
	if !smid.Hidden {
		t.Error("ticket_comments.source_message_id should be hidden (never on the record API)")
	}
	if !hasIndex(comments, "idx_ticket_comments_source_msgid") {
		t.Error("ticket_comments.source_message_id should have a unique index")
	}
}

// hasIndex reports whether the collection carries an index of the given name.
// Collection.Indexes holds raw "CREATE ... INDEX name ..." statements, so a
// name substring match is enough for the test.
func hasIndex(col *core.Collection, name string) bool {
	for _, idx := range col.Indexes {
		if strings.Contains(idx, name) {
			return true
		}
	}
	return false
}

func containsStr(vals []string, want string) bool {
	for _, v := range vals {
		if v == want {
			return true
		}
	}
	return false
}
