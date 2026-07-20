package migrations

import (
	"fmt"

	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"

	"github.com/stone-age-io/helpdesk/internal/authz"
)

// A `closed` ticket is final: requesters can't reply to it (a follow-up is a new
// ticket), which is the second half of the two-stage lifecycle alongside
// `resolved_at` (migration `1821000000`). `resolved` stays reopen-by-reply; only
// `closed` is sealed. Enforced by adding `@request.body.ticket.status != 'closed'`
// to the requester branch of the ticket_comments create rule — the same
// `@request.body.ticket.<field>` relation hop the rule already uses for
// `.customer`. Staff can still comment on closed tickets (their branch is
// unguarded), so an internal "customer called, opened #123" note is still
// possible. The portal hides the reply box on closed tickets; this rule is the
// server-side backstop.
//
// This supersedes the ticket_comments create rule from migration `1800000000`
// (never amended since). Kept as a full literal here rather than a shared helper
// because it's the only amendment.

func closeCommentGuardCreateRule() string {
	return "(" + authz.StaffRule + " && @request.body.author_staff = @request.auth.id && @request.body.author_user:isset = false)" +
		" || (" + authz.RequesterRule +
		" && @request.body.ticket.customer = @request.auth.customer" +
		" && @request.body.ticket.status != 'closed'" +
		" && @request.body.author_user = @request.auth.id" +
		" && @request.body.author_staff:isset = false" +
		" && @request.body.internal:isset = false)"
}

// original1800CommentsCreateRule is the pre-guard rule, restored on down.
func original1800CommentsCreateRule() string {
	return "(" + authz.StaffRule + " && @request.body.author_staff = @request.auth.id && @request.body.author_user:isset = false)" +
		" || (" + authz.RequesterRule +
		" && @request.body.ticket.customer = @request.auth.customer" +
		" && @request.body.author_user = @request.auth.id" +
		" && @request.body.author_staff:isset = false" +
		" && @request.body.internal:isset = false)"
}

func init() {
	m.Register(closeCommentGuardUp, closeCommentGuardDown)
}

func closeCommentGuardUp(app core.App) error {
	comments, err := app.FindCollectionByNameOrId("ticket_comments")
	if err != nil {
		return fmt.Errorf("find ticket_comments: %w", err)
	}
	rule := closeCommentGuardCreateRule()
	comments.CreateRule = &rule
	if err := app.Save(comments); err != nil {
		return fmt.Errorf("save ticket_comments: %w", err)
	}
	return nil
}

// closeCommentGuardDown is dev-loop only, like the other down paths.
func closeCommentGuardDown(app core.App) error {
	comments, err := app.FindCollectionByNameOrId("ticket_comments")
	if err != nil {
		return fmt.Errorf("find ticket_comments: %w", err)
	}
	rule := original1800CommentsCreateRule()
	comments.CreateRule = &rule
	if err := app.Save(comments); err != nil {
		return fmt.Errorf("save ticket_comments: %w", err)
	}
	return nil
}
