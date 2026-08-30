package migrations

import (
	"fmt"

	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

// Customers gain a `code`: the tenant token of the ecosystem's public namespace
// (ADR 0002 in platform-docs).
//
// The helpdesk already resolves everything else by (customer, code) — a machine
// intake's `location_code` and `thing_code` both land that way. What it could
// not resolve by code was the CUSTOMER, because the only tenant handle it held
// was `platform_org_id`: a PocketBase primary key belonging to another
// application's database. That is what subject token 2 has carried since
// 1800000000, and it is what ADR 0002 replaces with a code the whole ecosystem
// agrees on.
//
// This migration lands BEFORE the platform starts emitting codes, and that
// order is not incidental. Ingestion resolves token 2 against this collection;
// an unresolved org is logged and ACKED, not retried, so a platform that
// switched first would drop every machine-filed ticket on the floor with
// nothing but a log line to show for it. Readers before writers.
//
// Two shape notes:
//
// `code` is optional and the unique index is PARTIAL, matching platform-side
// and matching `platform_org_id` right beside it. The helpdesk deliberately
// serves customers the platform never onboarded, and those have no code until
// an operator assigns one — the same reason `things.code` is nullable here.
//
// `platform_org_id` is NOT dropped and NOT merged into this. It keeps a job the
// code cannot do: it answers "is this customer actually a platform
// organization", which stays a real question for a service desk whose customer
// list is a superset of the control plane's tenant list.
func init() {
	m.Register(customerCodeUp, customerCodeDown)
}

func customerCodeUp(app core.App) error {
	customers, err := app.FindCollectionByNameOrId("customers")
	if err != nil {
		return fmt.Errorf("find customers: %w", err)
	}
	if customers.Fields.GetByName("code") != nil {
		return nil // idempotent
	}

	// Max 31 to match the platform's ^[a-z][a-z0-9-]{1,30}$. The pattern itself
	// is deliberately NOT enforced here: this side never mints a code, it only
	// matches one an operator copied across, and a validator that disagreed with
	// the platform's by a character would reject a legitimate tenant.
	customers.Fields.Add(&core.TextField{Name: "code", Max: 31})

	// Partial, so the blanks that are the normal state for a non-platform
	// customer do not collide with each other — SQLite treats '' as a value.
	customers.AddIndex("idx_customers_code", true, "code", "code != ''")

	if err := app.Save(customers); err != nil {
		return fmt.Errorf("save customers: %w", err)
	}
	return nil
}

func customerCodeDown(app core.App) error {
	customers, err := app.FindCollectionByNameOrId("customers")
	if err != nil {
		return fmt.Errorf("find customers: %w", err)
	}
	customers.RemoveIndex("idx_customers_code")
	customers.Fields.RemoveByName("code")
	return app.Save(customers)
}
