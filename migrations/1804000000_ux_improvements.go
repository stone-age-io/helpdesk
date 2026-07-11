package migrations

import (
	"fmt"

	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

// Three small quality-of-life schema additions, all optional and additive:
//
//   - visits.completed_at — a trustworthy completion timestamp, stamped by
//     the internal/visits guard when a visit enters `completed`. Turns the
//     Dispatch view into a real "who went, when" history (the tech is the
//     visit's `assignee`); `updated` was an unreliable proxy since it bumps
//     on any edit.
//   - tickets.attachments / ticket_comments.attachments — file uploads so a
//     requester can attach a screenshot of the problem and staff can send
//     back a document. Files on internal comments inherit that collection's
//     requester-hiding list rule, so they never leak.
//
// No access-rule changes: the existing create rules don't restrict these
// fields, and PocketBase serves files only to callers who can view the
// owning record.

func init() {
	m.Register(uxImprovementsUp, uxImprovementsDown)
}

func uxImprovementsUp(app core.App) error {
	visits, err := app.FindCollectionByNameOrId("visits")
	if err != nil {
		return fmt.Errorf("find visits: %w", err)
	}
	if visits.Fields.GetByName("completed_at") == nil {
		visits.Fields.Add(&core.DateField{Name: "completed_at"})
		if err := app.Save(visits); err != nil {
			return fmt.Errorf("save visits: %w", err)
		}
	}

	// attachments: up to six files, 10MB each. MimeTypes left open — staff
	// and authenticated requesters are the only writers, and files are served
	// under random names gated by the record's view rule.
	for _, name := range []string{"tickets", "ticket_comments"} {
		col, err := app.FindCollectionByNameOrId(name)
		if err != nil {
			return fmt.Errorf("find %s: %w", name, err)
		}
		if col.Fields.GetByName("attachments") == nil {
			col.Fields.Add(&core.FileField{Name: "attachments", MaxSelect: 6, MaxSize: 10 << 20})
			if err := app.Save(col); err != nil {
				return fmt.Errorf("save %s: %w", name, err)
			}
		}
	}
	return nil
}

// uxImprovementsDown is dev-loop only, like the other down paths.
func uxImprovementsDown(app core.App) error {
	specs := []struct{ col, field string }{
		{"visits", "completed_at"},
		{"tickets", "attachments"},
		{"ticket_comments", "attachments"},
	}
	for _, s := range specs {
		col, err := app.FindCollectionByNameOrId(s.col)
		if err != nil {
			continue
		}
		col.Fields.RemoveByName(s.field)
		if err := app.Save(col); err != nil {
			return fmt.Errorf("save %s: %w", s.col, err)
		}
	}
	return nil
}
