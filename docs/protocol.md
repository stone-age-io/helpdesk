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
  "thing": "pump-7",                        // optional: free-text, stored as thing_note
  "thing_code": "PUMP-7",                   // optional: resolves to a things row (this customer)
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
- **`thing`** and **`location`** are free text, stored as `thing_note` and
  `location_note`. **`thing_code`** and **`location_code`** are the platform
  join keys: each resolves against this customer's `things` / `locations` rows
  (matched on `code`) and sets the corresponding relation — the queryable
  reporting axes. An unresolved code is logged and kept as a breadcrumb in its
  own note field (no row is auto-created), so the operator can add the missing
  row and later events resolve.

  The two resolve independently: a resolved `thing` does **not** backfill the
  ticket's `location`, even though the thing record has one. One payload field
  maps to one ticket field; inference belongs in the UI, not the projection.

  Both codes are `(customer, code)`-scoped, so a code can never resolve across
  tenants. Note that the platform does **not** freeze `things.code` or
  `locations.code` — renaming one upstream means later events stop resolving and
  fall back to free text until the helpdesk row is updated to match.
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
    "status": "in_progress", "priority": "high", "type": "reactive",
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
  "thing": "printer-3f",                 // optional: free-text (thing_note)
  "thing_code": "HQ-PRN-3",              // optional: resolves to a things row (this customer)
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

`thing_code` and `location_code` resolve to one of the token customer's `things`
/ `locations` rows (by `code`) and set the corresponding relation; an unresolved
code stays as free text in its note field (same behavior, and same customer
scoping, as the NATS intake).

The free-text device field is spelled **`thing`**, matching the NATS contract —
it was `asset` before the `things` collection existed.

## HTTP inbound (email provider)

```
POST /api/helpdesk/inbound/email/{provider}   # {provider} = postmark
Authorization: Basic <base64(user:secret)>
Content-Type: application/json
```

A distinct intake for **email**: an email-parsing provider (Postmark to start)
receives forwarded mail, parses the MIME, and posts its own JSON here. The route
exists only when `inbound.secret` is configured; the caller authenticates with
that secret via Basic auth (optionally IP-pinned to `inbound.allowed_ips`).
Unlike the token webhook, the tenant is **not** in the URL — it is resolved from
the sender. The full design (forwarding, threading, resolution ladder,
provider-agnostic core) is in [`email-ingestion.md`](email-ingestion.md); the
wire contract:

- The provider's payload is provider-specific (a thin adapter maps it to an
  internal `NormalizedInbound`). For Postmark the fields read are `MessageID`,
  `FromFull`, `Subject`, `StrippedTextReply`/`TextBody`, and `Headers`. Ingestion
  is text-only; attachments are ignored (a non-goal — see `email-ingestion.md`).
- **Threading:** a `[#N]` token in the subject routes a reply onto ticket N as a
  public comment (reopening it if `resolved`; a `closed` ticket instead spawns a
  new one). No token ⇒ a new ticket, `source = email`.
- **Tenant:** the sender resolves to a customer by exact `users.email`, else by
  `customers.email_domain` (never a shared provider like gmail.com). Unresolvable
  ⇒ the message is acked and dropped, not funneled to a catch-all.
- **Idempotency:** the email `Message-ID` dedupes both paths (`tickets.dedupe_key`
  and the hidden `ticket_comments.source_message_id`, each unique).

### Responses

Every intentionally-handled or intentionally-dropped message returns **2xx**, so
the provider stops retrying:

- `200` `{"status": "created|commented|duplicate", "id": "...", "number": 17}` —
  ticket created, reply threaded, or a redelivery deduped.
- `200` `{"status": "ignored", "reason": "..."}` — deliberately dropped
  (unresolved tenant, spam, or an auto-reply/loop).
- `401` — missing/invalid Basic-auth secret. `403` — caller IP not allowed.
- `422` — undecodable JSON body.
- `500` — genuine server fault (the provider should retry).
