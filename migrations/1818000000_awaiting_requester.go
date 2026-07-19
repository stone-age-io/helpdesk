package migrations

import (
	"fmt"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

// Adds tickets.awaiting_requester — a derived boolean the portal uses to tell a
// requester "the ball is in your court." It is maintained server-side by
// internal/tickets:
//
//   - true  when staff posts a PUBLIC comment on a non-resolved/closed ticket
//     (they asked something and are waiting on a reply),
//   - false when the requester replies (ball back to staff), or when the ticket
//     is resolved/closed (nothing is pending).
//
// It's a cache of "last public comment is staff's && ticket is open," kept as a
// column so the portal can filter and count it cheaply (list chip, dashboard
// tile) instead of scanning comments per ticket. Requesters already read their
// own tickets, so no rule change is needed.
//
// The up migration seeds the same truth for existing tickets by inspecting each
// ticket's most recent public comment. Idempotent: skips if the field exists,
// and the backfill only writes rows whose value actually differs.

func init() {
	m.Register(awaitingRequesterUp, awaitingRequesterDown)
}

func awaitingRequesterUp(app core.App) error {
	if err := addField(app, "tickets", &core.BoolField{Name: "awaiting_requester"}); err != nil {
		return err
	}
	return backfillAwaitingRequester(app)
}

func awaitingRequesterDown(app core.App) error {
	return removeField(app, "tickets", "awaiting_requester")
}

// backfillAwaitingRequester computes the flag for every existing ticket: a
// ticket awaits the requester when it isn't resolved/closed and its most recent
// public comment was authored by staff.
func backfillAwaitingRequester(app core.App) error {
	tickets, err := app.FindRecordsByFilter("tickets", "", "", 0, 0)
	if err != nil {
		return fmt.Errorf("list tickets: %w", err)
	}
	for _, t := range tickets {
		want := false
		if st := t.GetString("status"); st != "resolved" && st != "closed" {
			rows, err := app.FindRecordsByFilter(
				"ticket_comments",
				"ticket = {:t} && internal = false",
				"-created", 1, 0,
				dbx.Params{"t": t.Id},
			)
			if err == nil && len(rows) > 0 && rows[0].GetString("author_staff") != "" {
				want = true
			}
		}
		if t.GetBool("awaiting_requester") != want {
			t.Set("awaiting_requester", want)
			if err := app.Save(t); err != nil {
				return fmt.Errorf("backfill ticket %s: %w", t.Id, err)
			}
		}
	}
	return nil
}
