// Package authfix holds small hooks that adjust PocketBase's default auth
// semantics to match this app's needs (kiosk convention). Each function
// takes a core.App and must be called exactly once at process boot.
package authfix

import "github.com/pocketbase/pocketbase/core"

// EnforceEmailVisibility stamps emailVisibility=true on every new staff and
// requester row before save. PB defaults the field to false, which masks
// the email in API responses to anyone but the record owner — breaking the
// staff/requester list views and assignee pickers. Fires on create only;
// existing rows are covered by the 1802000000_auth_manage backfill.
func EnforceEmailVisibility(app core.App) {
	app.OnRecordCreate("users", "staff").BindFunc(func(e *core.RecordEvent) error {
		e.Record.SetEmailVisibility(true)
		return e.Next()
	})
}
