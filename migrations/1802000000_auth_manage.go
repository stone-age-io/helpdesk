package migrations

import (
	"fmt"

	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"

	"github.com/stone-age-io/helpdesk/internal/authz"
)

// PocketBase blocks direct writes to an auth record's email/password fields
// unless the request has "manage access" — granted only to PB superusers or
// callers matching the collection's ManageRule. The init migration set CRUD
// rules on `users` and `staff` but never ManageRule, so SPA admins (who are
// not PB superusers) cannot reset a requester's password or create staff
// accounts with a password from the UI (kiosk hit the identical wall —
// see kiosk migration 1788000000_auth_manage_rules).
//
// Setting ManageRule = adminRule grants admin staff manager-level access on
// both collections without touching the row-level rules.
//
// The same pass backfills emailVisibility=true on existing rows: PB defaults
// it to false, which masks email in API responses to everyone but the record
// owner — breaking the staff/requester list views. New rows are stamped by
// the authfix hook in cmd/helpdesk.

func init() {
	m.Register(addAuthManageUp, addAuthManageDown)
}

func addAuthManageUp(app core.App) error {
	for _, name := range []string{"users", "staff"} {
		col, err := app.FindCollectionByNameOrId(name)
		if err != nil {
			return fmt.Errorf("find %s: %w", name, err)
		}
		rule := authz.AdminRule
		col.ManageRule = &rule
		if err := app.Save(col); err != nil {
			return fmt.Errorf("save %s: %w", name, err)
		}

		rows, err := app.FindRecordsByFilter(name, "", "", 0, 0)
		if err != nil {
			return fmt.Errorf("list %s: %w", name, err)
		}
		for _, r := range rows {
			if r.EmailVisibility() {
				continue
			}
			r.SetEmailVisibility(true)
			if err := app.Save(r); err != nil {
				return fmt.Errorf("backfill emailVisibility on %s/%s: %w", name, r.Id, err)
			}
		}
	}
	return nil
}

func addAuthManageDown(app core.App) error {
	for _, name := range []string{"users", "staff"} {
		col, err := app.FindCollectionByNameOrId(name)
		if err != nil {
			continue
		}
		col.ManageRule = nil
		if err := app.Save(col); err != nil {
			return fmt.Errorf("save %s: %w", name, err)
		}
	}
	return nil
}
