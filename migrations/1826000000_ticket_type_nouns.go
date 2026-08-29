package migrations

import (
	"fmt"

	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

// Rename the ticket type values: issue → reactive, install → planned.
//
// WHY THE OLD NOUNS WERE WRONG
//
// They weren't parallel. "Issue" names a condition (something is broken);
// "install" names an activity (putting something in). Two different parts of
// speech can't sit on one axis, which is why the pair never read right.
//
// "Issue" is also near-tautological in a ticket app — GitHub and Jira have
// trained everyone that an issue IS a ticket, so `type = issue` said nothing.
//
// And "install" was far too narrow for what the value already gated. What it
// actually means is: planned work that hangs off a project, gets visits and an
// effort estimate, and is NOT a reply-driven conversation with the customer. A
// scheduled replacement, a decommission, a site survey, a PM round, a remote
// firmware campaign — all behave identically and none is an installation.
//
// The axis was always reactive vs. planned. The app's own prose said so first:
// CLAUDE.md opens with "reactive support tickets and proactive project /
// installation / field work". The vocabulary just never reached the enum.
//
// WHY THIS STAYS A FIXED SELECT AND NOT A MANAGED COLLECTION
//
// `ticket_categories` (1806000000) is a collection precisely because it is
// inert — a label humans read, safe for an admin to add to without a deploy.
// `type` is the opposite: code branches on it. tickets.markAwaitingRequester
// suppresses the "needs your reply" nag for planned work, and every report axis
// counts planned tickets as their own measure. An admin-invented type would
// leave both with no idea how to treat it, and teaching them would mean putting
// behavior flags on the type row — a configurable behavior engine for what is
// really one boolean.
//
// The rule this settles, and which status/priority already follow: enums for
// what the code branches on, collections for what only humans read. More
// granular KINDS of planned work (install vs. survey vs. replacement) are a
// labelling need and belong on category or a future axis, never here.
//
// WIRE CONTRACT: `ticket.type` rides the outbound notification envelope
// (docs/protocol.md, schema helpdesk.event v1), so the strings MSP-internal
// consumers see change with this migration. Acceptable pre-release; noted
// rather than version-bumped because no consumer exists yet.

func init() {
	m.Register(ticketTypeNounsUp, ticketTypeNounsDown)
}

// ticketTypeRename does the whole job in one direction, so up and down are the
// same code with the mapping flipped and can't drift apart.
func ticketTypeRename(app core.App, rewrite map[string]string, want []string) error {
	tickets, err := app.FindCollectionByNameOrId("tickets")
	if err != nil {
		return fmt.Errorf("find tickets: %w", err)
	}
	field, ok := tickets.Fields.GetByName("type").(*core.SelectField)
	if !ok {
		return fmt.Errorf("tickets.type is not a select field")
	}
	// Idempotent: already carrying the target values means this ran before.
	if hasValue(field.Values, want[0]) {
		return nil
	}
	field.Values = want
	if err := app.Save(tickets); err != nil {
		return fmt.Errorf("save tickets: %w", err)
	}

	// Rewrite the stored values. Raw SQL rather than record saves: this is a
	// pure column rewrite over every ticket, and going through app.Save would
	// fire the ticket hooks — re-stamping resolved_at and, worse, firing a
	// notification for every ticket in the table.
	for old, next := range rewrite {
		if _, err := app.DB().
			NewQuery("UPDATE tickets SET type = {:next} WHERE type = {:old}").
			Bind(map[string]any{"next": next, "old": old}).
			Execute(); err != nil {
			return fmt.Errorf("rewrite tickets.type %s→%s: %w", old, next, err)
		}
	}
	return nil
}

func ticketTypeNounsUp(app core.App) error {
	return ticketTypeRename(app,
		map[string]string{"issue": "reactive", "install": "planned"},
		[]string{"reactive", "planned"},
	)
}

func ticketTypeNounsDown(app core.App) error {
	return ticketTypeRename(app,
		map[string]string{"reactive": "issue", "planned": "install"},
		[]string{"issue", "install"},
	)
}
