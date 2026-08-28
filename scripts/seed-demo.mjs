#!/usr/bin/env node
/**
 * Demo data seeder for a helpdesk instance.
 *
 * Populates a running helpdesk with a coherent slice of demo data: 6 customers
 * across all five ticket statuses, all five sources, both ticket types, plus
 * projects, visits, a time ledger, a populated audit timeline, and the things /
 * types / metadata layer added in migration 1824000000.
 *
 * IDEMPOTENT — every record is found-or-created by a natural key, so re-running
 * tops up rather than duplicating. Tickets are keyed on `dedupe_key`
 * ("seed-<key>"), which is the same uniqueness the NATS and webhook intakes use;
 * a ticket's children (comments, visits, time) are only created when the ticket
 * itself was new, so re-runs don't stack replies.
 *
 * Zero dependencies: plain `fetch` on Node 18+. Deliberately NOT using the
 * PocketBase JS SDK — that lives in ui/node_modules, and a demo script that
 * needs an npm install in a sibling directory is a script nobody runs.
 *
 *   node scripts/seed-demo.mjs --url http://localhost:8090 \
 *       --email admin@helpdesk.local --password "…"
 *   node scripts/seed-demo.mjs --dry-run          # report, change nothing
 *
 * Env fallbacks: HELPDESK_URL, HELPDESK_ADMIN_EMAIL, HELPDESK_ADMIN_PASSWORD.
 *
 * Every demo login shares the password `demo12345`.
 *
 * Lifecycle transitions are driven with `X-Helpdesk-Quiet: 1` so the audit
 * timeline fills in without firing a burst of demo email at real addresses.
 * All demo domains are `.example`, which is reserved and undeliverable — but the
 * header means we don't rely on that.
 */

const DEMO_PASSWORD = 'demo12345'

// ---------------------------------------------------------------- args / config

function arg(name, fallbackEnv, def) {
  const i = process.argv.indexOf(`--${name}`)
  if (i !== -1 && process.argv[i + 1]) return process.argv[i + 1]
  return process.env[fallbackEnv] || def
}

const BASE = (arg('url', 'HELPDESK_URL', 'http://localhost:8090')).replace(/\/+$/, '')
const ADMIN_EMAIL = arg('email', 'HELPDESK_ADMIN_EMAIL', 'admin@helpdesk.local')
const ADMIN_PASSWORD = arg('password', 'HELPDESK_ADMIN_PASSWORD', '')
const DRY = process.argv.includes('--dry-run')

const stats = { created: 0, matched: 0, skipped: 0 }

// ---------------------------------------------------------------- tiny PB client

let token = ''

/**
 * `auth` overrides the admin token for one call. Needed because several create
 * rules pin authorship to the caller — ticket_comments requires
 * `author_staff = @request.auth.id` and time_entries requires
 * `staff = @request.auth.id`. So the seeder authenticates AS each author rather
 * than forging rows under one admin, which means the demo data lands through
 * exactly the rules a real user hits.
 */
async function api(path, { method = 'GET', body, headers = {}, auth } = {}) {
  const bearer = auth !== undefined ? auth : token
  const res = await fetch(`${BASE}${path}`, {
    method,
    headers: {
      'Content-Type': 'application/json',
      ...(bearer ? { Authorization: bearer } : {}),
      ...headers,
    },
    body: body ? JSON.stringify(body) : undefined,
  })
  const text = await res.text()
  let json
  try {
    json = text ? JSON.parse(text) : {}
  } catch {
    json = { raw: text }
  }
  if (!res.ok) {
    const err = new Error(`${method} ${path} → ${res.status}: ${JSON.stringify(json).slice(0, 400)}`)
    err.status = res.status
    err.data = json
    throw err
  }
  return json
}

async function login() {
  // Staff first (the normal case); fall back to the PB superuser so a fresh
  // instance can be seeded with the bootstrap credentials printed at first boot.
  try {
    const r = await api('/api/collections/staff/auth-with-password', {
      method: 'POST',
      body: { identity: ADMIN_EMAIL, password: ADMIN_PASSWORD },
    })
    token = r.token
    return `staff:${ADMIN_EMAIL}`
  } catch (staffErr) {
    try {
      const r = await api('/api/collections/_superusers/auth-with-password', {
        method: 'POST',
        body: { identity: ADMIN_EMAIL, password: ADMIN_PASSWORD },
      })
      token = r.token
      return `superuser:${ADMIN_EMAIL}`
    } catch {
      throw staffErr
    }
  }
}

// Cached per-identity tokens for the authorship-pinned creates above. Every
// demo account shares DEMO_PASSWORD, so this needs no extra configuration.
const tokenCache = new Map()
async function tokenFor(collection, email) {
  const cacheKey = `${collection}:${email}`
  if (tokenCache.has(cacheKey)) return tokenCache.get(cacheKey)
  const r = await api(`/api/collections/${collection}/auth-with-password`, {
    method: 'POST',
    auth: '', // an existing Authorization header would be ignored, but keep it clean
    body: { identity: email, password: DEMO_PASSWORD },
  })
  tokenCache.set(cacheKey, r.token)
  return r.token
}

const esc = (v) => String(v).replace(/'/g, "\\'")

async function findFirst(collection, filter) {
  const r = await api(
    `/api/collections/${collection}/records?perPage=1&filter=${encodeURIComponent(filter)}`,
  )
  return r.items?.[0] || null
}

/**
 * Found-or-create by natural key. `filter` must identify the record uniquely;
 * `data` is only sent when it doesn't exist. Returns { rec, created }.
 */
async function ensure(collection, filter, data, label) {
  const existing = await findFirst(collection, filter)
  if (existing) {
    stats.matched++
    return { rec: existing, created: false }
  }
  if (DRY) {
    stats.created++
    console.log(`  would create ${collection}: ${label}`)
    // A synthetic id keeps downstream references resolvable in a dry run.
    return { rec: { id: `dry-${collection}-${label}` }, created: true }
  }
  const rec = await api(`/api/collections/${collection}/records`, { method: 'POST', body: data })
  stats.created++
  console.log(`  + ${collection}: ${label}`)
  return { rec, created: true }
}

// Quiet update: drives a lifecycle transition so the audit trail records it,
// without emailing anyone about demo data.
async function quietUpdate(collection, id, data) {
  if (DRY) return
  await api(`/api/collections/${collection}/records/${id}`, {
    method: 'PATCH',
    body: data,
    headers: { 'X-Helpdesk-Quiet': '1' },
  })
}

// Dates relative to now, so a re-seeded demo never looks stale.
const day = 24 * 60 * 60 * 1000
function daysFromNow(n, hour = 14) {
  const d = new Date(Date.now() + n * day)
  d.setHours(hour, 0, 0, 0)
  return d.toISOString().replace('T', ' ').slice(0, 19) + 'Z'
}
const dateOnly = (n) => new Date(Date.now() + n * day).toISOString().slice(0, 10)

// ---------------------------------------------------------------- demo data

const CUSTOMERS = [
  { key: 'northwind', name: 'Northwind Traders', domain: 'northwind.example', showTime: true, org: 'org_northwind' },
  { key: 'harborview', name: 'Harborview Medical', domain: 'harborview.example', showTime: false },
  { key: 'lakeside', name: 'Lakeside Schools', domain: 'lakeside.example', showTime: true },
  { key: 'ironbridge', name: 'Ironbridge Manufacturing', domain: 'ironbridge.example', showTime: false, org: 'org_ironbridge' },
  { key: 'cedarpoint', name: 'Cedar Point Logistics', domain: 'cedarpoint.example', showTime: true },
  { key: 'summit', name: 'Summit Property Group', domain: 'summit.example', showTime: false },
]

// Location types mirror the platform's location_types. The schema is what gives
// a site's metadata typed fields instead of free-form key/value rows.
const LOCATION_TYPES = [
  {
    customer: 'northwind', code: 'warehouse', name: 'Warehouse',
    description: 'Distribution and storage facility.',
    schema: {
      type: 'object',
      properties: {
        dock_doors: { type: 'integer', title: 'Dock doors' },
        sqft: { type: 'integer', title: 'Square feet' },
        after_hours_access: { type: 'boolean', title: 'After-hours access' },
      },
    },
  },
  { customer: 'northwind', code: 'office', name: 'Office' },
  {
    customer: 'ironbridge', code: 'plant', name: 'Plant',
    description: 'Production facility.',
    schema: {
      type: 'object',
      properties: {
        shift_pattern: { type: 'string', title: 'Shift pattern', enum: ['1x8', '2x8', '3x8'] },
        hazmat: { type: 'boolean', title: 'Hazmat on site' },
      },
    },
  },
  { customer: 'ironbridge', code: 'building', name: 'Building' },
  { customer: 'harborview', code: 'clinic', name: 'Clinic' },
  { customer: 'lakeside', code: 'campus', name: 'Campus' },
  { customer: 'lakeside', code: 'building', name: 'Building' },
]

// Thing types carry the metadata schema for a class of device. Codes match the
// platform's thing_types.code — that string is the join key and also a NATS
// subject segment upstream, so keep it stable.
const THING_TYPES = [
  {
    customer: 'northwind', code: 'door-controller', name: 'Door Controller',
    description: 'Access-control panel driving one or more readers.',
    schema: {
      type: 'object',
      properties: {
        serial: { type: 'string', title: 'Serial number' },
        firmware: { type: 'string', title: 'Firmware version' },
        reader_count: { type: 'integer', title: 'Readers attached' },
        poe: { type: 'boolean', title: 'PoE powered' },
      },
      required: ['serial'],
    },
  },
  {
    customer: 'northwind', code: 'kiosk', name: 'Kiosk',
    schema: {
      type: 'object',
      properties: {
        serial: { type: 'string', title: 'Serial number' },
        screen_size: { type: 'string', title: 'Screen size' },
        last_imaged: { type: 'string', format: 'date', title: 'Last imaged' },
      },
    },
  },
  { customer: 'northwind', code: 'switch', name: 'Network Switch' },
  {
    customer: 'ironbridge', code: 'plc', name: 'PLC',
    description: 'Programmable logic controller on the plant floor.',
    schema: {
      type: 'object',
      properties: {
        serial: { type: 'string', title: 'Serial number' },
        rack_position: { type: 'string', title: 'Rack position' },
        firmware: { type: 'string', title: 'Firmware version' },
      },
      required: ['serial'],
    },
  },
  {
    customer: 'ironbridge', code: 'sensor', name: 'Sensor',
    schema: {
      type: 'object',
      properties: {
        serial: { type: 'string', title: 'Serial number' },
        measures: { type: 'string', title: 'Measures', enum: ['vibration', 'temperature', 'current', 'pressure'] },
        calibrated_on: { type: 'string', format: 'date', title: 'Calibrated on' },
      },
    },
  },
  { customer: 'harborview', code: 'badge-reader', name: 'Badge Reader' },
  { customer: 'lakeside', code: 'projector', name: 'Projector' },
  { customer: 'lakeside', code: 'ap', name: 'Wireless AP' },
]

// `parent` is given as another location's code — resolved after the first pass,
// exactly the two-pass id↔code dance a platform export→seed would need.
const LOCATIONS = [
  {
    customer: 'northwind', code: 'NW-HQ', name: 'Northwind HQ', type: 'office',
    address: '1400 Commerce Way, Columbus, OH 43215',
    contact: 'Regina Holt', contact_phone: '614-555-0142',
    notes: 'Reception badges visitors in. Dock is around the north side.',
    lat: 39.9612, lng: -82.9988,
  },
  {
    customer: 'northwind', code: 'NW-HQ-2', name: 'HQ — Second Floor', parent: 'NW-HQ', type: 'office',
    notes: 'Stairwell B is the fastest way up with a cart.',
  },
  {
    customer: 'northwind', code: 'NW-DC1', name: 'Grove City DC', type: 'warehouse',
    address: '3900 Southwest Blvd, Grove City, OH 43123',
    contact: 'Marcus Webb', contact_phone: '614-555-0177',
    notes: 'Gate code 4417. Ask for the shift lead at the guard shack.',
    lat: 39.8815, lng: -83.0929,
    metadata: { dock_doors: 24, sqft: 180000, after_hours_access: true },
  },
  {
    customer: 'ironbridge', code: 'IB-PLANT1', name: 'Ironbridge Plant 1', type: 'plant',
    address: '77 Foundry Rd, Youngstown, OH 44502',
    contact: 'Dale Ferris', contact_phone: '330-555-0110',
    notes: 'Hearing protection and hi-vis required past the blue line.',
    lat: 41.0998, lng: -80.6495,
    metadata: { shift_pattern: '3x8', hazmat: true },
  },
  {
    customer: 'ironbridge', code: 'IB-PLANT1-L3', name: 'Plant 1 — Line 3', parent: 'IB-PLANT1', type: 'building',
    notes: 'Line 3 is the far bay. Lockout/tagout board is by the entrance.',
  },
  {
    customer: 'harborview', code: 'HV-MAIN', name: 'Harborview Main Campus', type: 'clinic',
    address: '620 Marina Dr, Cleveland, OH 44114',
    contact: 'Dr. Anita Rao', contact_phone: '216-555-0163',
    notes: 'Badge in at the service entrance. No work in patient areas before 09:00.',
    lat: 41.5074, lng: -81.6944,
  },
  {
    customer: 'lakeside', code: 'LS-CENTRAL', name: 'Lakeside Central Campus', type: 'campus',
    address: '15 Academy St, Sandusky, OH 44870',
    contact: 'Bev Tanaka', contact_phone: '419-555-0188',
    lat: 41.4489, lng: -82.7080,
  },
  {
    customer: 'lakeside', code: 'LS-CENTRAL-HS', name: 'Central High School', parent: 'LS-CENTRAL', type: 'building',
    notes: 'Summer hours only in July. Check in at the main office.',
  },
  {
    customer: 'cedarpoint', code: 'CP-YARD', name: 'Cedar Point Yard',
    address: '900 Rail St, Toledo, OH 43604',
    contact: 'Owen Pratt', contact_phone: '419-555-0121',
    lat: 41.6528, lng: -83.5379,
  },
  {
    customer: 'summit', code: 'SU-TOWER', name: 'Summit Tower',
    address: '200 High St, Akron, OH 44308',
    contact: 'Lena Poole', contact_phone: '330-555-0134',
    notes: 'Freight elevator needs a fob from building security.',
    lat: 41.0814, lng: -81.5190,
  },
]

const THINGS = [
  {
    customer: 'northwind', code: 'RDR-01', name: 'North Door Reader',
    type: 'door-controller', location: 'NW-HQ',
    notes: 'Main visitor entrance. Fails to the locked state.',
    metadata: { serial: 'SN-DC-9931', firmware: '2.4.1', reader_count: 2, poe: true },
  },
  {
    customer: 'northwind', code: 'RDR-02', name: 'Dock Reader',
    type: 'door-controller', location: 'NW-DC1',
    metadata: { serial: 'SN-DC-9942', firmware: '2.3.8', reader_count: 1, poe: true },
  },
  {
    customer: 'northwind', code: 'KSK-LOBBY', name: 'Lobby Check-in Kiosk',
    type: 'kiosk', location: 'NW-HQ',
    metadata: { serial: 'SN-KSK-2201', screen_size: '21.5in', last_imaged: dateOnly(-120) },
  },
  {
    customer: 'northwind', code: 'SW-CORE-1', name: 'HQ Core Switch',
    type: 'switch', location: 'NW-HQ-2',
    notes: 'Second-floor IDF, rack 2.',
  },
  {
    customer: 'northwind', code: 'KSK-OLD', name: 'Retired Break Room Kiosk',
    type: 'kiosk', location: 'NW-HQ', retired: true,
    notes: 'Decommissioned; kept for ticket history.',
    metadata: { serial: 'SN-KSK-1104' },
  },
  {
    customer: 'ironbridge', code: 'PUMP-7', name: 'Line 3 Feed Pump Controller',
    type: 'plc', location: 'IB-PLANT1-L3',
    notes: 'Drives the feed pump on line 3. Overcurrent trips page the MSP.',
    metadata: { serial: 'SN-PLC-7007', rack_position: 'R2-S4', firmware: '5.1.0' },
  },
  {
    customer: 'ironbridge', code: 'VIB-3A', name: 'Line 3 Vibration Sensor A',
    type: 'sensor', location: 'IB-PLANT1-L3',
    metadata: { serial: 'SN-VIB-3311', measures: 'vibration', calibrated_on: dateOnly(-200) },
  },
  {
    customer: 'ironbridge', code: 'TEMP-1', name: 'Plant 1 Ambient Temp Sensor',
    type: 'sensor', location: 'IB-PLANT1',
    metadata: { serial: 'SN-TMP-4410', measures: 'temperature' },
  },
  {
    customer: 'harborview', code: 'HV-RDR-ER', name: 'ER Entrance Badge Reader',
    type: 'badge-reader', location: 'HV-MAIN',
    notes: 'Life-safety adjacent — never take offline without clinical sign-off.',
  },
  {
    customer: 'lakeside', code: 'PRJ-HS-204', name: 'Room 204 Projector',
    type: 'projector', location: 'LS-CENTRAL-HS',
  },
  {
    customer: 'lakeside', code: 'AP-HS-2F', name: 'HS Second Floor AP',
    type: 'ap', location: 'LS-CENTRAL-HS',
  },
  // No code: gear the platform never onboarded. This is the case that makes the
  // catalog a superset rather than a copy, so the demo should show one.
  {
    customer: 'summit', name: 'Lobby Reception Printer',
    location: 'SU-TOWER',
    notes: 'Customer-owned, never onboarded to the platform. No code, by design.',
  },
]

const STAFF = [
  { key: 'maya', email: 'maya@816tech.example', name: 'Maya Alvarez', role: 'admin' },
  { key: 'diego', email: 'diego@816tech.example', name: 'Diego Santos', role: 'agent' },
  { key: 'priya', email: 'priya@816tech.example', name: 'Priya Nair', role: 'agent' },
  { key: 'sam', email: 'sam@816tech.example', name: 'Sam Okafor', role: 'field' },
  { key: 'tomas', email: 'tomas@816tech.example', name: 'Tomas Brandt', role: 'field' },
]

const REQUESTERS = [
  { key: 'regina', email: 'regina.holt@northwind.example', name: 'Regina Holt', customer: 'northwind', phone: '614-555-0142' },
  { key: 'marcus', email: 'marcus.webb@northwind.example', name: 'Marcus Webb', customer: 'northwind', phone: '614-555-0177' },
  { key: 'anita', email: 'anita.rao@harborview.example', name: 'Anita Rao', customer: 'harborview', phone: '216-555-0163' },
  { key: 'joel', email: 'joel.mercer@harborview.example', name: 'Joel Mercer', customer: 'harborview' },
  { key: 'bev', email: 'bev.tanaka@lakeside.example', name: 'Bev Tanaka', customer: 'lakeside', phone: '419-555-0188' },
  { key: 'curtis', email: 'curtis.yang@lakeside.example', name: 'Curtis Yang', customer: 'lakeside' },
  { key: 'dale', email: 'dale.ferris@ironbridge.example', name: 'Dale Ferris', customer: 'ironbridge', phone: '330-555-0110' },
  { key: 'nadia', email: 'nadia.brooks@ironbridge.example', name: 'Nadia Brooks', customer: 'ironbridge' },
  { key: 'owen', email: 'owen.pratt@cedarpoint.example', name: 'Owen Pratt', customer: 'cedarpoint', phone: '419-555-0121' },
  { key: 'tessa', email: 'tessa.lund@cedarpoint.example', name: 'Tessa Lund', customer: 'cedarpoint' },
  { key: 'lena', email: 'lena.poole@summit.example', name: 'Lena Poole', customer: 'summit', phone: '330-555-0134' },
  { key: 'raul', email: 'raul.ibarra@summit.example', name: 'Raul Ibarra', customer: 'summit' },
]

const PROJECTS = [
  {
    key: 'nw-dc-access', customer: 'northwind', location: 'NW-DC1',
    title: 'Grove City DC access-control rollout',
    description: 'Readers, controllers, and door hardware for the new DC. Three trades, staged over four weeks.',
    status: 'active', lead: 'maya', start: dateOnly(-21), target: dateOnly(14),
  },
  {
    key: 'ls-summer-refresh', customer: 'lakeside', location: 'LS-CENTRAL-HS',
    title: 'Central High summer AP refresh',
    description: 'Replace 40 aging access points across the high school before term starts.',
    status: 'planned', lead: 'priya', start: dateOnly(30), target: dateOnly(75),
  },
  {
    key: 'ib-line3', customer: 'ironbridge', location: 'IB-PLANT1-L3',
    title: 'Line 3 controls modernization',
    description: 'PLC and sensor replacement on line 3 during the scheduled shutdown.',
    status: 'completed', lead: 'maya', start: dateOnly(-90), target: dateOnly(-40),
  },
]

/**
 * Tickets. `key` becomes dedupe_key "seed-<key>" — the idempotency handle.
 * `transitions` are applied as quiet PATCHes after creation so the audit
 * timeline shows a real progression rather than a single created event.
 */
const TICKETS = [
  {
    key: 'nw-reader-offline', customer: 'northwind', requester: 'regina',
    title: 'North door reader is offline',
    body: 'Staff badged in fine yesterday. This morning the reader has no lights and the door stays locked.',
    priority: 'high', source: 'portal', category: 'access-control', type: 'issue',
    location: 'NW-HQ', thing: 'RDR-01', assignee: 'diego', estimated_minutes: 60,
    transitions: [{ status: 'in_progress' }],
    comments: [
      { author: 'diego', body: 'On my way — bringing a spare PoE injector in case it is the drop.', requests_reply: false },
      { author: 'diego', body: 'Panel LED is dark. Suspect the switch port, not the reader.', internal: true },
      { author: 'regina', body: 'Thanks — side door is working so we are not blocked.' },
    ],
    visits: [{ status: 'scheduled', at: daysFromNow(1, 9), assignee: 'sam', notes: 'Bring PoE injector and a patch cable.' }],
    time: [{ staff: 'diego', minutes: 25, days: 0, note: 'Remote triage, checked controller reachability.' }],
  },
  {
    key: 'ib-pump-overcurrent', customer: 'ironbridge',
    title: 'Pump fault on line 3',
    body: 'Vibration sensor reporting overcurrent on the feed pump. Auto-filed by the plant controller.',
    priority: 'urgent', source: 'nats', category: 'iot-device', type: 'issue',
    location: 'IB-PLANT1-L3', thing: 'PUMP-7', assignee: 'maya', estimated_minutes: 180,
    origin_subject: 'helpdesk.org_ironbridge.tickets.create',
    transitions: [{ status: 'in_progress' }, { status: 'waiting' }],
    comments: [
      { author: 'maya', body: 'Pulled the trend — current spikes correlate with the 06:00 startup ramp.', internal: true },
      { author: 'maya', body: 'We have escalated to the pump vendor and are waiting on their engineer.' },
    ],
    time: [
      { staff: 'maya', minutes: 95, days: -1, note: 'Trend analysis and vendor escalation.' },
      { staff: 'tomas', minutes: 40, days: -1, note: 'On-site inspection of the drive cabinet.' },
    ],
  },
  {
    key: 'hv-badge-intermittent', customer: 'harborview', requester: 'anita',
    title: 'ER entrance badge reader intermittent',
    body: 'Roughly one in five badges needs a second tap. Started after last week’s power work.',
    priority: 'high', source: 'portal', category: 'access-control', type: 'issue',
    location: 'HV-MAIN', thing: 'HV-RDR-ER', assignee: 'priya',
    transitions: [{ status: 'in_progress' }, { status: 'resolved' }],
    comments: [
      { author: 'priya', body: 'Reseated the reader harness and reflashed the panel. Please keep an eye on it today.', requests_reply: true },
      { author: 'anita', body: 'No failed taps since this morning. Thank you.' },
    ],
    visits: [{ status: 'completed', at: daysFromNow(-2, 8), assignee: 'sam', notes: 'Service entrance, ask for Anita.', completedDays: -2 }],
    time: [{ staff: 'priya', minutes: 75, days: -2, note: 'On-site diagnosis and harness reseat.' }],
  },
  {
    key: 'nw-dc-install-doors', customer: 'northwind',
    title: 'Install door hardware — DC dock doors 1-6',
    body: 'Hang and wire controllers for the first six dock doors.',
    priority: 'normal', source: 'agent', category: 'access-control', type: 'install',
    location: 'NW-DC1', thing: 'RDR-02', project: 'nw-dc-access', assignee: 'sam',
    estimated_minutes: 960,
    transitions: [{ status: 'in_progress' }],
    visits: [
      { status: 'completed', at: daysFromNow(-7, 7), assignee: 'sam', notes: 'Gate code 4417.', completedDays: -7 },
      { status: 'scheduled', at: daysFromNow(3, 7), assignee: 'tomas', notes: 'Doors 4-6, bring the long ladder.' },
    ],
    time: [
      { staff: 'sam', minutes: 480, days: -7, note: 'Doors 1-3 hung and wired.' },
      { staff: 'sam', minutes: 120, days: -6, note: 'Rework on door 2 latch alignment.', non_billable: true },
    ],
  },
  {
    key: 'nw-dc-install-network', customer: 'northwind',
    title: 'Install network drops — DC controller closet',
    body: 'Twelve drops from the closet to the dock controllers.',
    priority: 'normal', source: 'agent', category: 'network', type: 'install',
    location: 'NW-DC1', project: 'nw-dc-access', assignee: 'tomas', estimated_minutes: 600,
    visits: [{ status: 'requested', assignee: null, notes: 'Needs scheduling once the doors are hung.' }],
  },
  {
    key: 'ls-projector-dim', customer: 'lakeside', requester: 'bev',
    title: 'Room 204 projector very dim',
    body: 'Image is washed out even with the blinds closed. Probably the lamp.',
    priority: 'low', source: 'portal', category: 'hardware', type: 'issue',
    location: 'LS-CENTRAL-HS', thing: 'PRJ-HS-204',
    transitions: [{ status: 'in_progress' }, { status: 'resolved' }, { status: 'closed' }],
    comments: [
      { author: 'priya', body: 'Lamp was at 96% of rated hours. Replaced and recalibrated.' },
      { author: 'bev', body: 'Looks great, thanks!' },
    ],
    time: [{ staff: 'priya', minutes: 45, days: -12, note: 'Lamp replacement.' }],
  },
  {
    key: 'cp-yard-camera', customer: 'cedarpoint', requester: 'owen',
    title: 'Yard camera feed dropping overnight',
    body: 'The northeast yard camera goes offline around 02:00 and comes back by 06:00.',
    priority: 'normal', source: 'webhook', category: 'network', type: 'issue',
    location: 'CP-YARD', assignee: 'diego',
    thing_note: 'NE yard camera (pole 4)',
    comments: [{ author: 'diego', body: 'Could you confirm whether the pole lighting is on a timer? The window matches a power cycle.', requests_reply: true }],
  },
  {
    key: 'su-printer-jam', customer: 'summit', requester: 'lena',
    title: 'Reception printer jamming constantly',
    body: 'Jams every few pages on the lower tray.',
    priority: 'low', source: 'email', category: 'hardware', type: 'issue',
    location: 'SU-TOWER', thing: 'Lobby Reception Printer',
    comments: [{ author: 'lena', body: 'Still happening this morning.' }],
  },
  {
    key: 'ib-sensor-calibration', customer: 'ironbridge', requester: 'dale',
    title: 'Vibration sensor needs recalibration',
    body: 'Readings drifted after the shutdown. Scheduling a calibration pass.',
    priority: 'normal', source: 'agent', category: 'iot-device', type: 'issue',
    location: 'IB-PLANT1-L3', thing: 'VIB-3A', project: 'ib-line3', assignee: 'tomas',
    transitions: [{ status: 'in_progress' }, { status: 'resolved' }, { status: 'closed' }],
    time: [{ staff: 'tomas', minutes: 150, days: -45, note: 'Calibration and verification run.' }],
  },
  {
    key: 'nw-kiosk-slow', customer: 'northwind', requester: 'marcus',
    title: 'Lobby kiosk slow to load check-in',
    body: 'Takes 30+ seconds to show the check-in screen in the mornings.',
    priority: 'low', source: 'portal', category: 'kiosk', type: 'issue',
    location: 'NW-HQ', thing: 'KSK-LOBBY', assignee: 'diego',
    comments: [{ author: 'diego', body: 'Scheduling a reimage — the current image is about four months old.', requests_reply: true }],
  },
  {
    key: 'nw-unmatched-intake', customer: 'northwind',
    title: 'Unknown device reporting faults',
    body: 'Machine intake referenced a device code we do not have in the catalog yet.',
    priority: 'normal', source: 'nats', category: 'iot-device', type: 'issue',
    origin_subject: 'helpdesk.org_northwind.tickets.create',
    // No `thing`: this is the unmatched-code fallback the intakes produce, and a
    // demo should show what that actually looks like in the UI.
    thing_note: 'NW-SENSOR-88',
    location_note: 'somewhere on the mezzanine',
  },
  {
    key: 'lakeside-ap-survey', customer: 'lakeside',
    title: 'Site survey for AP refresh',
    body: 'Walk the building, confirm mount points and cable runs before ordering.',
    priority: 'normal', source: 'agent', category: 'network', type: 'install',
    location: 'LS-CENTRAL-HS', thing: 'AP-HS-2F', project: 'ls-summer-refresh',
    assignee: 'priya', estimated_minutes: 240,
    visits: [{ status: 'canceled', at: daysFromNow(-5, 10), assignee: 'priya', notes: 'Canceled — building closed for testing.' }],
  },
]

// ---------------------------------------------------------------- seeding

const ids = {
  customers: {}, locations: {}, things: {}, locationTypes: {}, thingTypes: {},
  staff: {}, requesters: {}, categories: {}, projects: {},
}

async function seedCustomers() {
  console.log('\ncustomers')
  for (const c of CUSTOMERS) {
    const { rec } = await ensure(
      'customers', `name='${esc(c.name)}'`,
      {
        name: c.name, active: true,
        email_domain: c.domain,
        show_time_to_requester: !!c.showTime,
        ...(c.org ? { platform_org_id: c.org } : {}),
      },
      c.name,
    )
    ids.customers[c.key] = rec.id
  }
}

async function seedTypes() {
  console.log('\ntype taxonomies')
  for (const t of LOCATION_TYPES) {
    const cust = ids.customers[t.customer]
    const { rec } = await ensure(
      'location_types', `customer='${cust}' && code='${esc(t.code)}'`,
      {
        customer: cust, code: t.code, name: t.name,
        description: t.description || '',
        metadata_schema: t.schema || null,
      },
      `${t.customer}/${t.code}`,
    )
    ids.locationTypes[`${t.customer}:${t.code}`] = rec.id
  }
  for (const t of THING_TYPES) {
    const cust = ids.customers[t.customer]
    const { rec } = await ensure(
      'thing_types', `customer='${cust}' && code='${esc(t.code)}'`,
      {
        customer: cust, code: t.code, name: t.name,
        description: t.description || '',
        metadata_schema: t.schema || null,
      },
      `${t.customer}/${t.code}`,
    )
    ids.thingTypes[`${t.customer}:${t.code}`] = rec.id
  }
}

async function seedLocations() {
  console.log('\nlocations')
  // Pass 1: create without `parent`, since a parent may appear later in the list.
  for (const l of LOCATIONS) {
    const cust = ids.customers[l.customer]
    const { rec } = await ensure(
      'locations', `customer='${cust}' && code='${esc(l.code)}'`,
      {
        customer: cust, code: l.code, name: l.name,
        address: l.address || '', notes: l.notes || '',
        contact: l.contact || '', contact_phone: l.contact_phone || '',
        lat: l.lat || 0, lng: l.lng || 0,
        type: ids.locationTypes[`${l.customer}:${l.type}`] || '',
        metadata: l.metadata || null,
      },
      l.code,
    )
    ids.locations[l.code] = rec.id
  }
  // Pass 2: wire `parent` now that every id is known. This is the same two-pass
  // shape a platform export→seed needs, since exports carry raw ids, not codes.
  for (const l of LOCATIONS) {
    if (!l.parent || DRY) continue
    const id = ids.locations[l.code]
    const parentId = ids.locations[l.parent]
    if (!id || !parentId) continue
    const current = await api(`/api/collections/locations/records/${id}`)
    if (current.parent === parentId) continue
    await api(`/api/collections/locations/records/${id}`, { method: 'PATCH', body: { parent: parentId } })
    console.log(`  ~ locations: ${l.code} parent → ${l.parent}`)
  }
}

async function seedThings() {
  console.log('\nthings')
  for (const t of THINGS) {
    const cust = ids.customers[t.customer]
    // Codeless gear (the superset case) has no natural key but its name.
    const filter = t.code
      ? `customer='${cust}' && code='${esc(t.code)}'`
      : `customer='${cust}' && name='${esc(t.name)}'`
    const { rec } = await ensure(
      'things', filter,
      {
        customer: cust, code: t.code || '', name: t.name,
        type: ids.thingTypes[`${t.customer}:${t.type}`] || '',
        location: ids.locations[t.location] || '',
        notes: t.notes || '',
        retired: !!t.retired,
        metadata: t.metadata || null,
      },
      t.code || t.name,
    )
    ids.things[t.code || t.name] = rec.id
  }
}

async function seedPeople() {
  console.log('\nstaff')
  for (const s of STAFF) {
    const { rec } = await ensure(
      'staff', `email='${esc(s.email)}'`,
      {
        email: s.email, password: DEMO_PASSWORD, passwordConfirm: DEMO_PASSWORD,
        name: s.name, role: s.role, active: true, verified: true,
      },
      `${s.name} (${s.role})`,
    )
    ids.staff[s.key] = rec.id
  }

  console.log('\nrequesters')
  for (const r of REQUESTERS) {
    const { rec } = await ensure(
      'users', `email='${esc(r.email)}'`,
      {
        email: r.email, password: DEMO_PASSWORD, passwordConfirm: DEMO_PASSWORD,
        name: r.name, customer: ids.customers[r.customer], active: true, verified: true,
        ...(r.phone ? { phone: r.phone } : {}),
      },
      `${r.name} @ ${r.customer}`,
    )
    ids.requesters[r.key] = rec.id
  }
}

async function loadCategories() {
  // Seeded by migration 1806000000 — look them up rather than creating.
  const r = await api('/api/collections/ticket_categories/records?perPage=200')
  for (const c of r.items || []) ids.categories[c.key] = c.id
}

async function seedProjects() {
  console.log('\nprojects')
  for (const p of PROJECTS) {
    const cust = ids.customers[p.customer]
    const { rec } = await ensure(
      'projects', `customer='${cust}' && title='${esc(p.title)}'`,
      {
        customer: cust, location: ids.locations[p.location] || '',
        title: p.title, description: p.description, status: p.status,
        start_date: p.start, target_date: p.target,
        lead: ids.staff[p.lead] || '',
      },
      p.title,
    )
    ids.projects[p.key] = rec.id
  }
}

async function seedTickets() {
  console.log('\ntickets')
  for (const t of TICKETS) {
    const dedupe = `seed-${t.key}`
    const cust = ids.customers[t.customer]
    const thingId = t.thing ? ids.things[t.thing] : ''

    const { rec, created } = await ensure(
      'tickets', `dedupe_key='${esc(dedupe)}'`,
      {
        customer: cust,
        title: t.title, body: t.body,
        priority: t.priority, source: t.source, type: t.type || 'issue',
        status: 'open',
        dedupe_key: dedupe,
        category: ids.categories[t.category] || '',
        project: t.project ? ids.projects[t.project] : '',
        location: t.location ? ids.locations[t.location] || '' : '',
        location_note: t.location_note || '',
        thing: thingId || '',
        thing_note: t.thing_note || '',
        assignee: t.assignee ? ids.staff[t.assignee] : '',
        requester: t.requester ? ids.requesters[t.requester] : '',
        ...(t.estimated_minutes ? { estimated_minutes: t.estimated_minutes } : {}),
        ...(t.origin_subject ? { origin_subject: t.origin_subject } : {}),
      },
      `#${t.key}`,
    )

    if (DRY) continue

    // Children are each idempotent on their own natural key rather than gated on
    // "was the ticket new". Gating is tempting and wrong: a run that dies partway
    // leaves the ticket created but childless, and every later run then skips it
    // forever. Converging per-child costs one lookup each and self-heals.
    if (!created) stats.skipped++

    for (const c of t.comments || []) {
      if (await findFirst('ticket_comments', `ticket='${rec.id}' && body='${esc(c.body)}'`)) continue
      const staffRow = STAFF.find((s) => s.key === c.author)
      const userRow = REQUESTERS.find((r) => r.key === c.author)
      // Authored as the person, not forged by the admin — see api()'s `auth`.
      const as = staffRow
        ? await tokenFor('staff', staffRow.email)
        : await tokenFor('users', userRow.email)
      await api('/api/collections/ticket_comments/records', {
        method: 'POST',
        auth: as,
        body: {
          ticket: rec.id,
          body: c.body,
          // `internal` and `requests_reply` are staff-only: the requester branch
          // of the create rule pins `internal:isset = false`, so sending it at
          // all — even false — fails the rule.
          ...(staffRow
            ? {
                author_staff: ids.staff[c.author],
                internal: !!c.internal,
                requests_reply: !!c.requests_reply,
              }
            : { author_user: ids.requesters[c.author] }),
        },
        headers: { 'X-Helpdesk-Quiet': '1' },
      })
    }

    for (const v of t.visits || []) {
      // A ticket never has two visits with the same dispatch note in this data,
      // so (ticket, notes) is a serviceable natural key.
      if (await findFirst('visits', `ticket='${rec.id}' && notes='${esc(v.notes || '')}'`)) continue
      const body = {
        ticket: rec.id,
        status: v.status,
        notes: v.notes || '',
        ...(v.at ? { scheduled_at: v.at } : {}),
        ...(v.assignee ? { assignee: ids.staff[v.assignee] } : {}),
        ...(v.completedDays !== undefined ? { completed_at: daysFromNow(v.completedDays, 16) } : {}),
      }
      // A `requested` visit is the needs-scheduling case: no time, no tech. The
      // guard hook requires both on a *scheduled* visit, so send neither here.
      if (v.status === 'requested') {
        delete body.scheduled_at
        delete body.assignee
      }
      await api('/api/collections/visits/records', {
        method: 'POST', body, headers: { 'X-Helpdesk-Quiet': '1' },
      })
    }

    for (const e of t.time || []) {
      if (await findFirst('time_entries', `ticket='${rec.id}' && note='${esc(e.note || '')}'`)) continue
      const staffRow = STAFF.find((s) => s.key === e.staff)
      // time_entries pins `staff` to the caller too, so log as that technician.
      const as = await tokenFor('staff', staffRow.email)
      await api('/api/collections/time_entries/records', {
        method: 'POST',
        auth: as,
        body: {
          ticket: rec.id,
          staff: ids.staff[e.staff],
          minutes: e.minutes,
          work_date: dateOnly(e.days),
          note: e.note || '',
          ...(e.non_billable ? { non_billable: true } : {}),
        },
      })
    }

    // Quiet lifecycle transitions last, so the audit timeline reads as a real
    // progression instead of a single "created" row. Replayed only while the
    // ticket is still at the created default — otherwise a re-run would walk it
    // back through open → in_progress → … and fabricate a second set of audit
    // events every time.
    const fresh = await api(`/api/collections/tickets/records/${rec.id}`)
    if (fresh.status === 'open') {
      for (const tr of t.transitions || []) {
        await quietUpdate('tickets', rec.id, tr)
      }
    }
  }
}

// ---------------------------------------------------------------- main

async function main() {
  if (!ADMIN_PASSWORD) {
    console.error(
      'Missing admin password.\n' +
        '  node scripts/seed-demo.mjs --url <url> --email <admin> --password <pw>\n' +
        '  (or set HELPDESK_ADMIN_PASSWORD)',
    )
    process.exit(2)
  }

  console.log(`helpdesk demo seeder → ${BASE}${DRY ? '  [DRY RUN — no writes]' : ''}`)
  const who = await login()
  console.log(`authenticated as ${who}`)

  await seedCustomers()
  await seedTypes()
  await seedLocations()
  await seedThings()
  await seedPeople()
  await loadCategories()
  await seedProjects()
  await seedTickets()

  console.log(
    `\ndone — ${stats.created} created, ${stats.matched} already present` +
      (stats.skipped ? `, ${stats.skipped} tickets already existed (children reconciled)` : ''),
  )
  if (!DRY) console.log(`demo logins all use password: ${DEMO_PASSWORD}`)
}

main().catch((err) => {
  console.error(`\nfailed: ${err.message}`)
  process.exit(1)
})
