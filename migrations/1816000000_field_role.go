package migrations

import (
	"fmt"

	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

// Field agent role: a third `staff.role` value alongside agent/admin.
//
// A field agent is still ordinary staff — cross-customer, subject to every
// existing collection rule — so this is purely a new select value the SPA reads
// to pick a mobile, on-site shell (default landing = today's visits) instead of
// the desk app. Deliberately NOT a new auth collection and NOT a permission
// boundary: AdminRule still gates admin surfaces, and 1800000000's self-update
// guard (@request.body.role:isset = false) still blocks a field agent from
// self-promoting. The role only steers the UI (contrast the staff-vs-users
// split, which exists because requesters have a fundamentally different scope).
func init() {
	m.Register(fieldRoleUp, fieldRoleDown)
}

func fieldRoleUp(app core.App) error {
	staff, err := app.FindCollectionByNameOrId("staff")
	if err != nil {
		return fmt.Errorf("find staff: %w", err)
	}
	role, ok := staff.Fields.GetByName("role").(*core.SelectField)
	if !ok {
		return fmt.Errorf("staff.role is not a select field")
	}
	if !hasValue(role.Values, "field") {
		role.Values = append(role.Values, "field")
	}
	if err := app.Save(staff); err != nil {
		return fmt.Errorf("save staff: %w", err)
	}
	return nil
}

// fieldRoleDown is dev-loop only, like the other down paths: drop the value
// again. Any staff still on `field` would fail select validation on next save,
// but a down migration only runs against a throwaway dev DB.
func fieldRoleDown(app core.App) error {
	staff, err := app.FindCollectionByNameOrId("staff")
	if err != nil {
		return nil
	}
	role, ok := staff.Fields.GetByName("role").(*core.SelectField)
	if !ok {
		return nil
	}
	out := role.Values[:0]
	for _, v := range role.Values {
		if v != "field" {
			out = append(out, v)
		}
	}
	role.Values = out
	return app.Save(staff)
}

func hasValue(values []string, want string) bool {
	for _, v := range values {
		if v == want {
			return true
		}
	}
	return false
}
