# Helpdesk wire contract

How tickets reach the helpdesk from outside the SPA (NATS machine events and
the authenticated HTTP webhook), and how the helpdesk emits its own events
back onto NATS (outbound notifications).

## NATS ticket events

### Subjects

Customer-side apps (things, rule-router) publish inside their own org's
NATS account:

```
helpdesk.tickets.create
```

The platform's managed-org export/import (platform commit `45ca1e3`)
delivers those into the operator hub account with the org id injected as
token 2:

```
helpdesk.{platformOrgId}.tickets.create
```

The injection is the provenance mechanism: the subject rewrite is signed by
the operator import, so a customer cannot spoof another org's id. Ingestion
therefore parses the org id **from the subject only** — an org id in the
payload is ignored.

v1 consumes only the `create` verb. The `{verb}` position deliberately
leaves room for `comment` / `resolve` later without a subject migration.

### Payload (`helpdesk.tickets.create`)

```json
{
  "title": "pump fault on line 3",          // required
  "body": "vibration sensor overcurrent",   // optional
  "priority": "high",                       // optional: low|normal|high|urgent (else normal)
  "dedupe_key": "pump-7-overcurrent",       // optional: idempotency key, unique per ticket
  "thing": "pump-7",                        // optional: stored as the ticket's asset
  "location": "line-3",                     // optional: free-text, stored as location_note
  "location_code": "BLDG-C",                // optional: resolves to a locations row (this customer)
  "category": "iot-device"                  // optional: a ticket_categories key
}
```

Behavior:

- **Unknown org** (no customer row with that `platform_org_id`): the event
  is logged (`ingest: no customer mapped for platform org`) and acked. Map
  the customer in the SPA and later events flow; the missed event is not
  replayed.
- **`dedupe_key`**: if a ticket with the same key exists, the event is
  acked without creating a second ticket. Publishers should stamp a stable
  key for retry loops and flapping sources.
- **`thing`** is stored as the ticket's structured `asset`; **`location`** is
  free text stored as `location_note`. **`location_code`** resolves against
  this customer's `locations` rows (matched on `code`) and sets the ticket's
  `location` relation — the queryable reporting axis. An unresolved code is
  logged and kept as a breadcrumb in `location_note` (no row is auto-created),
  so the operator can add the missing `locations` row and later events resolve.
- **`category`** is matched against a `ticket_categories` `key`; an unknown
  or inactive key is ignored (the ticket is still created, unclassified) —
  the same graceful-degradation stance as an unmapped org.
- The full hub-side subject is recorded on the ticket as `origin_subject`;
  `source` is `nats`.
- Malformed payloads and unsupported verbs are logged and acked (terminal —
  redelivery cannot fix them).

### Stream / consumer (helpdesk-owned)

The helpdesk creates and owns its inbox stream in the hub account:

- Stream `HELPDESK_EVENTS` (configurable: `nats.stream`), subjects
  `helpdesk.*.tickets.>`, file storage, 7-day age limit.
- Durable consumer `helpdesk-ingest` (configurable: `nats.durable`),
  explicit ack — restarts resume from the last-acked sequence.

The helpdesk's NATS identity is a hub-account `nats_user` minted by the
platform, scoped to `sub helpdesk.>` (plus `pub helpdesk.>` once outbound
notifications are enabled — see below), delivered as a `.creds` file
(`nats.creds_file`).

## NATS notification events (outbound)

The mirror of ingestion: when a notification template has `publish_nats`
enabled, the helpdesk publishes a fixed JSON envelope for that event onto the
hub account — for MSP-internal consumers (Slack/Teams bridges, on-call/paging,
metrics), **never** customers. This is a second delivery channel alongside
email; the two are configured and gated independently per template.

### Subjects

```
helpdesk.{customerId}.events.{event_type}
```

- `{customerId}` is the ticket's `customer` relation id — always present
  (required field) and token-safe. It is **not** the platform org id
  (`platform_org_id` is optional, so it would leave a hole); the org id rides
  the payload instead when known.
- `{event_type}` is the notification event (`ticket.created`,
  `ticket.status_changed`, `visit.scheduled`, …); its embedded dot supplies the
  trailing `domain.verb` tokens.

Token 3 is the literal `events`, which is what keeps this stream disjoint from
the ingest stream (`helpdesk.*.tickets.>`): `events` ≠ `tickets`, so JetStream
accepts both, and an outbound event can never be re-ingested as a ticket.

### Envelope (`schema: helpdesk.event`, `version: 1`)

```json
{
  "schema": "helpdesk.event",
  "version": 1,
  "event_type": "ticket.status_changed",
  "occurred_at": "2026-07-15T14:02:11Z",
  "customer": { "id": "cust123", "name": "Acme Corp", "platform_org_id": "org_..." },
  "ticket": {
    "id": "rec123", "number": 42, "title": "Pump fault on line 3",
    "status": "in_progress", "priority": "high", "type": "issue",
    "source": "nats", "url": "https://helpdesk.example.com/t/rec123",
    "assignee": { "name": "Sam Staff", "email": "sam@816tech.example" }
  },
  "change": { "field": "status", "from": "open", "to": "in_progress" },
  "comment": null,
  "visit": null
}
```

- `customer.platform_org_id` is omitted when the customer isn't mapped.
- `change` is present only for `ticket.status_changed`; `comment` only for
  `ticket.commented`; `visit` only for the `visit.*` events (`visit.scheduled`,
  `visit.rescheduled`, `visit.canceled`, `visit.completed`).
- The `visit` block carries `scheduled_at`, `assignee_name`, `location`, `notes`
  as available, plus `old_scheduled_at` (only on `visit.rescheduled`) and
  `completed_at` (only on `visit.completed`). Empty fields are omitted.
- `visit.completed` is **NATS-only** — it publishes on this channel but is
  email-disabled by default (see `docs/notifications.md`). It is the machine
  signal that on-site work finished; the other visit events also email.
- The consumer is MSP-internal, so staff identity (assignee) is included — the
  portal's roster-hiding does not apply here.

### Stream (helpdesk-owned)

- Stream `HELPDESK_NOTIFICATIONS` (configurable: `nats.notify_stream`), subjects
  `helpdesk.*.events.>`, file storage, 7-day age limit, 2-minute `Duplicates`
  window. Each publish carries a `Nats-Msg-Id` header
  (`{event_type}:{occurrenceKey}`) so a republished event collapses inside that
  window. The MSP's automation owns the **consumer**; the helpdesk only
  publishes.
- Best-effort: if the creds lack publish/stream-management or the stream can't
  be ensured at boot, the helpdesk logs once and email keeps working — NATS
  publishes become silent no-ops.

## HTTP webhook

```
POST /api/helpdesk/inbound/{token}
Content-Type: application/json
```

`{token}` is the per-customer shared secret (`customers.webhook_token`).
Admin staff reveal or rotate it from the customer detail view (server
routes: `POST /api/helpdesk/customers/{id}/webhook-token`, add `?rotate=1`
to regenerate). Possession of the token both authenticates the caller and
selects the customer. This route is the future email-provider
(Postmark/Mailgun) integration point.

### Payload

```json
{
  "title": "printer on fire",            // required
  "body": "3rd floor copy room",         // optional
  "priority": "urgent",                  // optional: low|normal|high|urgent (else normal)
  "requester_email": "rita@acme.com",    // optional: links an existing portal account
  "dedupe_key": "alarm-1234",            // optional: idempotency key
  "category": "hardware",                // optional: a ticket_categories key (unknown ignored)
  "asset": "printer-3f",                 // optional: free-text device/system
  "location": "3rd floor copy room",     // optional: free-text (location_note)
  "location_code": "BLDG-C"              // optional: resolves to a locations row (this customer)
}
```

### Responses

- `201` `{"id": "...", "number": 17, "duplicate": false}` — ticket created
  (`source = webhook`).
- `200` `{"id": "...", "number": 17, "duplicate": true}` — a ticket with
  this `dedupe_key` already exists; its identifiers are returned.
- `400` — missing/invalid title or malformed JSON.
- `404` — unknown token (same shape for an inactive customer; the route is
  not an oracle).

`requester_email` is matched only against portal accounts belonging to the
token's customer — a stray email can never link a ticket across tenants.
Non-matching emails are silently ignored (the ticket is still created,
unlinked).

`location_code` resolves to one of the token customer's `locations` rows (by
`code`) and sets the ticket's `location` relation; an unresolved code stays as
free text in `location_note` (same behavior, and same customer scoping, as the
NATS intake).
