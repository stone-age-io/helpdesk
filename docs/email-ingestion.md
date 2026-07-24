# Email ingestion (inbound)

Turning inbound email into tickets and ticket comments, via an **inbound
email-parsing provider** (Postmark to start) that receives the mail, parses the
MIME, and `POST`s clean JSON to a helpdesk webhook. This is the concrete
realization of the "future email-provider integration point" the `internal/inbound`
package was built around (`inbound.go` doc comment), and it keeps the v1
deliberate non-goal — *native SMTP inbound* — intact: **the helpdesk never speaks
IMAP/SMTP and holds no mailbox credentials.** The provider owns the mail plumbing;
the helpdesk owns a stateless HTTP handler.

Status: **implemented** (2026-07-23, `feat/email-ingestion`). Postmark adapter
first; **text-only** — attachments are a deliberate non-goal (see Out of scope).
Package layout as built: `internal/inbound/email.go` (core),
`internal/inbound/postmark.go` (adapter), `internal/customers/hooks.go` (domain
guard), migration `1823000000`.

## Why this shape

We evaluated three transports (see the design discussion the plan came out of):

1. **IMAP polling** (self-hosted `go-imap` + `enmime`). Rejected: reintroduces the
   mail-server complexity v1 deferred, and Google Workspace killed Basic auth in
   May 2025 — so it also drags in XOAUTH2 + either a domain-wide-delegation service
   account (a *domain-wide mail-impersonation* key sitting in the helpdesk — flatly
   against the "helpdesk holds no broad credentials" stance in `CLAUDE.md`) or a
   3-legged OAuth "connect mailbox" flow with refresh-token reconnect UX.
2. **Provider inbound-parse webhook** (Postmark / SES / CloudMailin). **Chosen.**
   The provider does MIME parsing, attachment extraction, spam scoring, DKIM/SPF
   verification, and quoted-reply stripping. The helpdesk change collapses to
   roughly *the existing webhook handler + a comment-create branch + a few schema
   fields*, and it holds **zero mail credentials** (only a webhook secret).
3. This is also how the reference product works: Zendesk's robust path is
   forwarding into a vendor-owned inbox, not polling a customer mailbox; its
   OAuth "Gmail connector" is explicitly the low-volume convenience tier.

**Provider-agnostic on purpose.** All logic lives in a provider-neutral core; each
provider is a thin adapter that maps its wire format to one internal struct. Postmark
is the first adapter, but nothing in the core knows it exists, so an SES-inbound or
CloudMailin adapter is a drop-in later (see [Swapping providers](#swapping-providers)).

## Pipeline

```
                         (customers keep emailing the pretty address)
  requester  ──►  support@816tech.io
                        │  Google Workspace auto-forward
                        ▼
                 MX: in.816tech.io  ──►  Postmark (receive + parse MIME)
                        │  HTTPS POST (clean JSON)
                        ▼
   POST /api/helpdesk/inbound/email/{provider}      ← thin adapter (auth + map)
                        │  NormalizedInbound
                        ▼
                    Ingest(app, msg)                ← provider-agnostic core
                        │
          ┌─────────────┴──────────────┐
          ▼                            ▼
  reply token → ticket?          no match / no token
          │                            │
   status == closed? ──► new ticket    └─► resolve customer → CreateTicket
          else                              (source = "email")
          ▼
   create ticket_comment (public)
   → existing tickets hook auto-reopens
     a resolved ticket, clears awaiting_requester
```

Nothing here needs a background worker: it is request-in, DB-write, response-out,
exactly like the existing `POST /api/helpdesk/inbound/{token}` webhook.

## Provider-agnostic core (`internal/inbound/email.go`)

### `NormalizedInbound`

The one struct every adapter produces. Deliberately post-parse and text-only —
quoted history already stripped — so the core never touches MIME or attachments.

```go
type NormalizedInbound struct {
    MessageID   string             // provider's RFC Message-ID; idempotency key
    From        Addr               // sender (email + display name)
    Subject     string
    Body        string             // best available plain text: provider's stripped
                                   // reply if present, else full text/plain
    ReplyToken  string             // ticket number parsed from the [#N] subject
                                   // token; "" if none. (A plus-hash reply address
                                   // is a documented future upgrade — see Threading.)
    Headers     map[string]string  // lower-cased; for the loop guard
    SpamScore   float64            // provider-scored; SpamFlag folds it in
    SpamFlag    bool
}
```

### `Ingest(app core.App, msg NormalizedInbound) (Result, error)`

The whole decision, testable without HTTP (mirrors how `ingest.Consumer.Project`
is tested directly with no broker). Order matters:

1. **Loop / spam guard — drop early.** Ignore (ack, log, create nothing) when any of:
   `Auto-Submitted` is `auto-replied`/`auto-generated`; `Precedence` is
   `bulk`/`list`/`junk`; `From` is empty / `mailer-daemon@` / `postmaster@`; or
   `SpamFlag`. This is non-negotiable: the helpdesk emails from a neighboring
   address, so bounces and out-of-office replies *will* arrive, and without this a
   notification→reply could loop.
2. **Idempotency.** If a ticket already has `dedupe_key == MessageID`, or a comment
   has `source_message_id == MessageID`, no-op. Providers retry on non-2xx, so this
   absorbs redelivery. (Unique indexes are the real backstop — same pattern as
   `tickets.number` / `tickets.dedupe_key`.)
3. **Threading.** If `ReplyToken` resolves to a ticket:
   - **`closed`** → do *not* comment. Mirror the portal / migration `1822000000`
     ("a closed ticket is final; open a new one"): fall through to step 4, prefixing
     the body with a `Reply to closed ticket #N` breadcrumb.
   - **otherwise** → create a `ticket_comments` row (see below).
4. **New ticket.** Resolve the customer (ladder below), normalize into the existing
   `inbound.Payload`, and call `CreateTicket`. `source = "email"`.

Returns a `Result` the adapter turns into a response (`created` / `commented` /
`duplicate` / `ignored{reason}`).

### Creating the comment (why replies are nearly free)

A reply becomes a **public** `ticket_comments` row:

| field               | value                                                              |
|---------------------|--------------------------------------------------------------------|
| `ticket`            | the resolved ticket id                                             |
| `author_user`       | user whose `email` matches `From` **within the ticket's customer** (may be empty) |
| `body`              | `msg.Body`, prefixed with a `From: name <email>` provenance line   |
| `internal`          | `false`                                                            |
| `source_message_id` | `msg.MessageID` (new hidden field, unique index)                   |

Written server-side via `app.Save` (bypasses collection rules, like the activity
trail). The payoff: the existing `OnRecordAfterCreateSuccess("ticket_comments")`
hook in [`internal/tickets/hooks.go`](../internal/tickets/hooks.go) already does the
rest — a public comment with `author_user` set runs `handleRequesterReply`, which
**reopens a `resolved` ticket** (silently, via `notifications.Suppress`, attributing
the reopen to that user with `activity.SetActor`) and clears `awaiting_requester`.
Email threading and the two-stage lifecycle compose with **zero new lifecycle code**.

Edge case — **unmatched sender on a reply**: if `From` matches no user of that
customer, still record the public comment but leave `author_user` empty (so the hook
does *not* auto-reopen — we won't let an unverified/spoofed sender silently reopen or
impersonate) and log it. Staff see it in the timeline, with the real sender in the
provenance line, and act manually.

### Customer resolution ladder (new tickets)

A single forwarded `support@` address can't identify the tenant by recipient, so:

1. `From` matches a `users.email` → that user's `customer`, and set `requester`.
2. else `From` domain matches a `customers.email_domain` (new field) → that customer,
   no requester.
3. else **reject** — ack (`200 ignored`, never 500) + log. There is no default/triage
   customer: the helpdesk is not an open funnel, it only accepts mail it can attribute
   to a known tenant.

The resulting model is deliberate:

- **Customer *with* a mapped domain** — any employee at `acme.com` can email in cold
  (rung 2, unlinked requester; staff can link later).
- **Customer *without* a domain** (solo Gmail/Outlook contact) — their people must
  exist as registered `users` (rung 1), or the mail is dropped. Correct for a B2B MSP
  that knows its contacts, but operators should know a brand-new unregistered contact
  at a domain-less customer is silently dropped (logged), not queued.

Customer scoping is preserved end-to-end (requester match is customer-scoped, exactly
like the existing webhook at `inbound.go`), so a stray email can never cross tenants.

### Threading token

- **v1:** parse `\[#(\d+)\]` from the subject. Every notification subject already
  carries `[#{{.Ticket.Number}}]`
  ([`notifications/defaults.go`](../internal/notifications/defaults.go)), and mail
  clients preserve the subject on reply (`Re: [#42] …`). No outbound change, no new
  config. The only failure mode is a user manually deleting the `[#N]` from the
  subject — rare, and it degrades safely to a new ticket.
- **Future upgrade (not v1):** a plus-addressed `Reply-To: support+{number}@…` gives
  deterministic routing via Postmark's `MailboxHash`, immune to subject edits. Left
  out because it adds a header, a config value, and a dependency on plus-tag
  preservation through forwarding — cost we don't need while the subject token works.

For a reply to reach the provider at all, the **PocketBase sender address must be the
intake mailbox** (the one forwarded to Postmark), so replies return there by default
with no `Reply-To` (see [Outbound coupling](#outbound-coupling)).

### Body

Prefer the provider's stripped-reply field (Postmark `StrippedTextReply`) so quoted
history is gone without heuristics. Fall back to `TextBody`. (If we ever use a
provider without stripping, the grug fallback is the Zendesk delimiter trick: append
`##- reply above this line -##` to outbound and cut there on inbound.)

Ingestion is **text-only** — attachments are dropped, not stored (see Out of scope
for why).

## Thin Postmark adapter (`internal/inbound/postmark.go`)

The *only* Postmark-aware code. It:

1. **Authenticates the webhook** — Basic auth on the URL (Postmark supports a
   user:pass in the webhook URL) checked against `inbound.secret`, plus an optional
   source-IP allowlist (`inbound.allowed_ips`, Postmark's published egress ranges).
   No auth → `401`.
2. **Decodes** the Postmark inbound JSON into a local struct.
3. **Maps** to `NormalizedInbound`:

   | Postmark field                       | NormalizedInbound        |
   |--------------------------------------|--------------------------|
   | `MessageID`                          | `MessageID`              |
   | `FromFull.{Email,Name}`              | `From`                   |
   | `Subject`                            | `Subject`                |
   | `StrippedTextReply` \|\| `TextBody`  | `Body`                   |
   | `Headers[]`                          | `Headers` (lower-cased)  |
   | `X-Spam-Status` / `X-Spam-Score`     | `SpamFlag` / `SpamScore` |

4. **Calls `Ingest`** and translates `Result` → HTTP.

**Response contract** (providers retry on non-2xx, so be careful):

- `200` for anything intentionally handled *or* intentionally dropped
  (`{status: created|commented|duplicate|ignored, id?, number?}`). A dropped
  loop/spam message returns `200 ignored` so the provider stops retrying.
- `401` bad/missing secret.
- `422` undecodable body.
- `500` only for a genuine transient server error (let the provider retry).

## Schema changes

One new timestamped migration (`19xxxxxxxx_email_ingestion.go`), idempotent, using
`internal/authz` constants — no collection **rule** changes (all writes are
server-side via `app.Save`, which bypasses rules):

- `tickets.source` select: add **`email`** (alongside `portal|agent|nats|webhook`).
- `customers.email_domain` — text, **optional**, **unique when set** (partial unique
  index on non-empty values, so two customers can't claim the same domain). Optional
  because a customer may be a single contact on a shared provider (e.g. a solo
  operator on `gmail.com`) with no domain of their own — such customers leave it blank
  and match only on rung 1 (exact `From` → registered user).
  - **Public-domain guard:** reject setting `email_domain` to a shared/free domain
    (`gmail.com`, `outlook.com`, `hotmail.com`, `yahoo.com`, `icloud.com`, …) — a small
    blocklist checked in a `customers` save hook. Prevents a customer from claiming
    `gmail.com` and vacuuming every Gmail sender into their tenant. Cheap, and
    tenant isolation is worth the ~10 lines.
- `ticket_comments.source_message_id` — hidden text + **unique index**
  (`idx_ticket_comments_source_msgid`, partial: non-empty only). The comment-path
  idempotency backstop, same idiom as `tickets.dedupe_key`.

## Outbound coupling

**v1 needs no outbound code change at all.** The Reply-To is *not* a PocketBase
setting — the superuser UI configures only the **From / sender address**
(`settings.Meta.SenderAddress`, [`notifier.go`](../internal/notifications/notifier.go))
and SMTP. So threading is achieved purely by operator config:

- Set the **PocketBase sender address to the intake mailbox** (`support@816tech.io`,
  the one forwarded to Postmark). Replies then return to it by default — no `Reply-To`
  header needed — and the `[#N]` already in every subject threads them.

That's the whole coupling: one operator setting, zero code, no template edits.

**Escape hatch (only if sender ≠ intake):** if an install must send *from* one
address but receive replies at another, add an optional `inbound.reply_to` config and
one line in `notifier.deliver` — `msg.Headers["Reply-To"] = cfg.Inbound.ReplyTo`.
Deferred until a deployment actually needs it.

Note for Workspace shops: PocketBase's SMTP is username/password only, so if outbound
goes *through* Gmail it must use Google's IP-authenticated relay or a transactional
provider (which is what uSend/SES already is) — not user-auth Gmail. Orthogonal to
inbound, noted for completeness.

## Config (`config/config.go`)

New block, viper defaults + `HELPDESK_*` overrides, mirroring `NATSConfig`:

```yaml
inbound:
  secret: "<webhook basic-auth password>"   # empty ⇒ email ingestion disabled
  allowed_ips: []                            # optional provider egress allowlist
  # reply_to: "support@816tech.io"           # optional escape hatch; unset ⇒ sender address is the intake mailbox
```

`InboundConfig.Enabled()` = `secret != ""`. Disabled is valid — the app serves
without email ingestion (parallels `NATSConfig.Enabled()`). Note there is
deliberately **no** `reply_domain` (subject-token threading, see Outbound coupling)
and **no** `default_customer` (unmatched senders are rejected, see the resolution
ladder).

## Wiring (`cmd/helpdesk/main.go`)

Register the route inside the existing `OnServe` block, next to the other
`inbound.Register(e)` call — the provider webhook is plain HTTP, no lifecycle
resources to tear down. `Register` takes the `InboundConfig` (or a new
`inbound.RegisterEmail(e, cfg.Inbound)`), and no-ops when disabled.

## Testing

Follows repo convention (`testutil.SetupApp(t)`, real PB against `t.TempDir()`):

- **Core:** table-driven tests calling `Ingest` directly with hand-built
  `NormalizedInbound` values — new ticket, reply→comment→**reopen** (assert the
  resolved ticket flips to open and `awaiting_requester` clears), reply to `closed`
  → new ticket, each rung of the customer ladder, loop-guard drops, MessageID
  idempotency. No HTTP, no provider.
- **Adapter:** feed captured Postmark JSON fixtures through the handler, assert the
  mapping and the response codes (incl. `401`/`422`).
- Because these writes hit `ticket_comments`, tests must `notifier.WaitInFlight`
  before asserting on mail (the comment fires `ticket.commented`).

## Operator setup (one-time, out-of-band)

1. Add an inbound subdomain (`in.816tech.io`) and point its **MX** at Postmark.
2. Create a Postmark inbound stream; set its webhook to
   `https://helpdesk.816tech.io/api/helpdesk/inbound/email/postmark` with Basic auth.
3. In Google Workspace, auto-forward `support@816tech.io` → the Postmark inbound
   address.
4. Keep SPF/DKIM aligned for the sending domain so outbound (and any provider
   verification) passes.

## Security posture

- **No mail credentials in the helpdesk** — only a webhook secret. This is the whole
  reason we're not doing IMAP/OAuth; it keeps the app within the `CLAUDE.md`
  credential-minimization line.
- Webhook authenticated (secret + optional IP allowlist); unauthenticated → `401`.
- **Spoofing / DKIM (v1 = log-only):** we record the provider's DKIM/SPF verdict but
  do not block on it. Reject-unmatched is the primary spam/abuse control — mail we
  can't attribute to a known tenant is dropped, so the open-funnel risk is already
  closed. Author matching is by `From` **within the ticket's customer**; an unmatched
  sender never gets attribution and never auto-reopens.
  - **Named residual risk:** log-only means a spoofed *known* sender (forged
    `bob@acme.com` with DKIM `fail`) is still processed — it could post a comment or
    reopen a resolved ticket as "bob." Acceptable for an internal MSP tool at v1, and
    audited via the logged verdict. **Upgrade path** if it ever matters: on a *reply*
    with DKIM `fail`, skip the auto-reopen / hold for staff review (don't hard-block).
- Tenant isolation identical to the existing webhook: all matching is
  customer-scoped, so email can't cross tenants. The `email_domain` public-domain
  guard (see Schema) closes the one way domain-mapping could have leaked across them.

## Swapping providers

The seam is `NormalizedInbound` + `Ingest`. A new provider is one file:

- **SES inbound:** SES receipt rule → S3 (raw MIME) + Lambda; the Lambda (or a small
  SES→SNS→helpdesk route) parses MIME (`enmime`) into `NormalizedInbound` and calls
  the same core. More infra to own (Lambda/S3, and *we* parse MIME again), but zero
  core changes — the reason to keep the boundary clean now.
- **CloudMailin / Mailgun / SendGrid:** each is a field-mapping adapter like Postmark's.

## Out of scope (v1)

- **Attachments — a deliberate non-goal, not merely deferred.** Inbound mail is
  dominated by *inline* images: corporate signatures embed a logo plus social icons
  as `Content-Disposition: inline` parts (referenced by `cid:` in the HTML), so a
  single email routinely carries 3–4 of them, recurring on every reply. Attaching
  Postmark's `Attachments[]` naively would bury (or, under the ≤6-per-record cap,
  crowd out) the one screenshot that matters, on the very first email. Telling a
  signature logo from a real screenshot needs fuzzy heuristics (inline/CID flags also
  catch *pasted* screenshots; size thresholds are unreliable) — not grug. Text-only is
  cleaner *and* simpler: we already ingest the plain-text part, which has no image
  references, so there is no junk and nothing to filter. Files belong on the portal,
  where the requester attaches them deliberately; staff can point an emailer there.
- Per-customer inbound addresses (would sharpen tenant routing, but requires
  distributing addresses customers won't reliably use — the shared `support@` +
  resolution ladder is the grug default).
- Plus-addressed `Reply-To` / `MailboxHash` threading (subject-token is v1; this is
  the documented upgrade).
- Outbound-as-the-customer's-own-domain sender identity.
- HTML-body rendering/storage (we store the plain-text part).
- Inbound → visit/time actions; email only creates tickets and comments.

## Resolved decisions

Settled during design (recorded so the rationale survives):

1. **`customers.email_domain`** — optional, unique when set, with a public-domain
   blocklist. (See Schema.)
2. **Unmatched sender** — rejected (`200 ignored` + log); no default/triage customer.
   (See the resolution ladder.)
3. **DKIM `fail`** — log-only for v1, with the named residual risk and upgrade path.
   (See Security posture.)
4. **Reply-To / threading** — subject `[#N]` token only; no `reply_domain`. Operator
   sets the PB sender address to the intake mailbox. (See Outbound coupling.)
