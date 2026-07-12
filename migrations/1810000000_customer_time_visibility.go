package migrations

import (
	"fmt"

	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

// Requesters can optionally see the AGGREGATE time logged on their own
// tickets — a per-customer opt-in (default off). Whether to show hours is an
// MSP billing-model choice (T&M shops want it; fixed-fee shops don't) and it's
// effectively irreversible once a customer has seen it, so it's off until an
// admin turns it on per customer. Only the total is ever exposed, via the
// internal/timeentries route — never the per-entry rows, which carry staff
// names and candid notes.

func init() {
	m.Register(customerTimeVisibilityUp, customerTimeVisibilityDown)
}

func customerTimeVisibilityUp(app core.App) error {
	customers, err := app.FindCollectionByNameOrId("customers")
	if err != nil {
		return fmt.Errorf("find customers: %w", err)
	}
	if customers.Fields.GetByName("show_time_to_requester") == nil {
		customers.Fields.Add(&core.BoolField{Name: "show_time_to_requester"})
		if err := app.Save(customers); err != nil {
			return fmt.Errorf("save customers: %w", err)
		}
	}
	return nil
}

// customerTimeVisibilityDown is dev-loop only, like the other down paths.
func customerTimeVisibilityDown(app core.App) error {
	if customers, err := app.FindCollectionByNameOrId("customers"); err == nil {
		customers.Fields.RemoveByName("show_time_to_requester")
		if err := app.Save(customers); err != nil {
			return fmt.Errorf("save customers: %w", err)
		}
	}
	return nil
}
