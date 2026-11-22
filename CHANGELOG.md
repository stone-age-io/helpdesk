# Changelog

All notable changes to the helpdesk are recorded here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

## Versioning, and what it promises

The helpdesk is **before 1.0**, so a minor version can change any surface,
and each entry below says when one did. The sibling rule in the platform's
[Stability](https://github.com/stone-age-io/platform-docs/blob/main/docs/stability.md)
page applies here too: a client must ignore fields it does not know, because a
new field in a response, a record or a payload is an addition rather than a
break.

The surfaces worth pinning a version against, once there is more than one:

- The **inbound wire contracts** — the NATS subject `helpdesk.{orgCode}.cmd.tickets.create`
  and the webhook and email payloads, in [`docs/protocol.md`](docs/protocol.md).
- The **outbound event envelope**, `schema: helpdesk.event`, which carries its
  own `version` field and is bumped there rather than here.
- The **configuration keys** and their `HELPDESK_*` overrides, in
  [`docs/configuration.md`](docs/configuration.md).
- The **collection and field names** a sibling app or an export-then-seed
  script reads, in [`docs/data-model.md`](docs/data-model.md).

Not covered, and free to change in any release: the SPA's routes and layout,
log lines and error text, the PocketBase dashboard at `/_`, the database file
itself, and what `seed-demo` creates.

> [!NOTE]
> **Keep `## [Unreleased]` empty until it has real entries**
> `scripts/release-notes.sh` builds the GitHub release body from the section
> matching the tag, and a prerelease tag reads `## [Unreleased]`. It refuses to
> publish an empty body, which is the guard that stops a release shipping with
> nothing in it — so a placeholder line, or even a comment, turns that guard
> off and publishes the placeholder as the release notes.

## [Unreleased]

### Breaking

The NATS contracts now follow ADR 0008 in platform-docs, the ecosystem's
subject grammar `{app}.{scope}.{channel}.{rest}`. Nothing was in production, so
there is no compatibility window: a publisher on the old subject reaches no
stream.

| | Before | After |
|---|---|---|
| Publish a ticket, in a tenant account | `helpdesk.tickets.create` | `helpdesk.cmd.tickets.create` |
| The same, at the hub | `helpdesk.{orgCode}.tickets.create` | `helpdesk.{orgCode}.cmd.tickets.create` |
| `HELPDESK_EVENTS` subjects | `helpdesk.*.tickets.>` | `helpdesk.*.cmd.tickets.>` |
| Outbound events | `helpdesk.{customerCode}.events.{event_type}` | `helpdesk.{customerCode}.evt.{event_type}` |
| `HELPDESK_NOTIFICATIONS` subjects | `helpdesk.*.events.>` | `helpdesk.*.evt.>` |
| Event time in the envelope | `occurred_at` | `ts` |

- **The outbound envelope is `version: 2`**, for the `ts` rename.
- **An existing deployment needs no manual step.** At startup the helpdesk
  updates both streams' subjects and the ingest consumer's filter in place.
  A ticket command still stored under the old subject is not ingested after
  the upgrade, and ages out with the stream's 7-day limit.
- The platform's managed-org export maps the whole of `helpdesk.>`, so it does
  not change.
- **`GET /api/ready` rolls up as the platform does.** A report of `ok` and
  `skipped` checks is now `ok`, where it used to be `skipped`; only a report
  where every check skipped says `skipped`. The answer before the first probe
  carries an empty `checks` list. `ready` and the status code are unchanged.
- **The release ldflag moved** to
  `-X github.com/stone-age-io/kit/version.stamped=...`. The Dockerfile and
  goreleaser config pass it; a custom build script must change too.

### Added

- **`nats.code`**, the helpdesk's Thing code (ADR 0008). NATS service discovery
  now carries it as `code` metadata, which is how the console joins the running
  process to its Thing. While it is empty, discovery registration is skipped
  with a log line.
- **The NATS connection keeps retrying through auth errors**, so a creds file
  that creds-keeper rewrites after a Revoke is picked up at the next reconnect.

- **Releases are built rather than hand-cut.** A `v*` tag now runs goreleaser for
  cross-compiled binaries (linux, darwin, windows, freebsd; amd64 and arm64
  where it makes sense) and publishes a multi-arch container image to
  `ghcr.io/stone-age-io/helpdesk`. The release body is assembled from this file
  by `scripts/release-notes.sh` rather than from commit subjects, and the
  workflow asserts the published body is the one it built.
- **A Dockerfile**, two stages and no Node: this repo commits its SPA, so the
  bundle in the tag is the bundle that ships. `HEALTHCHECK` uses `/api/ready`.
- **CI builds the container image** on every push, so the Dockerfile is no
  longer first exercised by a tag push.

### Changed

- The version accepts a link-time stamp. Ordinary `go build` still reads
  the module version from VCS; the stamp exists because the container build
  excludes `.git` and would otherwise report `(devel)` on a tagged image.
- **Built on [stone-age-io/kit](https://github.com/stone-age-io/kit) v0.1.0.**
  The readiness engine, `/metrics` and its token, the NATS connection, service
  discovery, the version, the SPA and branding routes, and the email-visibility
  hook come from the kit. `internal/health`, `internal/natsx`,
  `internal/version` and `internal/authfix` are gone. What only the helpdesk
  knows stays here: its readiness checks (`internal/readiness`) and its two
  streams and publisher (`internal/streams`). Readiness checks now run
  concurrently, with `readiness.timeout_seconds` bounding the whole probe.


## [0.1.0] — 2026-11-14

First tagged release. Everything below is the starting state rather than a list
of changes, so it is grouped by what the application does rather than by
Added/Changed. Development history is in the commit log, which runs from
2026-07-10.

### Service desk

- **Tickets** with sequential numbers, a two-stage terminal (`resolved` is a
  grace window a requester reply reopens; `closed` is final), and a daily cron
  that promotes tickets left resolved past `auto_close_resolved_days`.
- **Two identity classes**: `staff` (cross-customer; `agent`, `admin` or
  `field`) and requesters (`users`, scoped to one customer), separated by
  collection rules rather than by application code.
- **Classification and grouping**: category, type (`reactive` | `planned`),
  project, location, thing, estimated minutes and a due date — the last being a
  plain date with no clock, no breach state and no escalation.
- **Audit trail**: every workflow-field change writes a staff-only
  `ticket_events` row, written server-side so it cannot be forged through the
  record API.
- **Attachments** on tickets and comments, visible only to callers who can see
  the owning record.

### Field work

- **Visits**: `requested → scheduled → completed | canceled`, with one enforced
  invariant (a scheduled visit has both a time and an assignee) and a
  `completed_at` stamp that survives later edits.
- **Dispatch**: a needs-scheduling bucket plus a day-grouped, filterable
  calendar.
- **A mobile field shell** with its own bottom-bar navigation, and a visit work
  view running Arrive → live timer → Complete.
- **Projects and locations**: a planning layer above the ticket ledger. A
  project's crew and total time are derived at read time, never stored.
- **Preventive maintenance**: recurring plans whose only output is an ordinary
  ticket, anchored either to the schedule or to the last completion.

### Time tracking

- **Time entries** in minutes, with an optional visit tag and a `non_billable`
  flag stored as the exception so the zero value is billable.
- **Start/stop timers**, one open session per agent, enforced by a unique index
  rather than by the stop route.
- **Per-customer disclosure**: `show_time_to_requester` gates what a requester
  sees, and when it is on they see the billable sum only.
- Deliberately absent: rates, dollar amounts, and any ledger lock on invoiced
  time.

### Intake

- **NATS ingestion** from `helpdesk.{orgCode}.cmd.tickets.create`, with the tenant
  read from the operator-signed subject and never from the payload.
- **An authenticated webhook**, and **inbound email** through a parsing provider
  (Postmark), which means the helpdesk holds no mailbox credentials. A reply
  carrying `[#N]` becomes a comment, but only from someone belonging to that
  ticket's customer.
- **Idempotency** on `dedupe_key`, scoped per customer, released when a machine
  ticket resolves so a recurring alarm opens a fresh ticket rather than being
  acked away forever.

### Outbound

- **Eight email event types** from database-stored templates, with a recipient
  spec per template and two independent suppression paths.
- **A second NATS channel**: the same events publish a fixed, versioned envelope
  to `helpdesk.{customerCode}.evt.{event_type}` when a template opts in.
- **Service discovery**: the binary registers as NATS micro service `helpdesk`,
  identity only, with no endpoints.

### Interfaces

- **A staff app, a requester portal and a field shell**, one Vue 3 SPA embedded
  in the binary so a fresh checkout builds without npm.
- **QR labels and scanning**: the payload is the bare code, resolved globally
  and then disambiguated, with manual entry as half the design rather than a
  fallback.
- **Reports** for staff and a customer-facing service summary, both scoped by
  customer, location and thing.
- **An operator branding overlay** — app name, logo and theme without a rebuild
  — and an installable PWA with a deliberately no-op service worker.

### Operations

- **One binary** embedding PocketBase v0.41.1, configured by `helpdesk.yaml`
  plus `HELPDESK_*` overrides. Note PocketBase moved to `ganigeorgiev.com/`
  module paths; the `github.com/pocketbase/` ones stop at v0.40.6.
- **Schema as Go migrations**, idempotent and applied on start.
- **`seed-demo`** fills a showcase instance with a backdated history that reads
  as real, which an external script cannot produce.
- **`version`** reports the build the toolchain stamped.

### Observability

- **`GET /api/ready`**, the shape the rest of the ecosystem uses: eight checks
  for the things that go silently wrong (no SMTP, a default application URL, a
  NATS connection that never came up), rolled up by a background prober so the
  endpoint never waits on a dependency. `503` only on a failure; a warning is
  `200`, because a probe can only restart or evict the process.
- **`GET /metrics`**, Prometheus text format: the readiness report as a state
  set, build info, row counts, tickets by status and the database size. No
  series carries a customer label. Optional static token, since scrapers cannot
  refresh a PocketBase JWT.

### Known gaps at 0.1.0

- **Things and locations do not sync from the platform**, and architecturally
  cannot: there is no event stream for them, and the alternatives are a
  control-plane credential this application must never hold or an edge KV
  mirror. The catalog is hand-curated and joins upstream by code.
- **DKIM on inbound email is log-only**, and email ingestion is text-only by
  design.
