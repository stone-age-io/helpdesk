package migrations

import (
	"encoding/json"
	"fmt"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"

	"github.com/stone-age-io/helpdesk/internal/authz"
	"github.com/stone-age-io/helpdesk/internal/notifications"
)

// Adds the outbound-email subsystem's three collections (kiosk notifier
// pattern, created here in its final shape rather than replaying kiosk's
// incremental history):
//
//   - notification_templates: admin-editable subject/body/recipients per
//     event type, seeded from the compiled-in defaults. Create + delete
//     rules are nil (API is read + update only) — rows are seeded here and
//     new built-ins ship by appending to notifications.SeededEventTypes().
//   - notification_dedupe: the (event_type, ref, day) tuples that already
//     fired today. The unique composite index is the enforcement mechanism
//     for SendIfFirst — no race between check and send.
//   - notification_send_log: one row per attempted recipient with status
//     sent/failed/skipped. Indexes cover the SPA's filter-and-sort access
//     pattern and the retention cron's created-based purge.

func init() {
	m.Register(addNotificationsUp, addNotificationsDown)
}

func addNotificationsUp(app core.App) error {
	templates, err := createNotificationTemplatesCollection(app)
	if err != nil {
		return err
	}
	if err := seedNotificationTemplates(app, templates); err != nil {
		return err
	}
	if err := createNotificationDedupeCollection(app); err != nil {
		return err
	}
	return createNotificationSendLogCollection(app, templates)
}

func addNotificationsDown(app core.App) error {
	for _, name := range []string{
		notifications.SendLogCollectionName,
		notifications.DedupeCollectionName,
		notifications.CollectionName,
	} {
		col, err := app.FindCollectionByNameOrId(name)
		if err != nil {
			continue
		}
		if err := app.Delete(col); err != nil {
			return fmt.Errorf("delete %s: %w", name, err)
		}
	}
	return nil
}

func createNotificationTemplatesCollection(app core.App) (*core.Collection, error) {
	if existing, err := app.FindCollectionByNameOrId(notifications.CollectionName); err == nil {
		return existing, nil
	}

	col := core.NewBaseCollection(notifications.CollectionName)
	col.Fields.Add(&core.TextField{Name: "event_type", Required: true})
	col.Fields.Add(&core.TextField{Name: "name", Required: true})
	col.Fields.Add(&core.BoolField{Name: "enabled"})
	col.Fields.Add(&core.TextField{Name: "subject"})
	col.Fields.Add(&core.TextField{Name: "body"})
	col.Fields.Add(&core.JSONField{Name: "recipients"})
	col.Fields.Add(&core.TextField{Name: "updated_by"})
	col.Fields.Add(&core.AutodateField{Name: "created", OnCreate: true})
	col.Fields.Add(&core.AutodateField{Name: "updated", OnCreate: true, OnUpdate: true})

	col.AddIndex("idx_notification_templates_event_type", true, "event_type", "")

	adminRule := authz.AdminRule
	col.ListRule = &adminRule
	col.ViewRule = &adminRule
	col.UpdateRule = &adminRule
	// CreateRule + DeleteRule intentionally nil — rows are seeded by this
	// migration and the API surface is read + update only.

	if err := app.Save(col); err != nil {
		return nil, fmt.Errorf("save %s: %w", notifications.CollectionName, err)
	}
	return col, nil
}

// seedNotificationTemplates inserts one row per built-in event type if it
// doesn't already exist. Re-runs after a partial application are safe.
func seedNotificationTemplates(app core.App, col *core.Collection) error {
	for _, et := range notifications.SeededEventTypes() {
		existing, _ := app.FindFirstRecordByFilter(
			notifications.CollectionName,
			"event_type = {:t}",
			dbx.Params{"t": et},
		)
		if existing != nil {
			continue
		}
		subject, body, ok := notifications.Defaults(et)
		if !ok {
			return fmt.Errorf("no defaults registered for event type %q", et)
		}
		recipients, err := json.Marshal(notifications.DefaultRecipients(et))
		if err != nil {
			return fmt.Errorf("marshal recipients for %q: %w", et, err)
		}
		rec := core.NewRecord(col)
		rec.Set("event_type", et)
		rec.Set("name", notifications.DefaultName(et))
		rec.Set("enabled", true)
		rec.Set("subject", subject)
		rec.Set("body", body)
		rec.Set("recipients", string(recipients))
		if err := app.Save(rec); err != nil {
			return fmt.Errorf("seed template %q: %w", et, err)
		}
	}
	return nil
}

func createNotificationDedupeCollection(app core.App) error {
	if _, err := app.FindCollectionByNameOrId(notifications.DedupeCollectionName); err == nil {
		return nil
	}
	col := core.NewBaseCollection(notifications.DedupeCollectionName)
	col.Fields.Add(&core.TextField{Name: "event_type", Required: true})
	col.Fields.Add(&core.TextField{Name: "ref", Required: true})
	col.Fields.Add(&core.TextField{Name: "day", Required: true})
	col.Fields.Add(&core.AutodateField{Name: "created", OnCreate: true})

	// Unique on the triple — SendIfFirst relies on this for race-free
	// "first fire of the day per key" gating.
	col.AddIndex("idx_notification_dedupe_triple", true, "event_type, ref, day", "")
	col.AddIndex("idx_notification_dedupe_created", false, "created", "")

	adminRule := authz.AdminRule
	col.ListRule = &adminRule
	col.ViewRule = &adminRule
	// Write rules nil — only the notifier inserts, via app.Save.

	if err := app.Save(col); err != nil {
		return fmt.Errorf("save %s: %w", notifications.DedupeCollectionName, err)
	}
	return nil
}

func createNotificationSendLogCollection(app core.App, templates *core.Collection) error {
	if _, err := app.FindCollectionByNameOrId(notifications.SendLogCollectionName); err == nil {
		return nil
	}

	col := core.NewBaseCollection(notifications.SendLogCollectionName)
	col.Fields.Add(&core.TextField{Name: "event_type", Required: true})
	col.Fields.Add(&core.RelationField{
		Name:         "template",
		CollectionId: templates.Id,
		MaxSelect:    1,
		// CascadeDelete false: log rows survive template deletes for audit.
	})
	col.Fields.Add(&core.TextField{Name: "recipient"})
	col.Fields.Add(&core.SelectField{
		Name:      "status",
		Values:    []string{notifications.SendStatusSent, notifications.SendStatusFailed, notifications.SendStatusSkipped},
		Required:  true,
		MaxSelect: 1,
	})
	col.Fields.Add(&core.TextField{Name: "error"})
	col.Fields.Add(&core.TextField{Name: "payload_summary"})
	col.Fields.Add(&core.AutodateField{Name: "created", OnCreate: true})

	col.AddIndex("idx_send_log_event_created", false, "event_type, created", "")
	col.AddIndex("idx_send_log_status", false, "status", "")
	col.AddIndex("idx_send_log_created", false, "created", "")

	adminRule := authz.AdminRule
	col.ListRule = &adminRule
	col.ViewRule = &adminRule
	// Create/update/delete rules intentionally nil — only the notifier
	// writes (via app.Save on a hydrated record) and the retention cron
	// deletes (via app.Delete, bypassing collection rules).

	if err := app.Save(col); err != nil {
		return fmt.Errorf("save %s: %w", notifications.SendLogCollectionName, err)
	}
	return nil
}
