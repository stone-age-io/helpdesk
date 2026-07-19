package migrations

import (
	"fmt"

	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

// Refines how tickets.awaiting_requester (1818000000) is set. The original
// trigger — "any public staff comment marks the ticket as awaiting a reply" —
// was too eager: staff often just post an update that needs no response. This
// replaces the inference with an explicit per-comment intent:
//
//   - ticket_comments.requests_reply (bool): staff tick "request a reply" on a
//     public comment when they actually need customer input. Only such a comment
//     flips awaiting_requester (see internal/tickets). Also, install tickets are
//     excluded entirely — they're proactive field work, not a reply-driven
//     conversation.
//
// Because the trigger changed from inferred to explicit and there is no
// historical opt-in, 1818000000's heuristic backfill no longer reflects
// reality — every ticket is reset to "not awaiting," and staff set it going
// forward. Idempotent: skips if the field exists; the reset is naturally
// re-runnable.

func init() {
	m.Register(requestsReplyUp, requestsReplyDown)
}

func requestsReplyUp(app core.App) error {
	if err := addField(app, "ticket_comments", &core.BoolField{Name: "requests_reply"}); err != nil {
		return err
	}
	if _, err := app.DB().NewQuery("UPDATE tickets SET awaiting_requester = false").Execute(); err != nil {
		return fmt.Errorf("reset awaiting_requester: %w", err)
	}
	return nil
}

func requestsReplyDown(app core.App) error {
	return removeField(app, "ticket_comments", "requests_reply")
}
