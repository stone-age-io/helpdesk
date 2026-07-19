# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with
code in this repository.

## What this is

`helpdesk` is 816tech's (MSP / platform operator) service-desk app for the
Stone-Age.io ecosystem: reactive support tickets **and** proactive
project / installation / field work. One Go binary (`cmd/helpdesk`) embedding
PocketBase plus a Vue 3 SPA with two shells: the staff app (`/staff/...`) and
the requester portal (`/portal/...`). The signature feature is machine-generated
tickets ingested from NATS with subject-based provenance; humans use the
portal, the staff app, or an authenticated webhook. Projects and locations
(migration `1812000000`) add a planning-and-grouping layer *above* the
ticket → visit → time ledger without changing it (see **Projects / locations**
below and `docs/service-delivery-plan.md`). The `helpdesk` name is retained as
the technical identifier — notably the operator-signed `helpdesk.>` NATS
contract — even as the product's scope has grown past a help desk.

It is a **standalone sibling app** to kiosk and access-control — NOT a
platform feature. Helpdesk agents must never hold control-plane credentials,
and the tenancy axes differ (platform tenant = customer org; helpdesk
tenant = the MSP itself). Follow the sibling conventions when in doubt; this
codebase deliberately lifts proven patterns from kiosk (notifier, durable
consumer, test harness) and access-control (bootstrap, natsx, subjects,
embedded UI).

## Build & run

The SPA is `//go:embed`-ed at Go compile time from `internal/webui/public`,
which is **committed** so a fresh checkout builds without npm. Rebuild and
re-commit that directory whenever anything under `ui/` changes:

```bash
cd ui && npm ci && npm run build   # vue-tsc + vite build → ../internal/webui/public
cd .. && go build ./cmd/helpdesk
./helpdesk serve                   # SPA at /, PocketBase dashboard at /_
go test ./...                      # real PB against t.TempDir(), full migrations
```

First boot seeds a bootstrap staff admin (`admin@helpdesk.local`, password
printed once to stdout). Config: `helpdesk.yaml` (or `$HELPDESK_CONFIG`)
plus `HELPDESK_*` env overrides — see `docs/configuration.md`. SMTP and the
application URL live in PocketBase settings (dashboard), not the YAML.

## Architecture

**Identity: two auth collections**, distinguished in rules by
`@request.auth.collectionName` (constants in `internal/authz`):

- `staff` — agents/admins (`role` select). Cross-customer. `AdminRule`
  gates management surfaces.
- `users` — requesters, repurposed default PB collection with a required
  `customer` relation. `AuthRule: active = true && customer != ''`. A
  requester sees only their own company's tickets and only non-internal
  comments — enforced by collection rules in `migrations/1800000000_init.go`,
  the home of all access rules (later migrations may amend specific rules,
  e.g. `1803000000` opens visit reads to requesters, `1808000000` opens
  status-only ticket_events + category reads for the portal timeline, and
  `1813000000` opens locations update to any staff);
  `docs/data-model.md`
  summarizes every collection and rule. Both auth collections stamp
  `emailVisibility = true` on create (`internal/authfix`) — PB masks emails
  by default, which would otherwise break the staff roster, the assignee
  pickers, and requester lists.

**Ticket lifecycle** (`internal/tickets`): a create hook assigns the next
sequential `number` (unique index is the collision backstop) and defaults
status/priority/source. Requesters interact via comments; ticket field
updates are staff-only by rule. A public requester comment on a
`resolved`/`closed` ticket **auto-reopens** it — silently, since the comment
itself already emailed staff. The same comment hooks maintain
`awaiting_requester` (migration `1818000000`), a derived boolean = "last public
comment was staff's and the ticket is still open" — set true on a public staff
comment, cleared on a requester reply or on resolve/close (pre-save). It's a
cheap-to-query cache (not a new source of truth) that powers the portal's
"needs your reply" prompt, list chip, and dashboard tile. Tickets and comments
carry **attachments**
(≤6 files, 10 MB each); PB serves files only to callers who can view the
owning record, so attachments on internal comments stay staff-only.
Classification (migration `1806000000`): an optional `category` (admin-managed
`ticket_categories` **relation**, not a select — staff-classified). A ticket
also carries a `type` (`issue` | `install`) and an optional `project`, free-text
`asset`, a structured `location` (→ `locations`) and a free-text `location_note`
fallback (all migration `1812000000`). An optional `estimated_minutes` (staff
effort estimate, migration `1815000000`) is compared against the logged
`time_entries` total per ticket and summed per project at read time — one
nullable column, distinct from `visits.duration_minutes` (a calendar block, not
an effort estimate). The portal create rule blocks requesters from setting
`category` / `type` / `project` / `location` / `estimated_minutes` (via
`:isset = false`); machine intakes set `asset` and resolve a payload
`location_code` to the `location` relation (unmatched → `location_note`).
`docs/data-model.md` covers it.

**Audit trail** (`internal/activity`): every workflow-field change (status,
priority, assignee, plus the classification/grouping fields category, type,
project, location) writes a `ticket_events` row rendered as a staff-only
timeline; relation values are resolved to labels at write time (category/
location name, project `#N Title`). Reads are staff-only (the trail names
technicians); it has no
create/update API rule — the hooks write it server-side via `app.Save`
(which bypasses collection rules), so it can't be forged or edited through
the record API. The actor comes from request auth, or is set explicitly with
`activity.SetActor` for server-initiated changes (e.g. the requester whose
comment auto-reopened a ticket). Values are stored already human-readable
(assignee resolved to a name), so the timeline needs no expands beyond the
actor.

**Visits / lite dispatch** (`internal/visits`): promoting a ticket to
on-site work = creating a visit; the ticket schema stays untouched
("needs on-site" is derived, never stored). Lifecycle
`requested → scheduled → completed | canceled` with no transition
enforcement — the one invariant, enforced by a pre-save guard hook, is
that a *scheduled* visit has both `scheduled_at` and `assignee` (both
optional at the schema level so a `requested` visit can exist before the
dispatcher picks a tech/time; empty status defaults from whether a time
is set). Free-text `location` carries dispatch directions; the structured
site (address, access notes) now comes from the ticket's `location` relation
(the `locations` collection, migration `1812000000`). A visit entering
`completed` stamps `completed_at`
(guard hook — back-datable, cleared if it leaves `completed`), giving the
Dispatch history a trustworthy "who went, when" that `updated` (bumps on any
edit) couldn't. Staff schedule from the ticket detail card or the
Dispatch view (needs-scheduling bucket sorted client-side by ticket
priority — a PB relation-hop sort on a select would be alphabetical —
plus a day-grouped, filterable list). Requesters get read-only visit
access via a `ticket.customer` relation-hop rule; the portal never shows
the technician's name (expand on `assignee` is dropped by `staff`'s
ViewRule, and relaxing it would leak the MSP roster).

**Projects / locations** (`internal/projects`, migration `1812000000`): the
service-delivery layer. A **location** is a customer's physical place with an
optional `code` — the join key to the platform's Location concept: machine
intakes resolve a payload `location_code` per `(customer, code)` and set the
ticket's `location` relation (unmatched → free-text `location_note`, no
auto-stub), making location a queryable dimension (tickets/installs/visits/time
by location). Still not a CMDB — a place, not an asset catalog. Locations live
in the Directory and any staff member creates/edits them via a detail view
(`1813000000`; delete stays admin); optional `lat`/`lng` come from a Leaflet
map picker (Nominatim address search) and drive a maps "Navigate" deep link on
the ticket. A **project**
groups 1..N tickets (often one `install`-type ticket per trade, plus reactive
tickets) at a location over a target window; sequential `number` (hook, like
tickets) and a single `lead` for whole-rollout accountability. Crucially it is
a *grouping layer above the ledger* — visits and time stay parented to tickets,
and a project's **crew** (lead ∪ ticket/visit assignees) and **total time** are
derived at read time via relation-hop queries on `ticket.project`, never stored
(the collection could be dropped and the app would still work). Requesters get
a read-only portal project view scoped by `ticket.customer` that never shows the
lead/crew (same roster-hiding as visits). The tripwire for splitting this into
its own sibling app: the project side needing its own portal/tenancy/ingestion —
not merely re-parenting a visit. `docs/service-delivery-plan.md` has the full
rationale.

**Time tracking** (`internal/timeentries`, `internal/timers`): labor is a
`time_entries` row (minutes + `work_date` + optional `visit` tag) — the ticket
is the canonical ledger, and `GET /tickets/{id}/time-total` exposes only the
sum, gated per-customer by `show_time_to_requester`. Agents either log minutes
by hand or run a **start/stop timer**: one open `time_sessions` row per agent
(unique index on `staff`; `started_at` server-stamped by the create hook),
resolved into a normal `time_entries` row by `POST
/api/helpdesk/timers/{id}/stop` (rounds elapsed to 5 min unless given a
`minutes` override; `complete_visit` also flips the attached visit to
`completed`, atomically). The timer is UX only — *not* a second ledger, and
minute precision is deliberately loose. Staff drive it from the ticket Time
card, the visit drawer, or the mobile-first visit **work view**
(`/staff/visits/:id/work`: Arrive → live timer → Complete).

**Outbound email + events** (`internal/notifications`, lifted from kiosk's
notifier):
DB-stored templates (`notification_templates`) rendered with
`text/template` + a small FuncMap (`formatTime`, `statusLabel`,
`pluralize`). Seven event types: `ticket.created`, `ticket.assigned`,
`ticket.commented`, `ticket.status_changed`, `visit.scheduled`,
`visit.rescheduled`, `visit.canceled`. Visit events fire on *transitions*,
not raw saves: scheduled = became scheduled (create or update),
rescheduled = time moved while scheduled, canceled = was scheduled
(canceling a bare `requested` visit is silent, as is completion and a
tech swap without a time change). The visit's technician overrides the
ticket assignee in the payload so the person dispatched gets the mail.
Recipients
are a per-template JSON spec `{requester, assignee, all_staff, extras}`;
the payload (`TicketContext`) implements `RequesterEmail()` /
`AssigneeEmail()` and *suppresses the author's side* on comment events so
nobody is emailed about their own comment. A save can also silence its own
mail two ways: the staff UI sends `X-Helpdesk-Quiet: 1` on a ticket update
that shouldn't email anyone (triage, mis-set-status fix, internal
reassignment), and `notifications.Suppress(rec)` marks a server-initiated
change whose message already went out another way (the auto-reopen); both
ride a transient record flag from the request hook into the after-success
hook. Hooks fire on
`OnRecordAfter*Success` (status/assignee diffs read `Record.Original()`,
which still holds pre-update values in the after-success hook — verified).
Sends are async goroutines; a nil notifier and missing SMTP are both clean
no-ops; every attempt writes `notification_send_log`; `SendIfFirst`
dedupes per (event, ref, UTC-day) via `notification_dedupe`'s unique index.
A daily cron in `cmd/helpdesk/main.go` prunes both tables at 90 days.
Editor API under `/api/helpdesk/notifications` (admin staff only; PATCH
parse-validates templates before save; compiled-in defaults back the
"Reset to defaults" affordance). Email deep links use the role-neutral SPA
route `/t/{id}`. **Second channel:** each template also carries a
`publish_nats` toggle (migration `1814000000`); when on, the same event
`dispatch` also publishes a fixed, versioned JSON envelope (`envelope.go`,
`schema: helpdesk.event`) to `helpdesk.{customerId}.events.{event_type}` via
an injected `Publisher` (nil until NATS connects → clean no-op, independent
of email). The channel is for MSP-internal consumers so the envelope is rich
(no portal redaction); the human suppression rules gate email only. `channel`
(`email`|`nats`) on the send log distinguishes rows. `docs/notifications.md`
has the full event / recipient / suppression matrix; `docs/protocol.md` has
the outbound envelope + subject contract.

**NATS ingestion** (`internal/subjects`, `internal/natsx`,
`internal/ingest`): customer apps publish `helpdesk.tickets.create` in
their own org account; the platform's managed-org export (platform commit
`45ca1e3`) delivers it hub-side as `helpdesk.{platformOrgId}.tickets.create`.
The org id is parsed **from the subject only** (token 2) — the export's
subject rewrite is operator-signed, so it's unforgeable; a payload org id
would not be. The helpdesk owns its inbox stream `HELPDESK_EVENTS`
(subjects `helpdesk.*.tickets.>`) and a durable consumer `helpdesk-ingest`.
Projection semantics: unknown org → warn + ack (operator sets
`customers.platform_org_id`, later events flow); `dedupe_key` + unique
partial index absorb redelivery/publisher retries; bad payloads ack
(terminal). NATS is **best-effort**: connect failure logs and the app
serves anyway. Auth is a platform-minted hub `nats_user` scoped to
`sub helpdesk.>` (widened to `pub helpdesk.>` for outbound notifications),
via creds file. The helpdesk also **owns a second, outbound** stream
`HELPDESK_NOTIFICATIONS` (subjects `helpdesk.*.events.>`, config
`nats.notify_stream`) for the notification publish channel — deliberately
disjoint from the ingest stream at token 3 (`events` vs `tickets`), so
JetStream accepts both and an emitted event can't loop back through ingest.
The MSP's automation owns that stream's consumer; the helpdesk only
publishes.

**Webhook inbound** (`internal/inbound`):
`POST /api/helpdesk/inbound/{token}` resolves the customer by
`webhook_token` (hidden field; admin-only reveal/rotate route at
`POST /api/helpdesk/customers/{id}/webhook-token[?rotate=1]` — minted on
first reveal). Idempotent via `dedupe_key` (200 + `duplicate:true`).
`requester_email` matches only within the token's customer. This route is
the future email-provider (Postmark/Mailgun) integration point. Wire
contract for both intakes: `docs/protocol.md`.

**UI** (`ui/`): Vue 3 + Vite + Pinia + Tailwind + daisyUI (custom light/dark
theme + soft badges, `ui/tailwind.config.js` + `.badge-soft*` in
`src/style.css`) + Leaflet (lazy-loaded location map picker), PocketBase JS
SDK, same-origin (`new PocketBase('/')`). One login page tries `staff`
then falls back to `users`; router guards by auth collection
(`meta.requires`), plus `meta.adminOnly` for admin surfaces (staff,
categories, notifications).
`/t/:id` forwards to the right detail view by role (bounces through login
with a `redirect` query).

## Conventions

- **Migrations are Go schema-as-code** in `migrations/`, timestamp-prefixed,
  idempotent (guard with FindCollectionByNameOrId), registered by
  side-effect import. Access rules use `internal/authz` constants. New
  seeded notification templates: append to `notifications.SeededEventTypes`
  + `Defaults` + `DefaultName` + `DefaultRecipients` and seed in a new
  migration.
- **Tests** use `testutil.SetupApp(t)` — a real PocketBase against
  `t.TempDir()` with all migrations applied. Notification tests capture
  mail via `OnMailerSend` (bind, record, don't call `e.Next()`). DB-backed
  notification tests live in the **external** `notifications_test` package
  because `migrations` imports `notifications` (import cycle otherwise).
  NATS projection is tested by calling `ingest.(*Consumer).Project`
  directly — no broker in tests (sibling convention).
- **gotcha**: `.gitignore` anchors the built binary as `/helpdesk` — a bare
  `helpdesk` pattern would ignore `cmd/helpdesk/` too (this bit us once).
  Similarly `config.Load` must not `SetConfigType`, or viper matches the
  extensionless binary as a config file.
- Notification sends from hooks mean **any** app.Save on
  tickets/comments/visits fires email — tests that save those records and
  assert on mail must drain with `WaitInFlight`.

## Out of scope (v1, deliberate)

Native SMTP inbound, request/reply NATS service, SLA timers/escalation,
knowledge base, canned responses, CSAT, ticket merge/split, magic links,
multi-MSP hosting (one helpdesk instance per MSP), calendar sync for
visits. See `docs/plan.md` for the full plan this repo implements.
