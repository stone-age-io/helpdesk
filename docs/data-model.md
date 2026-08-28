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
  (`agent` | `admin` | `field` — the third value added by `1816000000`),
  `active`, `avatar` (single image, optional —
  migration `1807000000`). `AuthRule: active = true`. Any staff member
  can read the roster (needed for assignee pickers); only admins
  create/delete. A staff member may self-update profile fields (`name`,
  `avatar`) but cannot change their own `role` or `active` (blocked by an
  `:isset` body guard).

  `field` is **not a permission boundary** — a field agent is ordinary staff,
  cross-customer, subject to every rule an `agent` is, and `AdminRule` still
  gates the admin surfaces. It exists only so the SPA can pick a mobile,
  on-site shell (landing on today's visits) instead of the desk app. Contrast
  the `staff` / `users` split, which is a real boundary because requesters have
  a fundamentally different scope. If a future need makes `field` restrictive,
  that's a rule change, not a role rename.
- **`users`** — requesters (the repurposed default PB collection), scoped to
  one customer. Fields: `customer` (relation, **required**), `active`,
  `avatar` (single image, optional). `AuthRule: active = true && customer != ''`.
  A requester sees only themselves in the collection; only admins
  create/delete; self-update cannot reassign `customer` or toggle `active`.
  Also carries `phone` (added `1812000000`) — the requester's direct line,
  self-editable in the profile modal; the number a dispatcher/tech calls.

Both collections stamp `emailVisibility = true` on create
(`internal/authfix`) — PB hides emails by default, which would break the
staff roster and pickers.

## Data collections

### `customers` — the company directory

`name` (unique), `active`, `platform_org_id` (unique when set — maps a
customer to the NATS subject org token), `webhook_token` (hidden; the inbound
webhook secret), `email_domain` (optional, unique when set, added `1823000000` —
the customer's own mail domain, used to route inbound email from an unregistered
sender to this tenant; normalized and blocked from shared providers like
gmail.com by `internal/customers`), `notes`, `show_time_to_requester` (bool,
default false — added `1810000000`).

Rules: read `StaffRule`; create/update/delete `AdminRule`. `webhook_token` is
a hidden field — it never leaves the server via the record API; staff reveal
or rotate it through `POST /api/helpdesk/customers/{id}/webhook-token`.

`show_time_to_requester` is a per-customer opt-in (default off) that lets the
portal show the **aggregate** time logged on that customer's tickets — never
the per-entry rows. It gates the `GET /api/helpdesk/tickets/{id}/time-total`
route (`internal/timeentries`): staff always get the full total, a requester
only for their own customer's ticket and only when the flag is on — and the
requester's figure is **billable-only** (entries flagged `non_billable` are
excluded), so what the customer sees matches what they'd be invoiced. Off by
default because exposing hours is an MSP billing-model choice and hard to walk
back.

### `tickets` — the unit of work

`number` (unique int, assigned by the create hook), `customer` (required),
`title`, `body`, `status` (`open` | `in_progress` | `waiting` | `resolved` |
`closed`), `priority` (`low` | `normal` | `high` | `urgent`), `assignee`
(→ staff), `requester` (→ users, optional — machine tickets have none),
`source` (`portal` | `agent` | `nats` | `webhook` | `email`; `email` added
`1823000000`), `origin_subject` (the full hub-side NATS subject, provenance for
machine tickets), `dedupe_key` (unique when set — ingestion idempotency, also
carries the inbound email `Message-ID`), `attachments` (≤6 files),
`category` (→ ticket_categories, optional — see below), `type` (`issue` |
`install`, default `issue` via the create hook — reactive vs. planned work),
`project` (→ projects, optional — groups install/reactive work),
`location` (→ locations, optional — the structured place, and the reporting
axis), `location_note` (free text — dispatch hints, or the unmatched-code
fallback from machine intake). All added/changed `1812000000`.
`thing` (→ things, optional — the structured device, the second reporting axis)
and `thing_note` (free text — a scratch description, or the unmatched-code
fallback), which replaced the free-text `asset` in `1824000000`.
`estimated_minutes` (int ≥ 1, optional — staff effort estimate, added
`1815000000`; compared against the logged `time_entries` total per ticket and
summed per project at read time — see `projects`). Distinct from
`visits.duration_minutes` (a *calendar block*, not an *effort estimate*).
`awaiting_requester` (bool, added `1818000000`) — a queryable flag maintained by
`internal/tickets` backing the portal's "needs your reply" prompt / list chip /
dashboard tile. Staff-explicit: set only when a public staff comment ticks
*Request a reply* (`ticket_comments.requests_reply`, `1819000000`), cleared on a
requester reply or on resolve/close. `install` tickets are excluded. Not a
source of truth. `resolved_at` (datetime, optional, added `1821000000`) — stamped
by the `internal/tickets` guard when the ticket enters `resolved`, cleared when
it leaves (mirrors visits' `completed_at`); it gives the auto-close cron a
trustworthy age. Nil unless currently resolved.

**Two-stage terminal.** `resolved` and `closed` are *not* synonyms: `resolved`
is a grace window (a requester comment reopens it), `closed` is final (requesters
can't comment — see `ticket_comments` below — and a reply never reopens). A daily
cron (`tickets.AutoCloseResolved`, wired in `cmd/helpdesk`) promotes tickets left
`resolved` past `auto_close_resolved_days` (config, default 7; `0` disables) to
`closed`, suppressing the mail. Both still read as inactive in every "active"
query (`status != 'resolved' && status != 'closed'`), so the split left the
queues untouched. `waiting` remains an agent-set "blocked on a third party"
status, orthogonal to `awaiting_requester`.

Rules:

- **read** — `StaffRule || (RequesterRule && customer = @request.auth.customer)`.
  A requester sees only their own company's tickets.
- **create** — staff freely; a requester only for their own customer, with
  `requester` = themselves, no `assignee`, `source = 'portal'`, and none of
  `category` / `type` / `project` / `estimated_minutes` (all pinned in the create
  rule so the portal can't forge them — classification, the grouping fields, and
  the effort estimate are triage, i.e. staff actions).

  `location` and `thing` are the exception, since `1825000000`: a requester
  **may** set both, because they are not judgements about the work but facts
  about where it is and what it is on, and at intake the requester is the only
  one who knows them. Each is guarded by a tenant hop instead of an `:isset`
  ban:

  ```
  @request.body.location = '' || @request.body.location.customer = @request.auth.customer
  ```

  Both halves are load-bearing. PocketBase validates that a relation id
  *exists*, not that it is yours, so without the hop a requester could attach
  another customer's site to their ticket — and the ticket detail expands and
  renders it straight back. And the `= ''` term must be there (and must come
  first): `:isset` means "the key was submitted", so an untouched picker sending
  `location: ""` fails an `:isset = false` guard, which would reject every ticket
  filed without a site. `= ''` covers the absent case too, which is why the
  clause has two terms and not three. `1825000000_portal_site_device_test.go`
  pins all of it behaviourally, over real HTTP.

  The `_note` fallbacks stay **unguarded**: they're harmless free text, and
  they're how a requester names a site or device that isn't in the catalog —
  which is most of them.
- **update** — `StaffRule`. Requesters never edit ticket fields; they act
  through comments.
- **delete** — `AdminRule`.

### `ticket_categories` — classification (added `1806000000`)

Admin-managed list of what tickets are about: `name` (unique), `key` (unique
slug — the stable handle used in queue filters and machine payloads, so
renaming `name` never breaks them), `active` (retire without deleting
history), `sort_order`, `color` (hex, rendered as a soft badge).

A managed collection + relation rather than a `select` field because it is
staff/admin-managed from the SPA: admins add/retire categories with no code
deploy, renames touch one row (a select denormalizes the value onto every
ticket), and it matches the app's grain. The same reasoning later produced
`thing_types` / `location_types` (`1824000000`).

Both of the ticket's "what is this about" fields have since followed the same
path from free text to relation: `location` in `1812000000`, once projects made
physical places recur, and `asset` in `1824000000`, which replaced it with
`thing` (→ `things`) plus a `thing_note` fallback. The earlier "**not** a CMDB —
no device catalog" objection recorded here applied to an *authored* catalog
someone would have to keep true by hand. `things` isn't one: it is a curated
**mirror** of the platform's `things`, joined by `(customer, code)`, whose source
of truth lives upstream. What it buys is the pair of questions free text could
never answer — every ticket for this device, and which devices burn the most
hours.

Rules: read `StaffRule || RequesterRule` (staff use it for the picker;
requesters read it so a ticket's category **badge** resolves portal-side —
opened by `1808000000`; the taxonomy is non-sensitive labels);
create/update/delete `AdminRule`.

### `ticket_comments` — the thread

`ticket` (required, cascade-delete), `author_staff` **or** `author_user`
(exactly one, matching the author's class), `body`, `internal` (bool —
staff-only working notes), `attachments` (≤6 files), `source_message_id`
(hidden text, unique when set, added `1823000000` — the inbound email
`Message-ID`, so a redelivered reply can't post a duplicate comment; empty for
UI/portal comments).

Rules:

- **read** — `StaffRule || (RequesterRule && ticket.customer =
  @request.auth.customer && internal = false)`. Internal notes never reach
  the portal, and attachments on them inherit that (PB gates file access by
  the record's view rule).
- **create** — staff set `author_staff` = themselves; a requester sets
  `author_user` = themselves, on their own company's ticket, and cannot set
  `internal` (guarded with `:isset`). A requester also **can't comment on a
  `closed` ticket** (`@request.body.ticket.status != 'closed'`, amended by
  `1822000000`) — closed is final, a follow-up is a new ticket. Staff can still
  comment on closed tickets (their branch is unguarded).
- **update/delete** — `AdminRule`.

### `ticket_events` — the audit trail (added `1805000000`)

One row per workflow-field change: `ticket` (cascade), `field`, `old_value`,
`new_value` (stored already human-readable), `actor_staff` / `actor_user`,
`created`. Written by `internal/activity`. Audited fields: `status`,
`priority`, `assignee` plus the classification/grouping fields `category`,
`type`, `project`, `location` — relation values resolve to a label at write
time (category/location name, project `#N Title`).

Rules: read `StaffRule || (RequesterRule && field = 'status' &&
ticket.customer = @request.auth.customer)` — staff see the whole trail;
requesters see only **status** transitions on their own tickets, for the
portal progress timeline (amended by `1808000000`). All other events —
priority/assignee (staff names — the roster we hide) and the newer
category/type/project/location — never match, and the
actor relations stay staff-gated so an actor expand is dropped for a
requester. No create/update/delete API rule — only the server hooks write
here, via `app.Save`, which bypasses collection rules, so the trail can't be
forged through the API.

### `time_entries` — labor log

`ticket` (cascade), `staff` (required), `minutes` (int ≥ 1), `work_date`,
`note`, `visit` (→ visits, optional — added `1809000000`), `non_billable`
(bool, default false — added `1820000000`).

The ticket is the **canonical labor ledger**: `ticket` is required, so the
ticket total is always `sum(minutes)` filtered by ticket. `visit` is an
optional *dimension* on an entry — presence marks it as on-site/field time and
enables per-visit and field-vs-desk subtotals with no rollup machinery. No
cascade on the visit FK: deleting a visit never deletes labor (the entry keeps
its ticket; the dangling visit ref resolves to nothing).

`non_billable` marks labor not to be invoiced (rework, goodwill). It is stored
as the **exception** rather than a `billable` flag on purpose: a PocketBase bool
has no unset state, so its zero value is false — naming it `non_billable` makes
the default (unset) mean *billable*, which needs no backfill, no defaulting
hook, and no per-writer discipline (every writer, including a raw API create, is
safe by construction). Billability is a property of the *labor*, not the
*ticket* — one ticket routinely mixes billable work with non-billable rework.
Reports split on it (billable = total − non_billable, plus a write-off rate) and
the customer-facing time total (below) excludes it.

Rules: read `StaffRule` (staff-only, all ops). Create requires `staff` =
self; update/delete is own-entry-or-admin. Requesters never see time entries.

### `time_sessions` — running timer (added `1811000000`)

`staff` (required), `ticket` (cascade), `visit` (→ visits, optional, no
cascade), `started_at`, `note`. A row's existence means "this agent has a
timer running" — at most **one per agent**, enforced by a unique index on
`staff`. Stopping or canceling **deletes** the row; the durable record is the
`time_entries` row that `internal/timers` mints from it on stop. So this is the
ergonomic front-end to the labor log, *not* a second ledger — it holds only the
open interval's start and never accumulates history.

`started_at` is server-stamped by the `internal/timers` create hook (any client
value is ignored), so elapsed time is trustworthy. The stop route `POST
/api/helpdesk/timers/{id}/stop` resolves the timer into a `time_entries` row —
rounding elapsed to the nearest 5 min, or taking a caller-supplied `minutes`
override — and deletes the session atomically; with `complete_visit` it also
flips the attached visit to `completed` in the same transaction (the
`internal/visits` guard then stamps `completed_at`). Minute precision is
deliberately loose — the feature is about ergonomics, not the clock.

Rules: mirror `time_entries` — read `StaffRule`, create requires `staff` =
self, update/delete own-or-admin. Requesters never see it.

### `visits` — lite dispatch (relaxed `1803000000`, extended `1804000000`)

`ticket` (cascade), `assignee` (→ staff, optional), `scheduled_at`
(optional), `status` (`requested` | `scheduled` | `completed` | `canceled`),
`location` (free text — dispatch directions; the structured site comes from
the ticket's `location` relation), `completed_at`, `notes`, `duration_minutes`
(int, optional — added `1809000000`).

`duration_minutes` is the **scheduled** block length (planned), paired with
`scheduled_at` to make a visit a real calendar block rather than a point in
time. It is deliberately distinct from **actual** labor, which lives in
`time_entries` tagged with the visit.

`assignee` and `scheduled_at` are optional at the schema level so a
`requested` visit can exist before a tech or time is known. The one invariant
— a `scheduled` visit must have **both** — is enforced by the
`internal/visits` guard hook, not the schema.

Rules: read `StaffRule || (RequesterRule && ticket.customer =
@request.auth.customer)` — a requester sees visits on their own tickets
(someone unlocks the door for the tech), but the portal never expands
`assignee`, so the MSP roster stays hidden. All writes are `StaffRule`.

### `locations` — customer places (added `1812000000`)

`customer` (required), `code` (the platform Location join key, optional —
unique per customer when set), `name` (required), `address`, `notes` (gate
codes / access directions), `contact`, `contact_phone`, `lat` / `lng`
(optional coordinates, added `1813000000`). Machine intakes resolve a payload
`location_code` per `(customer, code)` and set the ticket's `location`
relation; an unmatched code falls back to `location_note`, no auto-stub
(`docs/protocol.md`). `lat`/`lng` are set from the map picker in the Locations
detail view and back a maps "Navigate" deep link on the ticket (coordinates
preferred, `address` as fallback). `1824000000` added `type` (→ location_types),
`parent` (self-relation, no cascade delete) and `metadata`, closing the shape gap
with the platform's `locations` so an export seeds without translation.

`parent` exists because the seeder writes it, not because the UI needs it —
dispatch is site-level, and a ticket naming a `thing` already carries the
precision a room-level hierarchy would provide. PocketBase has no cycle detection
for self-relations, so the parent picker excludes the record *and its whole
subtree*, and any code that walks the chain carries a visited set and a depth cap.
A dangling `parent` (deleted site) renders blank, like `tickets.category`.

A location earns a relation where a one-off ticket visit's free-text location
did not: a project revisits the same site over weeks, so the place recurs. It
is still deliberately **not** a CMDB — a place with an address and access
notes, not an asset catalog.

Rules: read `StaffRule || (RequesterRule && customer = @request.auth.customer)`
(a requester sees their own company's sites); **create** and **update**
`StaffRule` (any agent manages sites day-to-day from the Directory — update
opened in `1813000000`); **delete** `AdminRule` (the one destructive op against
a location referenced by tickets/projects/visits).

### `things` — the device catalog (added `1824000000`)

`customer` (required), `code` (the platform Thing join key, optional — unique per
customer when set), `name` (required), `type` (→ thing_types), `location`
(→ locations), `notes`, `retired` (bool), `metadata` (json). A subset mirror of
the platform's `things` minus its entire identity half (`password`, `tokenKey`,
`email`, `nats_user`, `nebula_host`) — those are control-plane and the helpdesk
must never hold them.

Machine intakes resolve a payload `thing_code` per `(customer, code)` and set the
ticket's `thing`; an unmatched code falls back to `thing_note`, no auto-stub
(`docs/protocol.md`). Same contract as `location_code`, and the two resolve
independently — a resolved thing does **not** backfill the ticket's location,
even though it has one.

Deliberately a **superset** of the platform's catalog: `code` is nullable because
MSP work covers printers, door strikes and customer switches that were never
onboarded to the control plane. There is no live sync and there cannot be one —
the platform publishes no event stream for things, and the only read paths are a
control-plane credential or an edge KV mirror. Bulk-loading is an operator-run
export→seed, which is why the shape stays faithful.

`retired` rather than the platform's `active` because a Go bool's zero value is
`false`: an `active` field would make every hand-created row arrive *inactive*
and be skipped by any `active = true` filter. Storing the exception is the
`time_entries.non_billable` idiom. A seeder maps `retired = !active`.

Rules: read `StaffRule || (RequesterRule && customer = @request.auth.customer)`;
create/update `StaffRule` (any agent curates inventory, and the ticket form
inline-creates); delete `AdminRule` (things are referenced by tickets, and
retiring is almost always the right move instead).

### `thing_types` / `location_types` — classifiers + metadata schemas (added `1824000000`)

`customer` (required), `code` (join key, unique per customer when set), `name`
(required), `description`, `metadata_schema` (json). Identical shape — the
platform's versions differ only in fields that are pure NATS contract
(`capabilities`, `subject_prefix`, `operations`, `nats_role`), none of which
crosses the boundary. They stay two collections because that is the platform's
shape and it keeps an export→seed a 1:1 map.

`metadata_schema` is the reason they exist: a JSON Schema naming the keys records
of that type track, so `metadata` doesn't degenerate into a bag of drifting key
spellings (`serial` / `Serial` / `sn`). A type with a schema gives its records a
typed metadata form; one without falls back to free-form key/value rows, which is
why a property-less schema is stored as `null` rather than
`{type:'object',properties:{}}`. Schemas are **authored upstream** in the platform
console and edited here as raw JSON, parse-validated before save.

Rules: read `StaffRule || (RequesterRule && customer = @request.auth.customer)` —
a requester's ticket may render a typed metadata field, so the schema has to be
readable; create/update/delete `AdminRule`, matching `ticket_categories`.

### `projects` — installation / field-work container (added `1812000000`)

`number` (unique int, assigned by the `internal/projects` create hook),
`customer` (required), `location` (→ locations, optional), `title` (required),
`description`, `status` (`planned` | `active` | `completed` | `canceled`,
default `planned`), `start_date` / `target_date` (the target window), `lead`
(→ staff, optional — whole-rollout accountability, distinct from the
per-ticket assignees).

A project is a planning-and-grouping layer **above** the ticket → visit → time
ledger: it groups 1..N tickets (often one `install` ticket per trade, plus any
reactive tickets) and stores none of their execution data. Crew (lead ∪
ticket/visit assignees), total logged time, and total estimated effort
(`sum(ticket.estimated_minutes)`, shown as an estimated-vs-logged bar) are all
**derived** at read time via relation-hop queries on `ticket.project`, never
stored — so the project collection could be dropped and the helpdesk would
still work.

Rules: read `StaffRule || (RequesterRule && customer = @request.auth.customer)`
— a requester sees their own company's projects (the portal shows the tickets
and visits but never the `lead`/crew). create/update `StaffRule`; delete
`AdminRule`.

### Notification collections

`notification_templates`, `notification_dedupe`, `notification_send_log` —
lifted from the kiosk notifier. See `docs/notifications.md`. Two channels per
template: `enabled` (email) and `publish_nats` (publish a JSON envelope to
`helpdesk.{customerId}.events.{event_type}`, migration `1814000000`);
`notification_send_log.channel` (`email` | `nats`) records which path each row
is for.

## Idempotency & uniqueness indexes

These unique indexes are load-bearing, not just performance:

- `tickets.number` — the collision backstop for the sequential-number hook.
- `tickets.dedupe_key` (partial, `!= ''`) — absorbs NATS redelivery and
  webhook retries; a duplicate key is acked/answered without a second ticket.
- `customers.platform_org_id` (partial) — one customer per platform org.
- `customers.webhook_token` (partial) — token uniquely selects a customer.
- `customers.email_domain` (partial, `!= ''`) — a mail domain maps to one tenant.
- `ticket_comments.source_message_id` (partial, `!= ''`) — an inbound email
  `Message-ID` posts at most one comment (redelivery idempotency).
- `ticket_categories.name` / `.key` — categories are distinct; `key` is the
  stable filter/payload handle.
- `locations` (customer, code), `things` (customer, code), and
  `thing_types` / `location_types` (customer, code) — all partial on
  `code != ''`. A code is unique **within a customer** (it's the machine-intake
  and export→seed join key); two tenants both calling a reader `RDR-01` is
  correct. The partial predicate is what keeps a blank code legal, and blanks
  must stay legal: SQLite treats `''` as a value rather than NULL, and the
  catalog is deliberately a superset of the platform's — it covers gear that
  was never onboarded and therefore has no code.
- `time_sessions.staff` — one running timer per agent, enforced by the DB
  rather than by the stop route. This is what makes "start a timer" idempotent.
- `projects.number` — the collision backstop for the project-number hook.
- `notification_dedupe` (event, ref, UTC-day) — one send per event/ref/day.
- `notification_templates.event_type` and `customers.name` — one row per event,
  one company per name.
