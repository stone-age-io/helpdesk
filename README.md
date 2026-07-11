# Helpdesk

Service-ticket application for the [Stone-Age.io](https://stone-age.io)
ecosystem: 816tech (the platform operator / MSP) runs it to support customer
organizations. One Go binary embedding PocketBase (system of record, REST
API, auth) and a Vue 3 SPA (staff app + requester portal).

The differentiating capability is **machine-generated tickets**: things and
rule-router publish events inside a customer org's NATS account on
`helpdesk.>`, the platform's managed-org export delivers them into the
operator hub account as `helpdesk.{platformOrgId}.>` with unforgeable
subject-based provenance, and the helpdesk's durable JetStream consumer
turns them into tickets. Humans use the portal, staff app, or the
authenticated webhook.

## Features (v1)

- **Two identity classes**: `staff` (agents/admins, cross-customer) and
  requesters (`users`, scoped to one customer). One login page; the router
  shows the right shell.
- **Ticketing core**: sequential ticket numbers, status/priority/assignee,
  comment threads with staff-only internal notes, time entries, site
  visits.
- **Customers directory**: platform-org mapping for NATS ingestion,
  per-customer webhook tokens (admin reveal/rotate).
- **Outbound email**: DB-stored templates (Go `text/template`, editable in
  the SPA) fired from record hooks — created / assigned / commented /
  status changed / visit scheduled — with per-event recipient specs, a send
  log, and day-keyed dedupe. No SMTP configured = clean no-op.
- **Inbound machine tickets**: NATS durable consumer + authenticated
  webhook (`POST /api/helpdesk/inbound/{token}`), both idempotent via
  `dedupe_key`. See [docs/protocol.md](docs/protocol.md).

## Build & run

The SPA is `//go:embed`-ed at compile time; the committed
`internal/webui/public` means a fresh checkout builds without npm — but
**rebuild and re-commit it whenever `ui/` changes**.

```bash
cd ui && npm ci                 # once
npm run build                   # vue-tsc + vite → ../internal/webui/public (commit the output)
cd .. && go build ./cmd/helpdesk
./helpdesk serve                # UI at http://127.0.0.1:8090/ · PocketBase admin at /_
```

First start seeds a bootstrap staff admin (`admin@helpdesk.local`) and
prints its password **once**. Configuration is `helpdesk.yaml` +
`HELPDESK_*` env overrides — see
[docs/configuration.md](docs/configuration.md). SMTP (outbound email) and
the application URL (ticket links in emails) are configured in the
PocketBase dashboard, not the YAML.

```bash
go test ./...
```

## Repo layout

```
cmd/helpdesk/        PB bootstrap, OnServe wiring, embedded UI, retention cron
config/              viper Config (HELPDESK_ env prefix)
migrations/          Go schema-as-code (collections, rules, seeds)
internal/
  authz/             access-rule vocabulary shared by migrations + routes
  tickets/           ticket-number assignment + field defaults (create hook)
  notifications/     notifier core, templates, lifecycle hooks, editor API
  subjects/          NATS subject grammar (helpdesk.{org}.tickets.{verb})
  natsx/             NATS connect (creds file) + inbox stream helper
  ingest/            durable consumer → ticket projection
  inbound/           webhook route + webhook-token reveal/rotate
  webui/             //go:embed all:public (committed SPA dist)
  testutil/          real-PB-against-t.TempDir() test harness
ui/                  Vue 3 + Vite + Pinia + Tailwind + daisyUI SPA
docs/                plan, wire protocol, configuration
```

## Architecture notes

- Standalone sibling app (kiosk / access-control pattern), deliberately
  **not** a platform feature: helpdesk agents never hold control-plane
  credentials, and the tenancy axes differ (platform tenant = customer org;
  helpdesk tenant = the MSP).
- Tenancy is plain collection rules — `customers` + `users.customer` +
  staff roles (`internal/authz`). No pb-tenancy.
- NATS is best-effort: the app boots and serves portal/webhook traffic
  without a broker; the durable consumer resumes where it left off.
- The org id in a machine ticket comes from the **subject** (rewritten by
  the operator-signed platform import), never the payload.
