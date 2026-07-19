package migrations

import (
	"encoding/json"
	"fmt"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"

	"github.com/stone-age-io/helpdesk/internal/notifications"
)

// Seeds the visit.completed notification template as a NATS-only event.
//
// Unlike the seven templates seeded by 1801 (email-first, publish_nats opt-in),
// visit.completed ships email-disabled and publish_nats-enabled: completion is
// already communicated to humans by the ticket's status/comments, so an inbox
// message would be noise — but the wire event is a valuable "work done on site"
// signal for MSP-internal automation (billing / CMDB sync / SLA close-out).
//
// This migration must run after 1814 (which adds publish_nats). It reconciles
// the row's channel state authoritatively so both install paths land correct:
//
//   - Fresh install: 1801's generic seeder pre-creates the row (visit.completed
//     is now in SeededEventTypes) with enabled=true and no publish_nats column
//     yet; 1814 then adds publish_nats=false. This migration flips it to the
//     NATS-only state.
//   - Existing DB: 1801 ran before visit.completed existed, so the row is
//     absent; this migration creates it.
//
// Idempotent: find-or-create, then force the channel fields either way.

func init() {
	m.Register(seedVisitCompletedTemplateUp, seedVisitCompletedTemplateDown)
}

func seedVisitCompletedTemplateUp(app core.App) error {
	col, err := app.FindCollectionByNameOrId(notifications.CollectionName)
	if err != nil {
		return fmt.Errorf("find %s: %w", notifications.CollectionName, err)
	}
	et := notifications.EventTypeVisitCompleted

	rec, _ := app.FindFirstRecordByFilter(
		notifications.CollectionName,
		"event_type = {:t}",
		dbx.Params{"t": et},
	)
	if rec == nil {
		rec = core.NewRecord(col)
		rec.Set("event_type", et)
	}

	subject, body, ok := notifications.Defaults(et)
	if !ok {
		return fmt.Errorf("no defaults registered for event type %q", et)
	}
	recipients, err := json.Marshal(notifications.DefaultRecipients(et))
	if err != nil {
		return fmt.Errorf("marshal recipients for %q: %w", et, err)
	}

	rec.Set("name", notifications.DefaultName(et))
	rec.Set("subject", subject)
	rec.Set("body", body)
	rec.Set("recipients", string(recipients))
	// NATS-only: email off, wire channel on.
	rec.Set("enabled", false)
	rec.Set("publish_nats", true)

	if err := app.Save(rec); err != nil {
		return fmt.Errorf("seed %q template: %w", et, err)
	}
	return nil
}

func seedVisitCompletedTemplateDown(app core.App) error {
	rec, _ := app.FindFirstRecordByFilter(
		notifications.CollectionName,
		"event_type = {:t}",
		dbx.Params{"t": notifications.EventTypeVisitCompleted},
	)
	if rec == nil {
		return nil
	}
	if err := app.Delete(rec); err != nil {
		return fmt.Errorf("delete visit.completed template: %w", err)
	}
	return nil
}
