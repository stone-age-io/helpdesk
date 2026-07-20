package migrations_test

import (
	"strings"
	"testing"

	"github.com/stone-age-io/helpdesk/internal/testutil"
)

// The 1822000000 migration seals closed tickets: the ticket_comments create
// rule blocks the requester branch when the ticket is closed, while leaving the
// staff branch untouched.
func TestCloseCommentGuardRule(t *testing.T) {
	app := testutil.SetupApp(t)

	comments, err := app.FindCollectionByNameOrId("ticket_comments")
	if err != nil {
		t.Fatalf("find ticket_comments: %v", err)
	}
	r := comments.CreateRule
	if r == nil {
		t.Fatal("ticket_comments create rule is nil")
	}
	if !strings.Contains(*r, "@request.body.ticket.status != 'closed'") {
		t.Errorf("create rule missing the closed guard: %v", *r)
	}
	// Staff branch survives — staff can still comment on closed tickets.
	if !strings.Contains(*r, "@request.body.author_staff = @request.auth.id") {
		t.Errorf("create rule lost its staff branch: %v", *r)
	}
	// Requester branch still scopes to the caller's own company.
	if !strings.Contains(*r, "@request.body.ticket.customer = @request.auth.customer") {
		t.Errorf("create rule lost the requester customer scope: %v", *r)
	}
}
