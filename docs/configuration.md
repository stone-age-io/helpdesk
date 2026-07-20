# Configuration

Two configuration surfaces, deliberately split:

1. **`helpdesk.yaml` + `HELPDESK_*` env** — infrastructure the process
   needs before PocketBase is up (data dir, NATS).
2. **PocketBase settings** (dashboard at `/_`, → Settings) — operator
   concerns stored in the database: SMTP, application URL, OAuth2.

## helpdesk.yaml

Search order: `$HELPDESK_CONFIG` (explicit path) → `./helpdesk.yaml` →
`/etc/helpdesk/helpdesk.yaml`. A missing file is fine — defaults + env
cover containerized deployments. Every key has an env override with the
`HELPDESK_` prefix and `.` → `_` (e.g. `nats.creds_file` →
`HELPDESK_NATS_CREDS_FILE`).

```yaml
# PocketBase data directory (SQLite database, uploads).
data_dir: pb_data

# Optional operator branding overlay (see "Branding overlay" below).
branding:
  dir: ""                        # host dir of theme.css / logo.svg / branding.json

# Auto-close tickets left `resolved` (untouched) this many days, via a daily
# cron. 0 disables it (tickets then close only when staff close them by hand).
# The window is the grace period in which a requester reply reopens the ticket.
auto_close_resolved_days: 7      # env HELPDESK_AUTO_CLOSE_RESOLVED_DAYS

# NATS connection to the platform operator's hub account. Leave urls empty
# to run without NATS (tickets then arrive only via portal/agent/webhook).
nats:
  urls: []                       # e.g. ["nats://hub.example.com:4222"]
  creds_file: ""                 # required when urls is set
  stream: HELPDESK_EVENTS        # helpdesk-owned inbox stream (hub account)
  durable: helpdesk-ingest       # durable consumer name; stable across restarts
  notify_stream: HELPDESK_NOTIFICATIONS  # helpdesk-owned OUTBOUND event stream
```

### Branding overlay

Point `branding.dir` (env `HELPDESK_BRANDING_DIR`) at a host directory to
override the app name, logo, and DaisyUI theme **without rebuilding**. The
helpdesk serves that directory's files under `/branding/*`; `index.html`
`<link>`s `/branding/theme.css` and the SPA fetches `/branding/branding.json`
at boot. Empty/unset = embedded defaults, and the route still serves a silent
empty `theme.css` / `{}` `branding.json` so a stock install never 404s
(path traversal is rejected).

| File | Shape | Effect |
|---|---|---|
| `branding.json` | `{ "appName": "...", "logo": "logo.svg" }` | app name (sidebar + browser tab) and logo file, served at `/branding/<logo>`. |
| `theme.css` | DaisyUI `[data-theme=light\|dark]` OKLCH var overrides | recolors the UI; loaded after the bundled CSS. Override only what you need — the rest keeps the built-in theme. |
| the logo (e.g. `logo.svg`) | an image | replaces the built-in mark; `.brand-logo-img` is a CSS hook for per-theme swaps. |

Copy [`branding.example/`](../branding.example) to the host (e.g.
`/etc/helpdesk/branding/`), add your `logo.svg`, and set `branding.dir`.

### NATS credentials

The helpdesk authenticates to the hub account with a **platform-minted
`nats_user`** scoped to `sub helpdesk.>`, exported as a `.creds` file:

1. In the platform, create a hub-account `nats_user` with subscribe
   permission on `helpdesk.>`. To also emit outbound notification events, grant
   `pub helpdesk.>` (and stream-management for `HELPDESK_NOTIFICATIONS`); the
   helpdesk is otherwise blind to the grant.
2. Export its creds file; point `nats.creds_file` at it.
3. Start the helpdesk — it creates `HELPDESK_EVENTS` (subjects
   `helpdesk.*.tickets.>`) on first serve and begins consuming. If publish is
   granted, it also creates `HELPDESK_NOTIFICATIONS` (subjects
   `helpdesk.*.events.>`) for the outbound channel; if not, that setup fails
   softly and email still sends.

Setting `nats.urls` without `nats.creds_file` is a startup error. A broker
that is down at boot is **not** an error: the app logs, serves, and the
durable consumer resumes when connectivity returns.

Per-customer mapping: set `customers.platform_org_id` (SPA → customer
detail) to the customer's platform organization id. Events for unmapped
orgs are logged and dropped (acked).

## PocketBase settings (dashboard → Settings)

- **Application URL** — used to build the ticket deep links
  (`{appURL}/t/{id}`) in notification emails. Unset/localhost means emails
  render without working links.
- **Mail settings (SMTP)** — outbound email transport, plus the sender
  name/address stamped on notifications. With SMTP unconfigured PocketBase
  falls back to `sendmail`, and without that binary sends fail — failures
  are recorded per-recipient in the send log (SPA → Notifications) and
  never affect the originating write.
- **OAuth2** — optional Microsoft/Google login for the `users` (requester)
  collection; password auth works out of the box.

## First boot

The initial migration seeds one staff admin:

```
email:    admin@helpdesk.local
password: (printed to stdout exactly once)
```

Log into the SPA with it, change the password (or create a real admin and
deactivate the bootstrap account), then create customers, staff, and
requester accounts. The PocketBase dashboard (`/_`) additionally asks for
its own superuser on first visit — that account is for schema/settings
administration, separate from staff.

## Retention

`notification_send_log` and `notification_dedupe` are pruned daily (03:15
local) at 90 days — constant `sendLogRetentionDays` in
`cmd/helpdesk/main.go`.
