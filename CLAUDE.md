# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with
code in this repository.

## What this is

`helpdesk` is the service-desk app for the Stone-Age.io ecosystem, run by the
ecosystem's **operator** — the MSP that operates the platform and supports the
customer organizations on it. Reactive support tickets **and** proactive
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

An optional operator **branding overlay** (`branding.dir`, env
`HELPDESK_BRANDING_DIR`, empty by default) lets an install override the UI's app
name, logo, and DaisyUI theme **without rebuilding**: `serveBranding` serves that
host directory's `theme.css`/`logo.svg`/`branding.json` under `/branding/*` (the
route is always registered and returns a silent empty `theme.css`/`{}`
`branding.json` when unconfigured, so a stock install never 404s). `index.html`
`<link>`s `/branding/theme.css`; `stores/branding.ts` fetches `branding.json`
pre-mount; `BrandLogo.vue` prefers the overlay logo, else the built-in inline
mark. `branding.example/` is the committed template. Mirrors the sibling
`platform`/`access-control` apps' branding system.

Each shell carries its own stock wordmark (staff/field "Service Desk", portal
"Support"), which reads better than one name repeated everywhere — but an
overlay `appName` must **replace** those, not sit beside them, or a branded
install shows the operator's logo next to our name. That is what
`branding.shellName(fallback)` is for, and why the store tracks
`nameFromOverlay` separately: `appName` alone can't distinguish "the operator
chose this" from "nobody configured anything". Every shell and the sign-in card
go through it. Adding a new wordmark means calling `shellName()`, never
hardcoding the literal — the sidebar's `brand` prop used to be passed by every
caller, which silently made the overlay name dead everywhere it mattered.

The SPA is also an **installable PWA**, same shape as the platform app:
`ui/public/` holds `manifest.json`, the two icons (the Stone-Age mark in the
helpdesk indigo), and a deliberately **no-op `sw.js`** — the service worker
exists only to satisfy the browser's install criteria, so there is no offline
cache to go stale (an app whose whole job is live ticket state should not serve
yesterday's queue). `main.ts` registers it in PROD only; `index.html` carries the
manifest / theme-color / apple-touch-icon tags. Vite copies `ui/public/` into
`internal/webui/public`, and `serveSPA`'s file lookup serves them at the root.
Unlike the theme and app name, the manifest is **not** brandable — it's a static
file, so an operator overlay can't rename the installed app.

## Architecture

**Identity: two auth collections**, distinguished in rules by
`@request.auth.collectionName` (constants in `internal/authz`):

- `staff` — agents/admins/field techs (`role` select: `agent` | `admin` |
  `field`, the last added by `1816000000` and **UI-steering only, not a
  permission boundary** — it picks the mobile on-site shell). Cross-customer.
  `AdminRule`
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
updates are staff-only by rule. Status is a **two-stage terminal**:
`resolved` is a grace window — a public requester comment **auto-reopens** it
(silently, since the comment already emailed staff) — and `closed` is final:
requesters can't comment on a closed ticket (create rule, migration
`1822000000`; the portal shows "open a new ticket" instead) and a reply never
reopens it. Entering `resolved` stamps a nullable **`resolved_at`** (migration
`1821000000`, cleared when it leaves — mirrors visits' `completed_at`); a daily
cron (`AutoCloseResolved`, `cmd/helpdesk` cron `auto_close_resolved`) promotes
tickets left resolved past `auto_close_resolved_days` (config, default 7; `0`
disables) to `closed`, suppressing the mail (administrative, not a "we closed
your ticket" message) while still writing the status `ticket_events` row. Both
`resolved` and `closed` count as inactive everywhere ("active" queries are
`status != 'resolved' && status != 'closed'`), so the two-stage split didn't
touch the queues. `waiting` stays an agent-set "blocked on a third party"
status, distinct from the `awaiting_requester` nudge. The same comment hooks
maintain
`awaiting_requester` (migration `1818000000`), a cheap-to-query boolean that
powers the portal's "needs your reply" prompt, list chip, and dashboard tile.
It is **staff-explicit, not inferred**: a public staff comment sets it only when
the author ticks *Request a reply* (`ticket_comments.requests_reply`, migration
`1819000000`) — a plain status update doesn't nag the customer — and it's
cleared on a requester reply or on resolve/close (pre-save). `planned` tickets
are excluded entirely (proactive field work isn't a reply-driven conversation).
Not a source of truth. Tickets and comments carry **attachments**
(≤6 files, 10 MB each); PB serves files only to callers who can view the
owning record, so attachments on internal comments stay staff-only.
Classification (migration `1806000000`): an optional `category` (admin-managed
`ticket_categories` **relation**, not a select — staff-classified). A ticket
also carries a `type` (`reactive` | `planned`, migration `1826000000` — they
were `issue`/`install`, which weren't parallel nouns and undersold what the
value gates) and an optional `project`, a
structured `location` (→ `locations`) with a free-text `location_note` fallback
(migration `1812000000`), and — mirroring that pair exactly — a structured
`thing` (→ `things`) with a free-text `thing_note` fallback (migration
`1824000000`, which dropped the earlier free-text `asset`). An optional
`estimated_minutes` (staff effort estimate, migration `1815000000`) is compared
against the logged `time_entries` total per ticket and summed per project at read
time — one nullable column, distinct from `visits.duration_minutes` (a calendar
block, not an effort estimate). The portal create rule blocks requesters from
setting the triage fields `category` / `type` / `project` / `estimated_minutes`
(via `:isset = false`), while both `_note` fallbacks stay unguarded (harmless
free text). `location` and `thing` were in that list until migration
`1825000000`, which opened both to requesters — they aren't judgements about the
work, they're facts about where it is and what it's on, and at intake the
requester is the only one who knows them. Each swapped its `:isset` ban for a
**tenant hop**, `@request.body.<rel> = '' || @request.body.<rel>.customer =
@request.auth.customer`. Both terms matter: PocketBase validates that a relation
id *exists*, not that it's yours (so the hop is what stops a cross-tenant
attach), and `:isset` means "key was submitted", so an untouched picker sending
`""` would fail an `:isset = false` guard and reject every ticket filed without
a site. The migration comment carries the measured truth table; the tests pin it
over real HTTP (`testutil.Handler` — the first rule tests in this repo that
execute a rule rather than assert on its string). Machine intakes resolve payload `location_code` /
`thing_code` to their relations, unmatched → the matching note, no auto-stub.
`docs/data-model.md` covers it.

`category` and `type` look like the same kind of field and are not, which is
worth stating once: **enums for what the code branches on, collections for what
only humans read.** `category` is inert — a label, so it's an admin-managed
collection that grows without a deploy. `type` gates behaviour (`planned`
tickets never get the `awaiting_requester` nag; every report axis counts them
as their own measure), so it stays a fixed select — an admin-invented type would
have no defined meaning to either. `status` and `priority` sit on the same side
for the same reason. More granular *kinds* of planned work (install vs. survey
vs. replacement) are a labelling need and belong on `category` or a new axis,
never on `type`. Projects' own lifecycle is `pending | active | completed |
canceled` (`1827000000`) — `pending` was `planned` until a ticket `type` of
that name arrived, and the two shared the project detail screen: a planned
project full of planned tickets, one word meaning two things. The ticket axis
kept the word because that is where it carries the distinction; on a project it
was only the first of four states.

**Audit trail** (`internal/activity`): every workflow-field change (status,
priority, assignee, plus the classification/grouping fields category, type,
project, location, thing) writes a `ticket_events` row rendered as a staff-only
timeline; relation values are resolved to labels at write time (category/
location/thing name, project `#N Title`). Reads are staff-only (the trail names
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
by location). Locations live
in the Directory and any staff member creates/edits them via a detail view
(`1813000000`; delete stays admin); optional `lat`/`lng` come from a Leaflet
map picker (Nominatim address search) and drive a maps "Navigate" deep link on
the ticket. `1824000000` added `type`, `parent` and `metadata` to close the
shape gap with the platform's `locations`. A **project**
groups 1..N tickets (often one `planned`-type ticket per trade, plus reactive
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

The project detail view is **read mode as prose, not the form greyed out**, and
that is where it parts company with `LocationDetailView`, which it otherwise
mirrors. A location record is mostly a form and is opened to be edited; a
project is the status page for a rollout and is read many times for every time
it is changed, so a disabled `<input>` holding the title and a fixed four-row
`<textarea>` holding the scope — the same height empty or full — were furniture
in the way of the answer. Editing swaps each card to its controls; nothing else
changes shape. The visits the page had always fetched to build the crew set are
now *rendered* too: a lead could see who was on the rollout but not when anyone
was going.

Both record lists there are **bounded and expand in place** rather than linking
out, unlike the "View all →" idiom the thing and location detail views use — a
link has nowhere to point, because the staff queue carries no project filter and
this is the only page holding a project's tickets. The rollups that must cover
*every* ticket (summed estimates) and every time entry get their own
`fields`-trimmed queries rather than folding the visible page: a rollout's
ledger is the largest thing on the page and none of the rest of a time entry is
read here.

**Preventive maintenance** (`internal/maintenance`, migration `1829000000`):
recurrence, and the last CMMS pillar this app was missing. A
**maintenance_plan** says "this gets serviced every N days"; its only output is
an ordinary ticket (`type = planned`, `source = maintenance`), so it is a
planning layer *above* the ledger in exactly the sense projects are — visits,
time, the audit trail, portal visibility, notifications and every report already
work on tickets and needed no changes. Drop the collection and the app still
runs; only future generation stops.

`anchor` picks one of two behaviours, and each stays simple because each has
exactly **one writer** of `next_due`: `schedule` means the cron advances it by
whole intervals at generation ("quarterly stays quarterly however late the visit
ran"), while `completion` means the cron **parks** the plan by clearing
`next_due` and a ticket hook restarts it from `resolved_at + interval_days`
("every 90 days after last service"). The generator's `next_due != ''` filter is
what makes a parked plan invisible to it — so a completion-anchored plan is
*structurally* unable to stack up work, and the skip-if-still-open guard only
ever runs on `schedule` plans. A skip still advances: one inspection nobody did
must not become four open tickets. `paused`, not `active` — the
`things.retired` / `non_billable` idiom, so a new plan's zero value means
"running". Idempotency reuses `tickets.dedupe_key` as `pm:{planId}:{date}`, so
the 03:45 cron and a hand-run `helpdesk maintenance-run` (the
catch-up path, since PB's cron is process-local) can overlap harmlessly. The
cron runs *after* `auto_close_resolved` on purpose. Generation deliberately does
**not** `Suppress` mail — the opposite call from auto-close, because a new
preventive ticket is real news, and a plan has no requester so only staff are
mailed. **No config knob**: an install with no plans generates nothing and one
plan pauses with one toggle, so a flag would be a second disable for the same
thing.

`tickets.due_at` ships with it: a plain nullable date, **not an SLA clock**
(timers and escalation stay out of scope) — nothing measures it and nothing
escalates off it. It is audited, guarded staff-only in the portal create rule
alongside the other triage fields, and surfaced as a queue column + `due` filter
and a Dashboard card. Those buckets live in `ui/src/due.ts` and are shared by
both, deliberately unlike the neighbouring backlog-age buckets, which are
written twice and kept in sync only by comments. That module also owns
`formatDay` / `isPastDue`, and the reason is a real bug caught in review:
`due_at` is a *calendar date* stored at UTC midnight, so the `pbTime`
local→UTC conversion that is right for `visits.scheduled_at` (an instant) shifts
it by the offset — west of Greenwich everything due today read as overdue and
rendered a day early. Compare and format the date half directly.

**Things / types** (migration `1824000000`): the device axis, and the last
free-text ticket field promoted to a relation. A **thing** is a subset mirror of
the platform's `things` — minus its entire identity half (`password`, `tokenKey`,
`email`, `nats_user`, `nebula_host`), which is control-plane and must never cross
the boundary — joined by `(customer, code)` exactly like locations, so
`thing_code` on either intake resolves to the relation (unmatched → `thing_note`,
no auto-stub). It exists to answer the two questions free text couldn't: *every
ticket for this device* and *which devices burn the most hours*.

There is **no live sync and there cannot be one**: the platform publishes no
event stream for things, so the only read paths are a control-plane credential
(forbidden here) or an edge KV mirror. This is a hand-curated local catalog that
*joins* upstream by code, and deliberately a **superset** of it — `code` is
nullable because MSP work covers gear the platform never onboarded. Bulk-loading
is an operator-run export→seed, which is why both shapes stay faithful; note that
`stone` and leaf-sync store raw PocketBase ids for `type`/`location`/`parent`, so
a seeder needs the full exported set to build an id→code map before it can
rewrite any relation.

`thing_types` / `location_types` are the classifiers, and `metadata_schema` on
them is the point: a JSON Schema naming the keys records of that type track, so
`metadata` doesn't become a bag of drifting key spellings. Schemas are authored
upstream and edited here as raw JSON (parse-validated on save) — the helpdesk
renders schemas, it never builds them, which is why the platform's visual
SchemaBuilder is deliberately not ported while `MetadataEditor` /
`JsonSchemaForm` are. `retired` rather than the platform's `active` so the bool's
zero value means "in service" (the `non_billable` idiom); a seeder maps
`retired = !active`.

**Vocabulary: it is "location" and "thing" everywhere, UI copy included.** Not
"site" and "device" — both shells and the portal drifted into using all four
interchangeably, to the point that one ticket form labelled the same field
"Thing" while its placeholder said "Pick a device…" and its create button said
"New thing". A per-audience split (schema nouns for staff, plain English for the
portal) is the obvious alternative and is **unenforceable here**: the desk and
field shells share their views, so `TicketFormView` and the rosters would have
to fork to carry two vocabularies. One word each, and the deciding argument is
that **a thing is not always a device** — the catalog is deliberately a superset
of the platform's, covering a door, a gate, a circuit, a vehicle. Narrowing the
noun to "device" would quietly discourage exactly the tickets that most need
filing. "Location" carries no such loss.

Two things this does **not** mean. "On-site" is a different word — it means
physically present at the customer's premises (`on-site visit`, `time on site`)
and is unrelated to the `locations` record. And `visits.location` is not a
location at all: it is free-text dispatch directions, distinct from the ticket's
`location` relation that the visit reaches through its ticket. It is labelled
**"Directions"** in the UI, which is what `VisitWorkView` had always called it
while the schedule form was still labelling it "location".

**Demo seeding** (`internal/demoseed`): `./helpdesk seed-demo --confirm [--tickets N]`
fills a showcase instance — 8 customers, type taxonomies with real
`metadata_schema`s, a location hierarchy, 40 things, and a backdated ticket
history with comments, visits, and a billable/non-billable time ledger.
Idempotent (natural keys + a fixed-seed PRNG) and self-healing per child record.

In-process Go rather than an HTTP script for one decisive reason: **PocketBase's
autodate overwrites `created` on save and ignores it on update**, so no external
client can produce a demo whose ages look real — every ticket lands minutes old
and the queue's Age column and ticket-volume report both read flat. `app.DB()`
lets the seeder rewrite the column directly. Two consequences worth knowing:
generation must spend *all* the randomness before any writing (writing is
conditional, so RNG use there would diverge between runs and defeat the dedupe
keys), and transitions must re-fetch the record between steps or every audit row
diffs against the original `open`. The subcommand relies on the record hooks
being bound outside `OnServe`, so seeded tickets get their number and audit trail
from production code paths; mail is held back by `notifications.Suppress`.

**Time tracking** (`internal/timeentries`, `internal/timers`): labor is a
`time_entries` row (minutes + `work_date` + optional `visit` tag) — the ticket
is the canonical ledger, and `GET /tickets/{id}/time-total` exposes only the
sum, gated per-customer by `show_time_to_requester` (staff get the full total;
a requester gets the **billable-only** sum). `GET /reports/time-by-ticket` is
its batch companion (`ResolveTimeScope`, same policy, same redaction) returning
`{enabled, minutes: {ticketId: N}}` for a work_date window — it exposes nothing
`time-total` doesn't, just without the round trips, and answers
`enabled: false` rather than 403 for an opted-out requester so the portal hides
its hours section instead of showing a misleading zero. Agents either log minutes
by hand or run a **start/stop timer**: one open `time_sessions` row per agent
(unique index on `staff`; `started_at` server-stamped by the create hook),
resolved into a normal `time_entries` row by `POST
/api/helpdesk/timers/{id}/stop` (rounds elapsed to 5 min unless given a
`minutes` override; `complete_visit` also flips the attached visit to
`completed`, atomically; `non_billable` marks the resulting entry). The timer
is UX only — *not* a second ledger, and minute precision is deliberately loose.
Staff drive it from the ticket Time card, the visit drawer, or the mobile-first
visit **work view** (`/staff/visits/:id/work`: Arrive → live timer → Complete).
Each entry carries a **`non_billable`** flag (migration `1820000000`): stored as
the exception so the bool's zero value means billable — no backfill, no
defaulting hook, safe for every writer. Billability lives on the *labor*, not
the ticket (one ticket mixes billable work and non-billable rework/goodwill).
Reports split billable vs. non-billable (write-off rate); the timer stop route
and every log form carry it; a per-row toggle (`BillableTag.vue`) fixes a
mis-flag in place. Deliberately **not** built: rates/dollar amounts (minutes
only — billing math stays in accounting) and a NATS `time.logged` event (the
field is envelope-ready when a PSA consumer exists).

**Outbound email + events** (`internal/notifications`, lifted from kiosk's
notifier):
DB-stored templates (`notification_templates`) rendered with
`text/template` + a small FuncMap (`formatTime`, `statusLabel`,
`pluralize`). Eight event types: `ticket.created`, `ticket.assigned`,
`ticket.commented`, `ticket.status_changed`, `visit.scheduled`,
`visit.rescheduled`, `visit.canceled`, `visit.completed`. Visit events fire on
*transitions*, not raw saves: scheduled = became scheduled (create or update),
rescheduled = time moved while scheduled, canceled = was scheduled
(canceling a bare `requested` visit is silent, as is a tech swap without a time
change). `visit.completed` (migration `1817000000`) is the one event seeded
**email-disabled and `publish_nats` enabled**: completion is already visible to
humans through the ticket's status and comments, so mail would be noise, but
"work done on site" is exactly the signal MSP-internal automation wants.
The visit's technician overrides the
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
`schema: helpdesk.event`) to `helpdesk.{customerCode}.events.{event_type}` via
an injected `Publisher` (nil until NATS connects → clean no-op, independent
of email). Token 2 is `customers.code` — the ecosystem tenant token, the same
handle the *inbound* subject carries, so both directions of the boundary name a
tenant the same way and a consumer can join helpdesk events to platform data
without a mapping table only this database could produce. It was the customer
record id until ADR 0002; a customer with no code is **skipped, not published
under a fallback** (the reason lands on the send-log row), because a token that
is sometimes a shared code and sometimes a local primary key leaves a consumer
unable to tell which it holds. The channel is for MSP-internal consumers so the
envelope is rich (no portal redaction); the human suppression rules gate email
only. `channel` (`email`|`nats`) on the send log distinguishes rows. `docs/notifications.md`
has the full event / recipient / suppression matrix; `docs/protocol.md` has
the outbound envelope + subject contract.

**NATS ingestion** (`internal/subjects`, `internal/natsx`,
`internal/ingest`): customer apps publish `helpdesk.tickets.create` in
their own org account; the platform's managed-org export (platform commit
`45ca1e3`) delivers it hub-side as `helpdesk.{orgCode}.tickets.create`.
The org id is parsed **from the subject only** (token 2) — the export's
subject rewrite is operator-signed, so it's unforgeable; a payload org id
would not be. The helpdesk owns its inbox stream `HELPDESK_EVENTS`
(subjects `helpdesk.*.tickets.>`) and a durable consumer `helpdesk-ingest`.
Projection semantics: unknown org → warn + ack (operator sets
`customers.code`, later events flow); `dedupe_key` + unique
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
`requester_email` matches only within the token's customer. Wire
contract for both intakes: `docs/protocol.md`.

**Email ingestion** (`internal/inbound/email.go` + `postmark.go`,
`internal/customers`): inbound email via an email-parsing **provider** (Postmark
first), not IMAP/SMTP — the provider receives forwarded mail, parses the MIME,
and POSTs clean JSON to `POST /api/helpdesk/inbound/email/{provider}` (registered
only when `inbound.secret` is set; Basic-auth + optional IP allowlist). The
helpdesk holds **no mailbox credentials**. A provider-agnostic core
(`NormalizedInbound` + `IngestEmail`) does all the work; the adapter (`postmark.go`)
only maps the wire format, so SES/CloudMailin is a drop-in sibling file. A reply
carrying the `[#N]` subject token (already in every notification subject) becomes
a public `ticket_comment` — the existing comment hook then auto-reopens a resolved
ticket for free; a `closed` ticket spawns a new one instead. Otherwise it's a new
ticket (`source = email`, added to the select). The sender resolves to a customer
by exact `users.email`, else `customers.email_domain` (new field, unique, never a
public provider — guarded in `internal/customers`); unresolvable senders are
acked-and-dropped (no catch-all). Idempotency rides the email `Message-ID`
(`tickets.dedupe_key` and the hidden `ticket_comments.source_message_id`, both
migration `1823000000`). DKIM is log-only in v1, and ingestion is **text-only** —
attachments are a deliberate non-goal (inline signature images would swamp the
per-record file cap). Full design: `docs/email-ingestion.md`.

**UI** (`ui/`): Vue 3 + Vite + Pinia + Tailwind + daisyUI (custom light/dark
theme + soft badges, `ui/tailwind.config.js` + `.badge-soft*` in
`src/style.css`) + Leaflet over an OpenFreeMap basemap (lazy-loaded location
map picker), PocketBase JS
SDK, same-origin (`new PocketBase('/')`). One login page tries `staff`
then falls back to `users`; router guards by auth collection
(`meta.requires`), plus `meta.adminOnly` for admin surfaces (staff,
categories, notifications).
`/t/:id` forwards to the right detail view by role (bounces through login
with a `redirect` query).

**QR labels and scanning** (ADR 0002 in `platform-docs`) close the loop between a
device on a wall and its history. `QrLabelModal.vue` prints an operator-branded
label from a thing or location detail view; `/staff/scan` (`ScanView.vue` +
`QrScanner.vue`, `html5-qrcode`) reads one back. Four decisions worth not
re-litigating:

- **The payload is the bare `code`** — no host, no customer, no kind token. A
  sticker in a public hallway is an attacker-writable surface, and a URL payload
  would let a forged label send a human to arbitrary content; a bare in-system
  identifier means the worst case is resolving a different record inside an
  already-authenticated session. It also makes *maximum* error correction free:
  `DOOR-1` at EC level H is a 21×21 symbol, where the URL form needs 41×41.
- **In-app scanning only, and no resolver service.** Nothing fetches the decoded
  string as a destination. The same component in the platform console scans the
  same labels.
- **Resolve globally, then disambiguate — never within a sticky customer
  context.** `staff` have no customer field, so a tech has no ambient tenant, and
  `DOOR-1` is exactly the code every customer independently invents. Context
  (the tech's scheduled visits) *sorts* matches and never filters them; a
  collision shows a picker rather than a confident wrong answer. Things and
  locations carry separate `(customer, code)` indexes, so one customer may
  legitimately hold both under one code — both are searched.
- **Manual entry is half the design, not a fallback.** Every label prints its
  code in human-readable text because the symbol will eventually be scratched,
  greasy, or in a closet too dark to focus in.

Labels are sized to real stock in **millimetres**, not pixels, with a per-size
`@page` box written from script (`@page` cannot be interpolated from a template
or a scoped style block): **2″ × 1″** and **4″ × 2″**, the two sizes that exist in
both plain and UHF RFID stock. Both reserve a centred **RFID inlay keep-out** —
the artwork straddles it, QR one side and text the other — so one layout prints
correctly on either. The *RFID stock* toggle only reveals that reserved band for
checking against an inlay datasheet; it never changes the layout. Quiet zone is
the spec's 4 modules, giving 0.69 mm per module at 2″ × 1″ and 1.38 mm at
4″ × 2″, both comfortably above the ~0.5 mm a phone camera needs.

A record with no `code` gets no label button: the payload *is* the code.

The **field shell** (`FieldLayout.vue`) reaches the site/device axes too, since a
tech standing at a device is who the whole label loop was built for. A phone
bottom bar holds five thumb targets and the field app has eight destinations, so
the fifth is a door rather than a place: `Today · Schedule · Tickets · Time ·
More`, with `FieldMoreView` grouping Scan / Sites / Devices under "Look up" and
Projects under "Work". Projects gave up its tab — it is the one field
destination read *between* jobs rather than during one. The desktop field
sidebar lists all of it flat and never points at `/staff/more`; the routes
themselves are the same `/staff/*` any staff member can reach, so nothing forked.
Everything behind More keeps the More tab lit, because an unlit bar three taps
into a hub reads as "nowhere".

The rosters themselves are reused rather than duplicated, with one addition:
`useVisitContext()` — the customers a staff member has *scheduled visits* with,
extracted from `ScanView`, which already computed it inline. Two rules travel
with it and are why it is one composable and not three queries. **It sorts or
optionally narrows; it never silently filters** (the scanner must still resolve
globally and disambiguate with a picker — ADR 0002 — since filtering to "the
customer I was last looking at" would confidently show a different tenant's
`DOOR-1`). And **an empty set means "no context", not "no matches"**: every
caller hides its context affordance when the set is empty, so a dispatcher or an
admin — who has no visits assigned — never meets a control that would filter
their roster to nothing. Sites and Devices spend it as an opt-in *My scheduled
sites* toggle; the scanner spends it as a sort key only.

`LocationDetailView` carries a **Devices here** card. `ThingDetailView` has
answered "every ticket for this device" since the relation existed; this is the
other direction, and it is the one asked first on site — scan the door, see
what is on it. Retired gear is listed (badged), same reasoning as the scanner:
you cannot file against it, but "what *used* to be here" is a real question.

Its page is organised as **the record, then what is here**: Details beside the
map (two cards of about equal height), Metadata full width on its own row, then
Things here beside Recent tickets here. Everything from the map down used to be
stacked inside the right-hand column of a two-column grid, so one column carried
four cards against the other's one — the record's fields ran out after a screen
and the rest of the page was a single tall stack with a column of dead space
next to it. Metadata takes the full-width row because its height is whatever the
type's `metadata_schema` declares, which is exactly the thing that makes
whichever column holds it the taller one; in read mode it is hidden entirely
when empty, rather than spending a card on "No metadata recorded".

`RosterFilters.vue` is the search + customer + type row shared by the Locations
and Things rosters, and it settles a question worth not re-opening: type filters
key on the type **name**, not its id. `thing_types`/`location_types` are
customer-scoped, so eight customers each own a "Door Controller" row; filtering
by id would need the customer picked first — a control dead until you touch
another one — and would then answer only "this customer's". Name-keying dedupes
the options instead, so picking "Door Controller" means every door controller,
which is the cross-customer question a roster filter is actually asked (types
with a shared `code` are the same upstream concept by design). The customer
picker still narrows the type list, because the options derive from the rows in
scope rather than the full taxonomy — a dependent *list*, not a dependent gate —
and a narrowing that orphans the selected type clears it. `RecordTypesView`
deliberately does NOT merge by name: there each row *is* the record you came to
edit, so it takes a plain customer filter.

Staff **Reports** scope by customer / location / thing and roll every
ticket-hung axis through one `byTicketAxis` grouper — location (volume-first),
thing and thing type (hours-first, since "which devices burn the most time" is
why things stopped being free text). The unattributed "—" bucket always sorts
last however big it is; it is usually the largest row and it is not an answer.

The page **pins the filters and totals and tabs the rollups** (the open tab and
the whole scope ride the URL via `useQuerySync`, so a report links to the exact
figures you were reading and survives a reload). The totals aren't a report,
they're the denominator every report is read against, so they never go behind a
tab; the rollups genuinely are alternatives, and stacking all seven made a long
page of which six tables were noise. A shared "group by" dropdown is the wrong
control here — these tables don't share a column set (staff has Field/Visits,
location has Tickets/Planned, category has Total/Open), so one selector would
flatten away the distinctions that are the point. All seven render through
`ReportTable.vue` (a column spec + a `#label` slot), which also owns the
proportional bar. That bar scales to the largest **attributed** value, not the
largest overall: the "—" bucket is routinely the biggest row, and scaling to it
squashed every real device into the left quarter of the track. Grids holding
side-by-side cards need `items-start` — a grid row otherwise stretches to its
tallest cell and pads the short card with dead space.

The staff **Dashboard** is written in the Reports register on purpose — same
stat scale, shadow, card padding, headings and page rhythm — because the two
are read minutes apart and six trivial divergences added up to "different app".
What does *not* transfer is colour: the tiles carry the `TicketBadges` status /
priority palette so a tile reads as the chip you will meet in the queue, while
Reports' measures (hours, counts) have no semantic scale and stay neutral.

Below the tiles it is **main + rail**, the shape the ticket detail and the
new-ticket form already use: My Active Tickets at two-thirds, and Backlog age /
Due / inflow stacked in the third. The three rail cards used to sit in a
two-column grid with the agent's own queue underneath them, which cost twice —
three cards do not divide by two, so the chart sat half-width with a hole beside
it, and the one thing on the page you *act* on started below the fold under two
cards and a chart read once a morning. The rail cards trade the nested `stats`
blocks for compact label→value rows: a third-width column has no room for the
stat register's padding, and three of them at that scale would push the queue
straight back off the fold. The tiles row is where the shared Reports register
lives, and the rail keeps the palette, so its numbers still read as the chips
they open. Due says "no target dates set" rather than rendering three zeros in
the register the counts that matter use.
Every number on the page is a door into a pre-filtered queue, Backlog age
included — three inert numbers styled exactly like five links promised a click
nothing honoured, which is why `tickets` grew an `age` filter. Its buckets are
`[0,2d) · [2d,7d) · [7d,∞)`, evaluated **per query** (a tab left open overnight
would otherwise keep measuring from whenever it loaded), and the labels say
"2–7", not "3–7": the middle bucket opens at two days, and while the count
stood alone the mislabel was invisible — beside an Age column it is the first
thing you see. Charts print their values rather than hiding them in `title`
tooltips; hover does not exist on the phone this app is also read on, so a
hover-only count is no count at all.

`SearchSelect.vue` is the typeahead behind ~40 pickers, and two of its
decisions are easy to undo by accident. Its popup is **not** tied to the input:
filter inputs sit in `sm:w-52` columns, and daisyUI's menu lays each row out as
`display: grid; grid-auto-flow: column`, so a `w-full` popup made label and
sublabel split 13rem and truncate — cutting exactly the half that
disambiguates, two of a customer's sites both reading "Brightpath…". The list
sizes to its content and stacks the two, and the explicit `flex` on the row is
load-bearing (grid ignores `flex-direction`, so `flex-col` alone is inert). It
is also a hand-rolled ARIA combobox: the highlight is announced through
`aria-activedescendant` rather than by moving focus, because focus has to stay
in the input for typing to work.

Its sub-text is dimmed with **`opacity-60`, never `text-base-content/50`**, and
that generalises to anything inside a daisyUI `.active` row (menus here, but
tabs and steps behave the same). `.active` repaints the row `--n` and sets its
text to `--nc`; a hardcoded base-content overrides that inherited colour, and in
the **light** theme — where neutral and base-content are both dark — the sub-text
disappears into the highlight entirely (measured 1.03:1, i.e. invisible; 5.95:1
after). Dark mode hides this class of bug completely, because there both colours
are light and the wrong one still reads. Dim by opacity so the colour is
inherited and follows the row's state.

The **`.badge-soft` geometry lives in `@layer components`; its colours
deliberately do not** (`src/style.css`). A bare rule written after
`@tailwind utilities` beats a utility of equal specificity on source order, so
`class="badge-soft hidden sm:inline-flex"` did *nothing*: the badge kept its own
`display: inline-flex` and stayed visible at every width. That is the worst
shape a CSS bug can take — a responsive layout gives you no sign, and the two
callers that wrote it (both "Things here" cards) simply truncated their names on
a phone for as long as it stood. The `text-xs` / `text-[10px]` a few other
callers set were inert the same way. Moving only the geometry into the
components layer lets utilities win, which is what anyone writing one expects;
the colours stay outside it because they carry the meaning of the chip and
should not fall to a stray `text-*`.

`useQuerySync` (`composables/useQuerySync.ts`) mirrors filter state into the URL
on every filtered board in the app — the three staff ones (queue, Reports,
Dispatch) and, since the portal grew its own catalog surfaces, the five portal
ones (tickets, visits, projects, things, Summary). The staff boards already
*read* their filters from the query (that is how a dashboard tile or a
"View all →" arrives pre-filtered); only the write-back was missing, so a
filtered board could not be linked, survive a reload, or come back when you
opened a ticket out of it and pressed Back. Three rules are load-bearing:
**replace, never push** (a filter adjusts the view you are on; pushing buries
the page you came from under one entry per keystroke, so Back walks you through
your own edits instead of returning you); **defaults are omitted**, so what is
absent from the query is what the page would have chosen anyway and two
identical views produce one link; and **outbound only** — nothing watches the
route and writes back into the refs. That direction reads as the natural
completion of this and is a trap, because the guard stopping ref → query → ref
has to distinguish "the user navigated" from "we just wrote this"; the views
remount on every arrival that matters, and the mount-time read covers those.

`PortalTicketsView` is the one view that also watches the route inbound, and it
predates the composable: vue-router reuses the component for
`?location=A → ?location=B`, so a deep link arriving from the *same* page would
otherwise be ignored. It does not loop, because useQuerySync's write lands in
the query first, so every value read back is the one already held and Vue only
fires a watcher on change. Two rules keep it that way and both were violated
when it was written. Re-seed **every** key the sync writes — `status` and
`awaiting` used to be watched and never assigned, so arriving from the
dashboard's "Needs reply" tile and then clicking the sidebar's Tickets left the
toggle on while the URL said otherwise. And re-seed with the **same defaults**
the mount-time read uses: `status=active` is omitted from the URL, so a bare
`q('status')` re-seeds the empty string and filters on `status = ''`, which
matches nothing.

Two deliberate exceptions to the omit-defaults rule: Reports always writes
`from`/`to`, because "the trailing 30 days" means something different the day
after you send the link, and Dispatch writes the calendar's `focus` date,
because `view=week` alone lands the recipient on *their* current week.

The **portal** reads the location/thing axes as well as writing them. Intake
offers a picker per axis (customer-scoped by the collection rules, so no client
filter is needed or trustworthy), with things at the chosen location sorted to
the top but **never hidden** — `things.location` is optional and a requester may
know the box without knowing which location it's filed under. Both `_note`
fields survive as the "not in the catalog" escape hatch, and when a customer has
neither catalog populated the pickers don't render at all: the form degrades to
exactly the two free-text inputs it was before. `/portal/tickets` gains matching
`location` / `thing` filters seeded from the query string, so the ticket
detail's location and thing link into a filtered history (deliberately including
*retired* things — you can't file against decommissioned gear, but reading its
history is the point of keeping the row). `/portal/reports` is the
customer-facing Service Summary: tickets, visits and location/thing/category
rollups over a range, all from collections the requester can already read, plus
a billable-hours column that appears only when their customer opted in. It never
names a technician, same as the portal visit and project views. Its three
rollups render through the same `ReportTable.vue` staff Reports uses, so a
count, an hours figure and the unattributed row format identically on both and
this page carries no second copy of the proportional bar. It **does not tab**
them, which is the one deliberate divergence: staff has seven rollups of which
six are noise at any moment, this has three (two conditional), and a quarterly
summary is read top to bottom and exported whole. The hours column is appended
when hours are on rather than blanked when they are off — a column of dashes
hints at a figure being withheld.

The **intake form is one column and deliberately not the staff form's main +
rail.** That rail exists because an agent files thirty tickets a day and needs
eleven triage fields adjacent to the body; a requester files two a year and has
seven, five of them optional, so a rail would give the optional half equal
weight to the one required field and make a two-minute errand look like
paperwork. What it needed was shape, not a second column: two sections (the
problem, then "Where and what"), and each catalog picker on one row with the
free-text fallback it belongs to — the relation + `_note` pairing the schema
already has, which stacking them vertically hid. A row collapses to one
full-width control when its other half isn't there (no catalog, or a thing
already picked).

The requester gets **both catalog axes as first-class surfaces** —
`/portal/locations`, `/portal/things`, and a read-only detail view for each.
Nothing on the backend moved: `locations`, `things`, `location_types` and
`thing_types` have all carried the `portalRead` rule since `1824000000`, so this
is UI over rules that already existed. `/portal/sites` was the old path for the
first of them and moved with the vocabulary; nothing outside the SPA linked to
it (notification mail deep-links `/t/{id}` only). Four things are worth not
re-deriving:

- **The location detail overturns an earlier "no detail view" call, on a reason
  that did not exist when it was made.** The argument was that a detail page
  would be the filtered ticket list with a heading, and it was true — until
  `things` became a relation. *What is installed here* is a question only this
  page can answer, and on site it is the one asked first. Same reason the staff
  `LocationDetailView` grew its "Things here" card.
- **Things needed a list at all.** The portal could already *filter* by thing
  from the intake form and the ticket list, but there was nowhere to browse the
  catalog, so "which of our gear keeps failing" — the question the relation
  replaced free text to answer — had no customer-facing surface.
- **`notes` and `metadata` are withheld from both detail views.** On a location,
  `notes` is the access notes our technicians write for each other (gate codes,
  which door, who to avoid) — our operational text about their building, not
  their record of it. On a thing it is our service text, and `metadata` is a
  mirror of upstream config a requester cannot act on. Address, contact, code,
  type and parent are facts about their own property and do show. Visits, as
  everywhere else in the portal, never name a technician.
- **Locations stays a card grid while Things is a `ResponsiveList`**, which is a
  decision rather than drift: a location catalog is bounded by physical reality
  (five, a few dozen at the top end) where Things is a forty-row catalog that
  reads better dense, and the card carries an address, the next visit and two
  count launchers — more slots than a row has at phone width. What the card no
  longer does is *grow with the schedule*: it shows the soonest visit and rolls
  the rest into Visits, which owns the full filterable history. Cards that grew
  a row per booking, in a grid missing `items-start`, were what made that page
  ragged — one location with two visits set the row height and its address-less
  neighbour was padded out to match. Every slot on the card now renders whether
  or not it has content, so two cards in a row differ only by how long their
  text is. The address slot reserves its line with a space rather than a dash: a
  sub-location legitimately has no address of its own, and a "—" under the title
  reads as a stray mark rather than as an absence.

Portal filters ride the URL through the same `useQuerySync` as the staff boards
(tickets, visits, projects, things, Summary) — see that section for the three
rules; Summary takes the same always-write-the-range exception staff Reports
does, and for a sharper version of the reason, since a customer summary is a
thing you send to somebody. Two
traps it surfaces are worth stating once, because both fail silently. A boolean
ref must be mirrored as a **computed `'1' | ''`**, never as the ref: `String(true)`
is `'true'`, which a `=== '1'` read rejects, so the flag writes a param that
cannot survive a reload — and `String(false)` is `'false'`, which the
omit-defaults rule reads as a set value and pins onto every otherwise-clean
link. And an "all" option needs an **explicit sentinel** (`status=all`), not the
empty string it reads as in the markup: empty is exactly what an omitted filter
looks like, so a `View all N tickets →` link off a location or a thing would
land on the active-only default it was written to escape.

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
- **The basemap is OpenFreeMap, and `maplibre-gl` is pinned to 5, not 6.**
  `LocationPicker` moved off raster tiles because both of its sources had to go:
  CARTO put its rasters behind an API key and is retiring them, and the light
  layer was calling `tile.openstreetmap.org` directly, which the OSMF tile usage
  policy does not allow for a product. OpenFreeMap is keyless, uncapped,
  commercial-use-permitted and self-hostable — but it publishes no raster
  endpoint, so the basemap is a MapLibre style document rendered through
  `L.maplibreGL` onto a **WebGL canvas**; the draggable pin over it is unchanged
  Leaflet. The cost is real and worth knowing: MapLibre adds ~275 kB gzipped to
  the `LocationDetailView` route chunk, which is lazy so it reaches only staff
  who open a location. The pin matters more than it looks: v5 inlines its
  tile-parsing worker, v6 resolves it as a sibling file Vite never emits once the
  library is in a hashed chunk, and **nothing throws and nothing reaches the
  console** — the style loads and its background layer paints, so water, landuse,
  roads and labels vanish together and the map reads as a flat sheet of theme
  colour rather than as a failure. v5 is also what OpenFreeMap's own quick start
  pins. Mirrors the platform and access-control. Revisit when
  `@maplibre/maplibre-gl-leaflet` documents v6 rather than merely permitting it
  in `peerDependencies`.
- **gotcha**: `.gitignore` anchors the built binary as `/helpdesk` — a bare
  `helpdesk` pattern would ignore `cmd/helpdesk/` too (this bit us once).
  Similarly `config.Load` must not `SetConfigType`, or viper matches the
  extensionless binary as a config file.
- Notification sends from hooks mean **any** app.Save on
  tickets/comments/visits fires email — tests that save those records and
  assert on mail must drain with `WaitInFlight`.

## Out of scope (deliberate)

Native SMTP/IMAP inbound (email arrives via a parsing **provider** webhook
instead — see **Email ingestion** above), request/reply NATS service, SLA
timers/escalation, knowledge base, canned responses, CSAT, ticket merge/split,
magic links, multi-MSP hosting (one helpdesk instance per MSP), calendar sync for
visits. `tickets.due_at` (`1829000000`) is **not** a softening of the SLA line:
it is an inert date with no clock, no breach state and no escalation path, and
adding any of those three is the thing that stays out of scope. Also: rates or dollar amounts anywhere (minutes only — billing math
stays in accounting), a ledger lock on invoiced time, and **live sync of
things/locations from the platform**, which is architecturally closed rather
than merely unbuilt (see **Things / types**).

`docs/plan.md` is the original v1 plan and is kept for its rationale, not as a
description of the app — the same is true of `docs/service-delivery-plan.md` and
`docs/nats-notifications-plan.md`. Each carries a status banner saying where
reality has moved past it. For current behaviour use this file plus
`docs/data-model.md`. `docs/overview.md` is the narrative counterpart to both:
the mental model, what each role does, and a tour on seeded demo data — the
place to point a newcomer, and the place to update when a concept (not a field)
changes.
