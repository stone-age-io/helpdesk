package migrations

import (
	"fmt"

	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

// Optional profile avatars for staff and requesters: one self-uploaded image
// per account. Additive and idempotent, like the attachments migration.
//
// No access-rule change is needed: both auth collections are already
// self-updatable for profile fields (the staff/users update rules only block
// role/customer/active escalation, not new fields), and PocketBase serves the
// file only to callers who can view the owning record — the roster is
// staff-readable and a requester can view their own record. Thumbs let the UI
// request a small square instead of shipping the full upload on every render.

func init() {
	m.Register(avatarsUp, avatarsDown)
}

func avatarsUp(app core.App) error {
	for _, name := range []string{"staff", "users"} {
		col, err := app.FindCollectionByNameOrId(name)
		if err != nil {
			return fmt.Errorf("find %s: %w", name, err)
		}
		if col.Fields.GetByName("avatar") == nil {
			col.Fields.Add(&core.FileField{
				Name:      "avatar",
				MaxSelect: 1,
				MaxSize:   2 << 20, // 2 MB
				MimeTypes: []string{"image/png", "image/jpeg", "image/webp", "image/gif"},
				Thumbs:    []string{"100x100", "40x40"},
			})
			if err := app.Save(col); err != nil {
				return fmt.Errorf("save %s: %w", name, err)
			}
		}
	}
	return nil
}

// avatarsDown is dev-loop only, like the other down paths.
func avatarsDown(app core.App) error {
	for _, name := range []string{"staff", "users"} {
		col, err := app.FindCollectionByNameOrId(name)
		if err != nil {
			continue
		}
		col.Fields.RemoveByName("avatar")
		if err := app.Save(col); err != nil {
			return fmt.Errorf("save %s: %w", name, err)
		}
	}
	return nil
}
