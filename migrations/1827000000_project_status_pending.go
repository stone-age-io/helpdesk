package migrations

import (
	"fmt"

	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

// Rename the projects lifecycle's first state: planned → pending.
//
// Fallout from 1826000000, which gave tickets a `type` of `planned`. Projects
// already had a `status` of `planned` meaning something else — "scoped but not
// started" — and the two met on one screen: the staff project detail view shows
// the project's own status badge above a ticket list whose planned tickets each
// carry their own `planned` badge. A planned project full of planned tickets is
// the same word twice, meaning two different things, in one view.
//
// The ticket axis keeps the word because that is where it does precise semantic
// work: planned vs. reactive IS the distinction, and it gates behaviour. On a
// project it was only ever the first of four lifecycle states, gating nothing.
//
// `pending` rather than `new`: "new" describes AGE, not state. A rollout scoped
// in March against an October target is not new by June, but it has still not
// started — and long lead times are precisely what the projects layer is for.
// `pending` stays true however far the target date is, and it collides with
// nothing else in the app (tickets use open/in_progress/waiting/resolved/closed,
// visits requested/scheduled/completed/canceled).
//
// The state itself is NOT dropped. Projects carry start_date and target_date
// because they are scoped before work begins; collapsing pending into active
// would erase the difference between "agreed for Q4" and "crews are on site",
// which is most of what the grouping layer is for.
//
// Unlike tickets.type, projects.status gates no behaviour anywhere — it drives a
// badge colour and a roster filter, nothing more. It stays a fixed select for
// the reason 1826000000 records (enums for what code branches on, collections
// for what only humans read): it is a LIFECYCLE, and lifecycles are closed sets
// whose transitions the app reasons about, even where today it only paints them.

func init() {
	m.Register(projectStatusPendingUp, projectStatusPendingDown)
}

// projectStatusRename mirrors ticketTypeRename (1826000000) — same shape, same
// reasons — but against projects.status.
func projectStatusRename(app core.App, rewrite map[string]string, want []string) error {
	projects, err := app.FindCollectionByNameOrId("projects")
	if err != nil {
		return fmt.Errorf("find projects: %w", err)
	}
	field, ok := projects.Fields.GetByName("status").(*core.SelectField)
	if !ok {
		return fmt.Errorf("projects.status is not a select field")
	}
	// Idempotent: already carrying the target values means this ran before.
	if hasValue(field.Values, want[0]) {
		return nil
	}
	field.Values = want
	if err := app.Save(projects); err != nil {
		return fmt.Errorf("save projects: %w", err)
	}

	// Raw SQL, not record saves: a pure column rewrite over every project, and
	// app.Save would fire the project hooks on each one.
	for old, next := range rewrite {
		if _, err := app.DB().
			NewQuery("UPDATE projects SET status = {:next} WHERE status = {:old}").
			Bind(map[string]any{"next": next, "old": old}).
			Execute(); err != nil {
			return fmt.Errorf("rewrite projects.status %s→%s: %w", old, next, err)
		}
	}
	return nil
}

func projectStatusPendingUp(app core.App) error {
	return projectStatusRename(app,
		map[string]string{"planned": "pending"},
		[]string{"pending", "active", "completed", "canceled"},
	)
}

func projectStatusPendingDown(app core.App) error {
	return projectStatusRename(app,
		map[string]string{"pending": "planned"},
		[]string{"planned", "active", "completed", "canceled"},
	)
}
