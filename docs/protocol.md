# Helpdesk wire contract

How tickets reach the helpdesk from outside the SPA: NATS machine events
and the authenticated HTTP webhook.

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
  "thing": "pump-7",                        // optional provenance hint
  "location": "line-3"                      // optional provenance hint
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
- **`thing` / `location`** are folded into the ticket body as a trailing
  `[thing: … · location: …]` line.
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
platform, scoped to `sub helpdesk.>`, delivered as a `.creds` file
(`nats.creds_file`).

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
  "dedupe_key": "alarm-1234"             // optional: idempotency key
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
