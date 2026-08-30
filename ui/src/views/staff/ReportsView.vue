<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { pb } from '@/pb'
import type { Customer, Location, Thing, Ticket, TimeEntry, Visit } from '@/types'
import CategoryBadge from '@/components/CategoryBadge.vue'
import SearchSelect from '@/components/SearchSelect.vue'
import ReportTable, { type ReportColumn } from '@/components/ReportTable.vue'
import { useQuerySync, useQueryValue } from '@/composables/useQuerySync'

// Aggregate the data the app already captures — logged time, completed
// visits, and ticket volume — over a date range, optionally scoped to one
// customer, location and/or thing. No new storage; just rollups the per-ticket
// cards never surfaced. Handy for month-end billing, utilization, and spotting what
// breaks most. Every rollup is exportable (per-report + Export all), and the
// underlying time/visit rows export in detail.
//
// LAYOUT: the filters and the totals row are PINNED, and the rollups are tabbed
// one at a time beneath them. The totals are not a report — they are the
// denominator every report is read against ("14h on this device" means nothing
// without "180h logged overall"), so putting them behind a tab of their own
// would hide the number you are comparing to. The rollups genuinely are
// alternatives: you arrive with one question, and stacking all seven made a
// very long page of which six tables were noise. Tabs also give each table the
// full width, which is what killed the ragged whitespace the old 2-up grid
// produced whenever a short table sat beside a long one (CSS grid stretches a
// row to its tallest cell).
//
// A shared "group by" dropdown would be the other obvious control here, and it
// is the wrong one: these tables do not share a column set (staff has Field and
// Visits, location has Tickets and Planned, category has Total and Open), so
// one selector would force flattening away distinctions that are the point.
//
// Ticket rollups count tickets *created* in the range. Resolution-time and
// reopen-rate metrics need the ticket_events history and land in a later
// pass (they aren't derivable from ticket fields alone).

const entries = ref<TimeEntry[]>([])
const doneVisits = ref<Visit[]>([])
const tickets = ref<Ticket[]>([])
const loading = ref(false)
const error = ref('')

// Filter option lists.
const customers = ref<Customer[]>([])
const locations = ref<Location[]>([])
const things = ref<Thing[]>([])

// Default to the trailing 30 days.
function isoDate(offsetDays: number): string {
  const d = new Date()
  d.setDate(d.getDate() + offsetDays)
  return d.toISOString().slice(0, 10)
}
// The scope reads from the URL too, so a report can be sent to someone as the
// exact figures you were looking at. `from`/`to` are always written, never
// omitted as defaults: "the trailing 30 days" means something different the day
// after you send it, and a link to a month-end total that quietly slides
// forward is worse than no link.
const q = useQueryValue()
const from = ref(q('from', isoDate(-30)))
const to = ref(q('to', isoDate(0)))
const customerFilter = ref(q('customer'))
const locationFilter = ref(q('location'))
const thingFilter = ref(q('thing'))

const customerOptions = computed(() => customers.value.map((c) => ({ id: c.id, label: c.name })))
// Location picker narrows to the selected customer's sites when one is chosen.
const locationOptions = computed(() => {
  const list = customerFilter.value
    ? locations.value.filter((l) => l.customer === customerFilter.value)
    : locations.value
  return list.map((l) => ({ id: l.id, label: l.name, sublabel: l.expand?.customer?.name || undefined }))
})
// Thing picker narrows the same way, and by site too when one is chosen — a
// device's location is optional, so unsited gear stays offered rather than
// vanishing behind a site scope it never claimed.
const thingOptions = computed(() => {
  let list = things.value
  if (customerFilter.value) list = list.filter((t) => t.customer === customerFilter.value)
  if (locationFilter.value) list = list.filter((t) => !t.location || t.location === locationFilter.value)
  return list.map((t) => ({
    id: t.id,
    label: t.name,
    sublabel: [t.expand?.type?.name, t.expand?.customer?.name].filter(Boolean).join(' · ') || undefined,
  }))
})
const customerName = computed(() => customers.value.find((c) => c.id === customerFilter.value)?.name || '')
const locationName = computed(() => locations.value.find((l) => l.id === locationFilter.value)?.name || '')
const thingName = computed(() => things.value.find((t) => t.id === thingFilter.value)?.name || '')

function pbTime(localDate: string, endOfDay: boolean): string {
  return new Date(`${localDate}T${endOfDay ? '23:59:59' : '00:00:00'}`).toISOString().replace('T', ' ')
}
function rangeFilter(field: string): string {
  return `${field} >= '${pbTime(from.value, false)}' && ${field} <= '${pbTime(to.value, true)}'`
}
// Customer/location/thing scope. `prefix` is '' for tickets (all three live on
// the record) and 'ticket.' for time_entries/visits (relation-hop to the ticket).
function scopeFilter(prefix: string): string {
  const parts: string[] = []
  if (customerFilter.value) parts.push(`${prefix}customer = '${customerFilter.value}'`)
  if (locationFilter.value) parts.push(`${prefix}location = '${locationFilter.value}'`)
  if (thingFilter.value) parts.push(`${prefix}thing = '${thingFilter.value}'`)
  return parts.join(' && ')
}
function and(...clauses: string[]): string {
  return clauses.filter(Boolean).join(' && ')
}

async function load() {
  loading.value = true
  error.value = ''
  try {
    ;[entries.value, doneVisits.value, tickets.value] = await Promise.all([
      pb.collection('time_entries').getFullList<TimeEntry>({
        filter: and(rangeFilter('work_date'), scopeFilter('ticket.')),
        sort: '-work_date',
        expand: 'staff,ticket,ticket.customer,ticket.location,ticket.thing,ticket.thing.type',
      }),
      pb.collection('visits').getFullList<Visit>({
        filter: and(`status = 'completed'`, rangeFilter('completed_at'), scopeFilter('ticket.')),
        sort: '-completed_at',
        expand: 'assignee,ticket,ticket.customer,ticket.location,ticket.thing,ticket.thing.type',
      }),
      pb.collection('tickets').getFullList<Ticket>({
        filter: and(rangeFilter('created'), scopeFilter('')),
        sort: '-created',
        expand: 'category,location,thing,thing.type',
      }),
    ])
  } catch (err: any) {
    error.value = err?.message || 'Failed to load reports'
  } finally {
    loading.value = false
  }
}

async function loadOptions() {
  try {
    ;[customers.value, locations.value, things.value] = await Promise.all([
      pb.collection('customers').getFullList<Customer>({ sort: 'name' }),
      pb.collection('locations').getFullList<Location>({ sort: 'name', expand: 'customer' }),
      // Retired gear included: you can't file new work against it, but reading
      // what it cost before it was pulled is the point of keeping the row.
      pb.collection('things').getFullList<Thing>({ sort: 'name', expand: 'customer,type' }),
    ])
  } catch {
    // Filters degrade to date-only; the rollups still load.
  }
}

// --- rollups ---
interface Row {
  label: string
  minutes: number
  billableMinutes: number // subset of minutes NOT flagged non_billable
  fieldMinutes: number // subset of minutes attributed to an on-site visit
  visits: number
}
function group(keyer: (fromEntry: boolean, rec: any) => string): Row[] {
  const map = new Map<string, Row>()
  const row = (label: string) => {
    const k = label || '—'
    if (!map.has(k)) map.set(k, { label: k, minutes: 0, billableMinutes: 0, fieldMinutes: 0, visits: 0 })
    return map.get(k)!
  }
  for (const e of entries.value) {
    const r = row(keyer(true, e))
    r.minutes += e.minutes
    if (!e.non_billable) r.billableMinutes += e.minutes
    if (e.visit) r.fieldMinutes += e.minutes
  }
  for (const v of doneVisits.value) row(keyer(false, v)).visits += 1
  return [...map.values()].sort((a, b) => b.minutes - a.minutes || b.visits - a.visits)
}

const byPerson = computed(() =>
  group((isEntry, rec) =>
    isEntry ? rec.expand?.staff?.name || '' : rec.expand?.assignee?.name || '',
  ),
)
const byCustomer = computed(() =>
  group((_isEntry, rec) => rec.expand?.ticket?.expand?.customer?.name || ''),
)

// The axes that hang off the ticket — location, thing, thing type — roll up
// identically: time and visits come from work done in range, tickets/planned
// from tickets created in range, and the "—" bucket is work where the field was
// never set (most reactive tickets). One grouper rather than three near-copies;
// only the label and the sort differ.
//
// `keyOf` reads the label off a TICKET record. Entries and visits reach theirs
// through `expand.ticket`, which is why the callers below are one-liners.
interface AxisRow {
  label: string
  minutes: number
  billableMinutes: number
  visits: number
  tickets: number
  planned: number
}
function byTicketAxis(keyOf: (t: any) => string, sortBy: 'tickets' | 'minutes'): AxisRow[] {
  const map = new Map<string, AxisRow>()
  const row = (label: string) => {
    const k = label || '—'
    if (!map.has(k)) map.set(k, { label: k, minutes: 0, billableMinutes: 0, visits: 0, tickets: 0, planned: 0 })
    return map.get(k)!
  }
  for (const e of entries.value) {
    const r = row(keyOf(e.expand?.ticket))
    r.minutes += e.minutes
    if (!e.non_billable) r.billableMinutes += e.minutes
  }
  for (const v of doneVisits.value) row(keyOf(v.expand?.ticket)).visits += 1
  for (const t of tickets.value) {
    const r = row(keyOf(t))
    r.tickets += 1
    if (t.type === 'planned') r.planned += 1
  }
  // The "—" bucket sinks to the bottom regardless of size. It is usually the
  // biggest row — most reactive tickets name no device — but "work we didn't
  // attribute" is not an answer to "which devices cost us the most", and
  // letting it head the table buries the row you came for.
  return [...map.values()].sort((a, b) => {
    if ((a.label === '—') !== (b.label === '—')) return a.label === '—' ? 1 : -1
    return sortBy === 'minutes'
      ? b.minutes - a.minutes || b.tickets - a.tickets
      : b.tickets - a.tickets || b.minutes - a.minutes
  })
}

// Where the work happens. Volume-first: a site's ticket count is the headline.
const byLocation = computed(() => byTicketAxis((t) => t?.expand?.location?.name || '', 'tickets'))
// What the work is ON — the question free-text `asset` could never answer, and
// the stated reason things were promoted to a relation. Hours-first: "which
// devices burn the most time" is the point, not which are mentioned most.
const byThing = computed(() => byTicketAxis((t) => t?.expand?.thing?.name || '', 'minutes'))
// The same question one level up the taxonomy: door controllers cost us N hours
// across every customer. This is what customer-scoped types are FOR — the name
// is shared even though each customer owns its own type row, so grouping by it
// aggregates across the whole book of business. (Same reasoning as the roster
// filters in RosterFilters.vue.)
const byThingType = computed(() => byTicketAxis((t) => t?.expand?.thing?.expand?.type?.name || '', 'minutes'))

const totalMinutes = computed(() => entries.value.reduce((s, e) => s + e.minutes, 0))
const totalFieldMinutes = computed(() =>
  entries.value.filter((e) => e.visit).reduce((s, e) => s + e.minutes, 0),
)
// Billable split: the invoiceable share vs. rework/goodwill written off. The
// write-off rate is the utilization number an MSP watches.
const totalNonBillableMinutes = computed(() =>
  entries.value.filter((e) => e.non_billable).reduce((s, e) => s + e.minutes, 0),
)
const totalBillableMinutes = computed(() => totalMinutes.value - totalNonBillableMinutes.value)
const billablePct = computed(() =>
  totalMinutes.value ? Math.round((totalBillableMinutes.value / totalMinutes.value) * 100) : 0,
)
const totalVisits = computed(() => doneVisits.value.length)
const totalTickets = computed(() => tickets.value.length)

// Ticket volume by category (created in range): total + how many are still
// open, so a big "Uncategorized" or a hot category jumps out.
interface CatRow {
  label: string
  color?: string
  count: number
  open: number
}
const byCategory = computed<CatRow[]>(() => {
  const map = new Map<string, CatRow>()
  for (const t of tickets.value) {
    const cat = t.expand?.category
    const label = cat?.name || 'Uncategorized'
    if (!map.has(label)) map.set(label, { label, color: cat?.color, count: 0, open: 0 })
    const row = map.get(label)!
    row.count += 1
    if (t.status !== 'resolved' && t.status !== 'closed') row.open += 1
  }
  return [...map.values()].sort((a, b) => b.count - a.count)
})

// Source mix (portal / agent / nats / webhook): how much work arrives by
// each channel — the machine-generated share is the automation story.
const bySource = computed(() =>
  [...tickets.value.reduce((m, t) => m.set(t.source, (m.get(t.source) || 0) + 1), new Map<string, number>()).entries()]
    // The dimension is named "label" like every other rollup, so ReportTable
    // reads it off the first column with no special case.
    .map(([label, count]) => ({ label, count, pct: totalTickets.value ? Math.round((count / totalTickets.value) * 100) : 0 }))
    .sort((a, b) => b.count - a.count),
)

function fmtHours(m: number): string {
  if (!m) return '—'
  const h = Math.floor(m / 60)
  return h > 0 ? `${h}h ${m % 60}m` : `${m}m`
}

// --- CSV export ---
function csvEscape(v: unknown): string {
  const s = String(v ?? '')
  return /[",\n]/.test(s) ? `"${s.replace(/"/g, '""')}"` : s
}
function download(name: string, lines: string[]) {
  const blob = new Blob([lines.join('\n')], { type: 'text/csv;charset=utf-8' })
  const a = document.createElement('a')
  a.href = URL.createObjectURL(blob)
  a.download = name
  a.click()
  URL.revokeObjectURL(a.href)
}
const suffix = () => `${from.value}_${to.value}`

// Each rollup is a self-describing report: minutes stay numeric in CSV (for
// spreadsheet math), even though the screen shows h/m. This one list drives the
// tab strip, the card heading and the CSV, so a rollup can never appear as a tab
// without an export or drift out of sync with its own header row.
interface Report {
  key: string
  /** Short tab label; `title` is the full heading above the table. */
  tab: string
  title: string
  filename: string
  header: string[]
  rows: () => (string | number)[][]
}
const reports = computed<Report[]>(() => [
  {
    key: 'staff',
    tab: 'Staff',
    title: 'By staff / technician',
    filename: 'by-staff',
    header: ['name', 'minutes', 'billable_minutes', 'field_minutes', 'visits'],
    rows: () => byPerson.value.map((r) => [r.label === '—' ? '(unattributed)' : r.label, r.minutes, r.billableMinutes, r.fieldMinutes, r.visits]),
  },
  {
    key: 'customer',
    tab: 'Customer',
    title: 'By customer',
    filename: 'by-customer',
    header: ['customer', 'minutes', 'billable_minutes', 'field_minutes', 'visits'],
    rows: () => byCustomer.value.map((r) => [r.label === '—' ? '(none)' : r.label, r.minutes, r.billableMinutes, r.fieldMinutes, r.visits]),
  },
  {
    key: 'location',
    tab: 'Location',
    title: 'By location',
    filename: 'by-location',
    header: ['location', 'tickets', 'planned', 'minutes', 'billable_minutes', 'visits'],
    rows: () => byLocation.value.map((r) => [r.label === '—' ? '(no location)' : r.label, r.tickets, r.planned, r.minutes, r.billableMinutes, r.visits]),
  },
  {
    key: 'thing',
    tab: 'Thing',
    title: 'By thing',
    filename: 'by-thing',
    header: ['thing', 'minutes', 'billable_minutes', 'tickets', 'planned', 'visits'],
    rows: () => byThing.value.map((r) => [r.label === '—' ? '(no thing)' : r.label, r.minutes, r.billableMinutes, r.tickets, r.planned, r.visits]),
  },
  {
    key: 'thingtype',
    tab: 'Thing type',
    title: 'By thing type',
    filename: 'by-thing-type',
    header: ['thing_type', 'minutes', 'billable_minutes', 'tickets', 'planned', 'visits'],
    rows: () => byThingType.value.map((r) => [r.label === '—' ? '(untyped)' : r.label, r.minutes, r.billableMinutes, r.tickets, r.planned, r.visits]),
  },
  {
    key: 'category',
    tab: 'Category',
    title: 'Tickets by category',
    filename: 'tickets-by-category',
    header: ['category', 'total', 'open'],
    rows: () => byCategory.value.map((r) => [r.label, r.count, r.open]),
  },
  {
    key: 'source',
    tab: 'Source',
    title: 'Tickets by source',
    filename: 'tickets-by-source',
    header: ['source', 'count', 'share_pct'],
    rows: () => bySource.value.map((r) => [r.label, r.count, r.pct]),
  },
])

// --- the visible rollup ---
// Which tab is open rides the URL (?view=thing), so a report is linkable, is
// what you land back on after a reload, and survives the back button. An
// unrecognised value falls back rather than rendering nothing.
const DEFAULT_VIEW = 'customer' // month-end billing is the errand people arrive with
const view = ref(reports.value.some((r) => r.key === q('view')) ? q('view') : DEFAULT_VIEW)

// The open tab and the scope it is read under travel together — a tab alone
// was linkable but pointed at whatever range the recipient's page defaulted to.
useQuerySync(
  { view, from, to, customer: customerFilter, location: locationFilter, thing: thingFilter },
  { view: DEFAULT_VIEW },
)
const activeReport = computed(() => reports.value.find((r) => r.key === view.value))

// Column specs. The measure carrying `bar` is the one each rollup is sorted by
// — the bar is a reading aid for the ranking, so pointing it at another column
// would fight the row order instead of reinforcing it.
const H = { numeric: true, hours: true } as const
const N = { numeric: true } as const
const COLUMNS: Record<string, ReportColumn[]> = {
  staff: [
    { key: 'label', label: 'Name' },
    { key: 'minutes', label: 'Time', ...H, bar: true },
    { key: 'billableMinutes', label: 'Billable', ...H },
    { key: 'fieldMinutes', label: 'Field', ...H },
    { key: 'visits', label: 'Visits', ...N },
  ],
  customer: [
    { key: 'label', label: 'Customer' },
    { key: 'minutes', label: 'Time', ...H, bar: true },
    { key: 'billableMinutes', label: 'Billable', ...H },
    { key: 'fieldMinutes', label: 'Field', ...H },
    { key: 'visits', label: 'Visits', ...N },
  ],
  location: [
    { key: 'label', label: 'Location' },
    { key: 'tickets', label: 'Tickets', ...N, bar: true },
    { key: 'planned', label: 'Planned', ...N },
    { key: 'minutes', label: 'Time', ...H },
    { key: 'billableMinutes', label: 'Billable', ...H },
    { key: 'visits', label: 'Visits', ...N },
  ],
  thing: [
    { key: 'label', label: 'Thing' },
    { key: 'minutes', label: 'Time', ...H, bar: true },
    { key: 'billableMinutes', label: 'Billable', ...H },
    { key: 'tickets', label: 'Tickets', ...N },
    { key: 'planned', label: 'Planned', ...N },
    { key: 'visits', label: 'Visits', ...N },
  ],
  thingtype: [
    { key: 'label', label: 'Type' },
    { key: 'minutes', label: 'Time', ...H, bar: true },
    { key: 'billableMinutes', label: 'Billable', ...H },
    { key: 'tickets', label: 'Tickets', ...N },
    { key: 'planned', label: 'Planned', ...N },
    { key: 'visits', label: 'Visits', ...N },
  ],
  category: [
    { key: 'label', label: 'Category' },
    { key: 'count', label: 'Total', ...N, bar: true },
    { key: 'open', label: 'Open', ...N },
  ],
  source: [
    { key: 'label', label: 'Source' },
    { key: 'count', label: 'Count', ...N, bar: true },
    { key: 'pct', label: 'Share %', ...N },
  ],
}

// What the unattributed row reads as, per rollup.
const NULL_LABELS: Record<string, string> = {
  staff: 'Unattributed',
  customer: 'None',
  location: 'No location',
  thing: 'No thing',
  thingtype: 'Untyped',
}

const ROWS: Record<string, () => any[]> = {
  staff: () => byPerson.value,
  customer: () => byCustomer.value,
  location: () => byLocation.value,
  thing: () => byThing.value,
  thingtype: () => byThingType.value,
  category: () => byCategory.value,
  source: () => bySource.value,
}
const activeRows = computed(() => ROWS[view.value]?.() || [])
const activeColumns = computed(() => COLUMNS[view.value] || [])

function exportOne(key: string) {
  const rep = reports.value.find((r) => r.key === key)
  if (!rep) return
  const lines = [rep.header.join(','), ...rep.rows().map((r) => r.map(csvEscape).join(','))]
  download(`${rep.filename}-${suffix()}.csv`, lines)
}

// Export all rollups as one snapshot file: range + scope + totals header, then
// each report as a titled section.
function exportAll() {
  const lines: string[] = [`Reports,${from.value} to ${to.value}`]
  if (customerName.value) lines.push(`Customer,${csvEscape(customerName.value)}`)
  if (locationName.value) lines.push(`Location,${csvEscape(locationName.value)}`)
  if (thingName.value) lines.push(`Thing,${csvEscape(thingName.value)}`)
  lines.push('', 'Totals', 'metric,value',
    `time_minutes,${totalMinutes.value}`,
    `billable_minutes,${totalBillableMinutes.value}`,
    `non_billable_minutes,${totalNonBillableMinutes.value}`,
    `field_minutes,${totalFieldMinutes.value}`,
    `visits_completed,${totalVisits.value}`,
    `tickets_created,${totalTickets.value}`, '')
  for (const rep of reports.value) {
    lines.push(rep.title, rep.header.join(','))
    for (const row of rep.rows()) lines.push(row.map(csvEscape).join(','))
    lines.push('')
  }
  download(`reports-${suffix()}.csv`, lines)
}

// Detail exports: the underlying time and visit rows, not the rollups.
function exportTime() {
  const lines = [['work_date', 'staff', 'customer', 'ticket', 'site', 'thing', 'minutes', 'billable', 'on_site', 'note'].join(',')]
  for (const e of entries.value) {
    lines.push(
      [
        e.work_date,
        e.expand?.staff?.name || '',
        e.expand?.ticket?.expand?.customer?.name || '',
        e.expand?.ticket?.number ?? '',
        e.expand?.ticket?.expand?.location?.name || '',
        e.expand?.ticket?.expand?.thing?.name || '',
        e.minutes,
        e.non_billable ? 'no' : 'yes',
        e.visit ? 'yes' : '',
        e.note || '',
      ]
        .map(csvEscape)
        .join(','),
    )
  }
  download(`time-detail-${suffix()}.csv`, lines)
}
function exportVisits() {
  const lines = [['completed_at', 'technician', 'customer', 'ticket', 'site', 'thing', 'directions'].join(',')]
  for (const v of doneVisits.value) {
    lines.push(
      [
        v.completed_at || '',
        v.expand?.assignee?.name || '',
        v.expand?.ticket?.expand?.customer?.name || '',
        v.expand?.ticket?.number ?? '',
        v.expand?.ticket?.expand?.location?.name || '',
        v.expand?.ticket?.expand?.thing?.name || '',
        v.location || '',
      ]
        .map(csvEscape)
        .join(','),
    )
  }
  download(`visits-detail-${suffix()}.csv`, lines)
}

// Narrowing the scope drops any finer selection it no longer owns: switching
// customer can orphan both the location and the thing, and switching location
// can orphan a sited thing. Clearing retriggers this watcher, which then loads
// once — so each branch returns rather than falling through to load().
watch([from, to, customerFilter, locationFilter, thingFilter], (cur, prev) => {
  const [, , c, l] = cur
  const [, , pc, pl] = prev
  const thing = thingFilter.value
    ? things.value.find((t) => t.id === thingFilter.value)
    : undefined
  if (c !== pc && c) {
    if (l && !locations.value.some((loc) => loc.id === l && loc.customer === c)) {
      locationFilter.value = ''
      return
    }
    if (thing && thing.customer !== c) {
      thingFilter.value = ''
      return
    }
  }
  // A thing with no location is filed under no site, so a site scope never
  // orphans it — matching thingOptions.
  if (l !== pl && l && thing?.location && thing.location !== l) {
    thingFilter.value = ''
    return
  }
  load()
})
onMounted(() => {
  load()
  loadOptions()
})
</script>

<template>
  <div class="space-y-4">
    <div class="flex flex-row justify-between items-center gap-2">
      <h1 class="text-2xl font-bold">Reports</h1>
      <div class="dropdown dropdown-end">
        <div tabindex="0" role="button" class="btn btn-sm btn-primary">Export ▾</div>
        <ul tabindex="0" class="dropdown-content menu menu-sm bg-base-100 rounded-box shadow-lg border border-base-300 w-60 max-w-[calc(100vw-2rem)] p-1 z-50">
          <li><a @click="exportAll">All reports (CSV)</a></li>
          <li class="menu-title px-2 pt-2 pb-1 text-xs">Detail rows</li>
          <li><a @click="exportTime">Time entries — detail</a></li>
          <li><a @click="exportVisits">Completed visits — detail</a></li>
        </ul>
      </div>
    </div>

    <!-- Filters: date range + customer/location/thing scope (applies to every rollup). -->
    <div class="flex flex-col sm:flex-row sm:flex-wrap gap-2 sm:items-end">
      <div class="form-control w-full sm:w-auto">
        <label class="label py-1"><span class="label-text text-xs">From</span></label>
        <input v-model="from" type="date" class="input input-bordered input-sm w-full sm:w-auto" />
      </div>
      <div class="form-control w-full sm:w-auto">
        <label class="label py-1"><span class="label-text text-xs">To</span></label>
        <input v-model="to" type="date" class="input input-bordered input-sm w-full sm:w-auto" />
      </div>
      <div class="form-control w-full sm:w-52">
        <label class="label py-1"><span class="label-text text-xs">Customer</span></label>
        <SearchSelect v-model="customerFilter" :options="customerOptions" size="sm" empty-label="All customers" placeholder="Any customer…" />
      </div>
      <div class="form-control w-full sm:w-52">
        <label class="label py-1"><span class="label-text text-xs">Location</span></label>
        <SearchSelect v-model="locationFilter" :options="locationOptions" size="sm" empty-label="All locations" placeholder="Any location…" />
      </div>
      <div class="form-control w-full sm:w-52">
        <label class="label py-1"><span class="label-text text-xs">Thing</span></label>
        <SearchSelect v-model="thingFilter" :options="thingOptions" size="sm" empty-label="All things" placeholder="Any thing…" />
      </div>
    </div>

    <div v-if="error" class="alert alert-error">{{ error }}</div>
    <div v-if="loading" class="flex justify-center p-12"><span class="loading loading-spinner loading-lg"></span></div>

    <template v-else>
      <!-- Totals -->
      <div class="stats stats-vertical sm:stats-horizontal shadow-sm bg-base-100 w-full">
        <div class="stat">
          <div class="stat-title">Time logged</div>
          <div class="stat-value text-2xl tabular-nums">{{ fmtHours(totalMinutes) }}</div>
          <div v-if="totalFieldMinutes > 0" class="stat-desc">{{ fmtHours(totalFieldMinutes) }} on-site</div>
        </div>
        <div class="stat">
          <div class="stat-title">Billable</div>
          <div class="stat-value text-2xl tabular-nums">{{ fmtHours(totalBillableMinutes) }}</div>
          <div class="stat-desc">
            {{ billablePct }}% billable<template v-if="totalNonBillableMinutes > 0"> · {{ fmtHours(totalNonBillableMinutes) }} written off</template>
          </div>
        </div>
        <div class="stat">
          <div class="stat-title">Visits completed</div>
          <div class="stat-value text-2xl tabular-nums">{{ totalVisits }}</div>
        </div>
        <div class="stat">
          <div class="stat-title">Tickets created</div>
          <div class="stat-value text-2xl tabular-nums">{{ totalTickets }}</div>
        </div>
      </div>

      <!-- Rollups: one at a time, full width. The tab strip scrolls sideways on
           a narrow screen rather than wrapping into a second row. -->
      <div role="tablist" class="tabs tabs-bordered overflow-x-auto flex-nowrap">
        <a
          v-for="r in reports"
          :key="r.key"
          role="tab"
          class="tab whitespace-nowrap"
          :class="{ 'tab-active font-semibold': view === r.key }"
          @click="view = r.key"
        >
          {{ r.tab }}
        </a>
      </div>

      <div class="card bg-base-100 shadow-sm">
        <div class="card-body p-4 space-y-2">
          <div class="flex items-center justify-between gap-2">
            <h2 class="font-semibold text-sm">{{ activeReport?.title }}</h2>
            <button class="btn btn-ghost btn-xs" @click="exportOne(view)">CSV</button>
          </div>
          <p v-if="view === 'thingtype'" class="text-xs text-base-content/50">
            Grouped by type <em>name</em>, so a class of device aggregates across every
            customer that owns one.
          </p>
          <ReportTable
            :columns="activeColumns"
            :rows="activeRows"
            :null-label="NULL_LABELS[view] || 'Unattributed'"
            :empty="view === 'category' || view === 'source' ? 'No tickets in range.' : 'No activity in range.'"
          >
            <!-- Category is the one dimension that renders as something other
                 than text; every other rollup takes the default cell. -->
            <template v-if="view === 'category'" #label="{ row }">
              <CategoryBadge v-if="row.label !== 'Uncategorized'" :name="row.label" :color="row.color" />
              <span v-else class="text-base-content/50">Uncategorized</span>
            </template>
            <template v-else-if="view === 'source'" #label="{ row }">
              <span class="capitalize">{{ row.label }}</span>
            </template>
          </ReportTable>
        </div>
      </div>
    </template>
  </div>
</template>
