package migrations

import (
	"fmt"

	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"

	"github.com/stone-age-io/helpdesk/internal/notifications"
)

// Adds the NATS publish channel to the notification subsystem:
//
//   - notification_templates.publish_nats (bool): per-event opt-in. When true,
//     the notifier publishes a fixed JSON envelope to
//     helpdesk.{customerId}.events.{event_type} in addition to (and independent
//     of) email. Defaults false — the zero value, so seeded rows stay
//     email-only until an admin opts in.
//   - notification_send_log.channel (select email|nats): which delivery path a
//     row records, so the audit trail covers both. Pre-existing rows have an
//     empty channel; the SPA treats empty as email.
//
// Both amendments are idempotent (skip if the field already exists), so the
// migration is safe to re-run and applies cleanly to fresh installs (after
// 1801 creates the collections) and existing local DBs alike.

func init() {
	m.Register(addNotificationsNATSUp, addNotificationsNATSDown)
}

func addNotificationsNATSUp(app core.App) error {
	if err := addField(app, notifications.CollectionName, &core.BoolField{Name: "publish_nats"}); err != nil {
		return err
	}
	return addField(app, notifications.SendLogCollectionName, &core.SelectField{
		Name:      "channel",
		Values:    []string{notifications.ChannelEmail, notifications.ChannelNATS},
		MaxSelect: 1,
	})
}

func addNotificationsNATSDown(app core.App) error {
	if err := removeField(app, notifications.CollectionName, "publish_nats"); err != nil {
		return err
	}
	return removeField(app, notifications.SendLogCollectionName, "channel")
}

// addField adds a field to an existing collection, skipping if a field of that
// name is already present (idempotent re-run).
func addField(app core.App, collection string, field core.Field) error {
	col, err := app.FindCollectionByNameOrId(collection)
	if err != nil {
		return fmt.Errorf("find %s: %w", collection, err)
	}
	if col.Fields.GetByName(field.GetName()) != nil {
		return nil
	}
	col.Fields.Add(field)
	if err := app.Save(col); err != nil {
		return fmt.Errorf("add %s.%s: %w", collection, field.GetName(), err)
	}
	return nil
}

// removeField drops a field from a collection if present (idempotent).
func removeField(app core.App, collection, field string) error {
	col, err := app.FindCollectionByNameOrId(collection)
	if err != nil {
		return nil // collection already gone (e.g. 1801 down ran first)
	}
	if col.Fields.GetByName(field) == nil {
		return nil
	}
	col.Fields.RemoveByName(field)
	if err := app.Save(col); err != nil {
		return fmt.Errorf("remove %s.%s: %w", collection, field, err)
	}
	return nil
}
