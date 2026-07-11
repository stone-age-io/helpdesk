# Data model & access rules

The schema is Go-as-code in `migrations/`. `1800000000_init.go` creates every
collection and sets the baseline access rules; later timestamped migrations
amend specific pieces. This doc is the human-readable summary — the
migrations are the source of truth.

Tenancy is **plain PocketBase collection rules**, not pb-tenancy. Every rule
is built from four constants in `internal/authz`:

| Constant        | Expands to                                                     |
| --------------- | -------------------------------------------------------------- |
| `StaffRule`     | `@request.auth.collectionName = 'staff'`                       |
| `AdminRule`     | `StaffRule && @request.auth.role = 'admin'`                    |
| `RequesterRule` | `@request.auth.collectionName = 'users'`                       |

The whole security model reduces to: **staff see everything; a requester sees
only their own company's non-internal data.**

## Identity: two auth collections

Requests are attributed to one of two auth collections, distinguished in
rules by `@request.auth.collectionName`.

- **`staff`** — agents and admins, cross-customer. Fields: `name`, `role`
  (`agent` | `admin`), `active`. `AuthRule: active = true`. Any staff member
  can read the roster (needed for assignee pickers); only admins
  create/delete. A staff member may self-update profile fields but cannot
  change their own `role` or `active` (blocked by an `:isset` body guard).
- **`users`** — requesters (the repurposed default PB collection), scoped to
  one customer. Fields: `customer` (relation, **required**), `active`.
  `AuthRule: active = true && customer != ''`. A requester sees only
  themselves in the collection; only admins create/delete; self-update
  cannot reassign `customer` or toggle `active`.

Both collections stamp `emailVisibility = true` on create
(`internal/authfix`) — PB hides emails by default, which would break the
staff roster and pickers.

## Data collections

### `customers` — the company directory

`name` (unique), `active`, `platform_org_id` (unique when set — maps a
customer to the NATS subject org token), `webhook_token` (hidden; the inbound
webhook secret), `notes`.

Rules: read `StaffRule`; create/update/delete `AdminRule`. `webhook_token` is
a hidden field — it never leaves the server via the record API; staff reveal
or rotate it through `POST /api/helpdesk/customers/{id}/webhook-token`.

### `tickets` — the unit of work

`number` (unique int, assigned by the create hook), `customer` (required),
`title`, `body`, `status` (`open` | `in_progress` | `waiting` | `resolved` |
`closed`), `priority` (`low` | `normal` | `high` | `urgent`), `assignee`
(→ staff), `requester` (→ users, optional — machine tickets have none),
`source` (`portal` | `agent` | `nats` | `webhook`), `origin_subject` (the
full hub-side NATS subject, provenance for machine tickets), `dedupe_key`
(unique when set — ingestion idempotency), `attachments` (≤6 files).

Rules:

- **read** — `StaffRule || (RequesterRule && customer = @request.auth.customer)`.
  A requester sees only their own company's tickets.
- **create** — staff freely; a requester only for their own customer, with
  `requester` = themselves, no `assignee`, and `source = 'portal'` (all
  pinned in the create rule so the portal can't forge them).
- **update** — `StaffRule`. Requesters never edit ticket fields; they act
  through comments.
- **delete** — `AdminRule`.

### `ticket_comments` — the thread

`ticket` (required, cascade-delete), `author_staff` **or** `author_user`
(exactly one, matching the author's class), `body`, `internal` (bool —
staff-only working notes), `attachments` (≤6 files).

Rules:

- **read** — `StaffRule || (RequesterRule && ticket.customer =
  @request.auth.customer && internal = false)`. Internal notes never reach
  the portal, and attachments on them inherit that (PB gates file access by
  the record's view rule).
- **create** — staff set `author_staff` = themselves; a requester sets
  `author_user` = themselves, on their own company's ticket, and cannot set
  `internal` (guarded with `:isset`).
- **update/delete** — `AdminRule`.

### `ticket_events` — the audit trail (added `1805000000`)

One row per workflow-field change: `ticket` (cascade), `field`, `old_value`,
`new_value` (stored already human-readable), `actor_staff` / `actor_user`,
`created`. Written by `internal/activity`.

Rules: read `StaffRule` (the trail names technicians, so it is **never**
portal-side). No create/update/delete API rule — only the server hooks write
here, via `app.Save`, which bypasses collection rules, so the trail can't be
forged through the API.

### `time_entries` — labor log

`ticket` (cascade), `staff` (required), `minutes` (int ≥ 1), `work_date`,
`note`.

Rules: read `StaffRule` (staff-only, all ops). Create requires `staff` =
self; update/delete is own-entry-or-admin. Requesters never see time entries.

### `visits` — lite dispatch (relaxed `1803000000`, extended `1804000000`)

`ticket` (cascade), `assignee` (→ staff, optional), `scheduled_at`
(optional), `status` (`requested` | `scheduled` | `completed` | `canceled`),
`location` (free text — dispatch directions, no sites collection),
`completed_at`, `notes`.

`assignee` and `scheduled_at` are optional at the schema level so a
`requested` visit can exist before a tech or time is known. The one invariant
— a `scheduled` visit must have **both** — is enforced by the
`internal/visits` guard hook, not the schema.

Rules: read `StaffRule || (RequesterRule && ticket.customer =
@request.auth.customer)` — a requester sees visits on their own tickets
(someone unlocks the door for the tech), but the portal never expands
`assignee`, so the MSP roster stays hidden. All writes are `StaffRule`.

### Notification collections

`notification_templates`, `notification_dedupe`, `notification_send_log` —
lifted from the kiosk notifier. See `docs/notifications.md`.

## Idempotency & uniqueness indexes

These unique indexes are load-bearing, not just performance:

- `tickets.number` — the collision backstop for the sequential-number hook.
- `tickets.dedupe_key` (partial, `!= ''`) — absorbs NATS redelivery and
  webhook retries; a duplicate key is acked/answered without a second ticket.
- `customers.platform_org_id` (partial) — one customer per platform org.
- `customers.webhook_token` (partial) — token uniquely selects a customer.
- `notification_dedupe` (event, ref, UTC-day) — one send per event/ref/day.
