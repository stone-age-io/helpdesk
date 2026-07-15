package migrations

import (
	"fmt"

	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"

	"github.com/stone-age-io/helpdesk/internal/authz"
)

// Locations become agent-editable and gain coordinates.
//
// 1812000000 created `locations` with an admin-only UpdateRule so only admins
// curated the roster. In practice agents manage sites day-to-day, so the
// helpdesk moves Locations into the Directory and opens update to any staff;
// delete stays admin as the one destructive op (a location is referenced by
// tickets/projects/visits). `lat`/`lng` back the map pin and a maps deep link
// on the ticket — both optional, so a hand-entered site without coordinates
// stays valid. Create was already staff (1812, for inline quick-create).
func init() {
	m.Register(locationsAgentGeoUp, locationsAgentGeoDown)
}

func locationsAgentGeoUp(app core.App) error {
	loc, err := app.FindCollectionByNameOrId("locations")
	if err != nil {
		return fmt.Errorf("find locations: %w", err)
	}

	staffRule := authz.StaffRule
	loc.UpdateRule = &staffRule

	if loc.Fields.GetByName("lat") == nil {
		loc.Fields.Add(&core.NumberField{Name: "lat"})
	}
	if loc.Fields.GetByName("lng") == nil {
		loc.Fields.Add(&core.NumberField{Name: "lng"})
	}

	if err := app.Save(loc); err != nil {
		return fmt.Errorf("save locations: %w", err)
	}
	return nil
}

func locationsAgentGeoDown(app core.App) error {
	loc, err := app.FindCollectionByNameOrId("locations")
	if err != nil {
		return nil // collection already dropped by 1812000000's down migration
	}
	adminRule := authz.AdminRule
	loc.UpdateRule = &adminRule
	loc.Fields.RemoveByName("lat")
	loc.Fields.RemoveByName("lng")
	return app.Save(loc)
}
