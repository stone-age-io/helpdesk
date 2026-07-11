# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with
code in this repository.

## What this is

`helpdesk` is 816tech's (MSP / platform operator) service-ticket app for the
Stone-Age.io ecosystem. One Go binary (`cmd/helpdesk`) embedding PocketBase
plus a Vue 3 SPA with two shells: the staff app (`/staff/...`) and the
requester portal (`/portal/...`). The signature feature is machine-generated
tickets ingested from NATS with subject-based provenance; humans use the
portal, the staff app, or an authenticated webhook.

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
  e.g. `1803000000` opens visit reads to requesters, and `1808000000` opens
  status-only ticket_events + category reads for the portal timeline);
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
itself already emailed staff. Tickets and comments carry **attachments**
(≤6 files, 10 MB each); PB serves files only to callers who can view the
owning record, so attachments on internal comments stay staff-only.
Classification (migration `1806000000`): an optional `category` (admin-managed
`ticket_categories` **relation**, not a select — staff-classified, the portal
create rule blocks requesters via `category:isset = false`) plus free-text
`asset`/`location` (the pragmatic "item" tier — provenance, deliberately not a
CMDB; machine intakes populate them from the payload). `docs/data-model.md`
covers it.

**Audit trail** (`internal/activity`): every workflow-field change (status,
priority, assignee) writes a `ticket_events` row rendered as a staff-only
timeline. Reads are staff-only (the trail names technicians); it has no
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
is set). Free-text `location` carries dispatch directions — deliberately
no sites collection. A visit entering `completed` stamps `completed_at`
(guard hook — back-datable, cleared if it leaves `completed`), giving the
Dispatch history a trustworthy "who went, when" that `updated` (bumps on any
edit) couldn't. Staff schedule from the ticket detail card or the
Dispatch view (needs-scheduling bucket sorted client-side by ticket
priority — a PB relation-hop sort on a select would be alphabetical —
plus a day-grouped, filterable list). Requesters get read-only visit
access via a `ticket.customer` relation-hop rule; the portal never shows
the technician's name (expand on `assignee` is dropped by `staff`'s
ViewRule, and relaxing it would leak the MSP roster).

**Outbound email** (`internal/notifications`, lifted from kiosk's notifier):
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
route `/t/{id}`. `docs/notifications.md` has the full event / recipient /
suppression matrix.

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
`sub helpdesk.>`, via creds file.

**Webhook inbound** (`internal/inbound`):
`POST /api/helpdesk/inbound/{token}` resolves the customer by
`webhook_token` (hidden field; admin-only reveal/rotate route at
`POST /api/helpdesk/customers/{id}/webhook-token[?rotate=1]` — minted on
first reveal). Idempotent via `dedupe_key` (200 + `duplicate:true`).
`requester_email` matches only within the token's customer. This route is
the future email-provider (Postmark/Mailgun) integration point. Wire
contract for both intakes: `docs/protocol.md`.

**UI** (`ui/`): Vue 3 + Vite + Pinia + Tailwind + daisyUI, PocketBase JS
SDK, same-origin (`new PocketBase('/')`). One login page tries `staff`
then falls back to `users`; router guards by auth collection
(`meta.requires`), plus `meta.adminOnly` for the notification editor.
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
