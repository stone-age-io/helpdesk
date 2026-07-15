<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { pb } from '@/pb'
import type { Customer, Location, Ticket, TimeEntry, Visit } from '@/types'
import CategoryBadge from '@/components/CategoryBadge.vue'
import SearchSelect from '@/components/SearchSelect.vue'

// Aggregate the data the app already captures — logged time, completed
// visits, and ticket volume — over a date range, optionally scoped to one
// customer and/or location. No new storage; just rollups the per-ticket cards
// never surfaced. Handy for month-end billing, utilization, and spotting what
// breaks most. Every rollup is exportable (per-report + Export all), and the
// underlying time/visit rows export in detail.
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

// Default to the trailing 30 days.
function isoDate(offsetDays: number): string {
  const d = new Date()
  d.setDate(d.getDate() + offsetDays)
  return d.toISOString().slice(0, 10)
}
const from = ref(isoDate(-30))
const to = ref(isoDate(0))
const customerFilter = ref('')
const locationFilter = ref('')

const customerOptions = computed(() => customers.value.map((c) => ({ id: c.id, label: c.name })))
// Location picker narrows to the selected customer's sites when one is chosen.
const locationOptions = computed(() => {
  const list = customerFilter.value
    ? locations.value.filter((l) => l.customer === customerFilter.value)
    : locations.value
  return list.map((l) => ({ id: l.id, label: l.name, sublabel: l.expand?.customer?.name || undefined }))
})
const customerName = computed(() => customers.value.find((c) => c.id === customerFilter.value)?.name || '')
const locationName = computed(() => locations.value.find((l) => l.id === locationFilter.value)?.name || '')

function pbTime(localDate: string, endOfDay: boolean): string {
  return new Date(`${localDate}T${endOfDay ? '23:59:59' : '00:00:00'}`).toISOString().replace('T', ' ')
}
function rangeFilter(field: string): string {
  return `${field} >= '${pbTime(from.value, false)}' && ${field} <= '${pbTime(to.value, true)}'`
}
// Customer/location scope. `prefix` is '' for tickets (customer/location live on
// the record) and 'ticket.' for time_entries/visits (relation-hop to the ticket).
function scopeFilter(prefix: string): string {
  const parts: string[] = []
  if (customerFilter.value) parts.push(`${prefix}customer = '${customerFilter.value}'`)
  if (locationFilter.value) parts.push(`${prefix}location = '${locationFilter.value}'`)
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
        expand: 'staff,ticket,ticket.customer,ticket.location',
      }),
      pb.collection('visits').getFullList<Visit>({
        filter: and(`status = 'completed'`, rangeFilter('completed_at'), scopeFilter('ticket.')),
        sort: '-completed_at',
        expand: 'assignee,ticket,ticket.customer,ticket.location',
      }),
      pb.collection('tickets').getFullList<Ticket>({
        filter: and(rangeFilter('created'), scopeFilter('')),
        sort: '-created',
        expand: 'category,location',
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
    ;[customers.value, locations.value] = await Promise.all([
      pb.collection('customers').getFullList<Customer>({ sort: 'name' }),
      pb.collection('locations').getFullList<Location>({ sort: 'name', expand: 'customer' }),
    ])
  } catch {
    // Filters degrade to date-only; the rollups still load.
  }
}

// --- rollups ---
interface Row {
  label: string
  minutes: number
  fieldMinutes: number // subset of minutes attributed to an on-site visit
  visits: number
}
function group(keyer: (fromEntry: boolean, rec: any) => string): Row[] {
  const map = new Map<string, Row>()
  const row = (label: string) => {
    const k = label || '—'
    if (!map.has(k)) map.set(k, { label: k, minutes: 0, fieldMinutes: 0, visits: 0 })
    return map.get(k)!
  }
  for (const e of entries.value) {
    const r = row(keyer(true, e))
    r.minutes += e.minutes
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

// By location — the axis the ticket→location relation unlocks. Time and visits
// come from work in range; tickets/installs count tickets created in range.
// The "—" bucket is work with no location set (most reactive tickets).
interface LocRow {
  label: string
  minutes: number
  visits: number
  tickets: number
  installs: number
}
const byLocation = computed<LocRow[]>(() => {
  const map = new Map<string, LocRow>()
  const row = (label: string) => {
    const k = label || '—'
    if (!map.has(k)) map.set(k, { label: k, minutes: 0, visits: 0, tickets: 0, installs: 0 })
    return map.get(k)!
  }
  for (const e of entries.value) row(e.expand?.ticket?.expand?.location?.name || '').minutes += e.minutes
  for (const v of doneVisits.value) row(v.expand?.ticket?.expand?.location?.name || '').visits += 1
  for (const t of tickets.value) {
    const r = row(t.expand?.location?.name || '')
    r.tickets += 1
    if (t.type === 'install') r.installs += 1
  }
  return [...map.values()].sort((a, b) => b.tickets - a.tickets || b.minutes - a.minutes)
})

const totalMinutes = computed(() => entries.value.reduce((s, e) => s + e.minutes, 0))
const totalFieldMinutes = computed(() =>
  entries.value.filter((e) => e.visit).reduce((s, e) => s + e.minutes, 0),
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
    .map(([source, count]) => ({ source, count, pct: totalTickets.value ? Math.round((count / totalTickets.value) * 100) : 0 }))
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
// spreadsheet math), even though the screen shows h/m.
interface Report {
  key: string
  title: string
  filename: string
  header: string[]
  rows: () => (string | number)[][]
}
const reports = computed<Report[]>(() => [
  {
    key: 'staff',
    title: 'By staff / technician',
    filename: 'by-staff',
    header: ['name', 'minutes', 'field_minutes', 'visits'],
    rows: () => byPerson.value.map((r) => [r.label === '—' ? '(unattributed)' : r.label, r.minutes, r.fieldMinutes, r.visits]),
  },
  {
    key: 'customer',
    title: 'By customer',
    filename: 'by-customer',
    header: ['customer', 'minutes', 'field_minutes', 'visits'],
    rows: () => byCustomer.value.map((r) => [r.label === '—' ? '(none)' : r.label, r.minutes, r.fieldMinutes, r.visits]),
  },
  {
    key: 'location',
    title: 'By location',
    filename: 'by-location',
    header: ['location', 'tickets', 'installs', 'minutes', 'visits'],
    rows: () => byLocation.value.map((r) => [r.label === '—' ? '(no location)' : r.label, r.tickets, r.installs, r.minutes, r.visits]),
  },
  {
    key: 'category',
    title: 'Tickets by category',
    filename: 'tickets-by-category',
    header: ['category', 'total', 'open'],
    rows: () => byCategory.value.map((r) => [r.label, r.count, r.open]),
  },
  {
    key: 'source',
    title: 'Tickets by source',
    filename: 'tickets-by-source',
    header: ['source', 'count', 'share_pct'],
    rows: () => bySource.value.map((r) => [r.source, r.count, r.pct]),
  },
])

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
  lines.push('', 'Totals', 'metric,value',
    `time_minutes,${totalMinutes.value}`,
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
  const lines = [['work_date', 'staff', 'customer', 'ticket', 'minutes', 'on_site', 'note'].join(',')]
  for (const e of entries.value) {
    lines.push(
      [
        e.work_date,
        e.expand?.staff?.name || '',
        e.expand?.ticket?.expand?.customer?.name || '',
        e.expand?.ticket?.number ?? '',
        e.minutes,
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
  const lines = [['completed_at', 'technician', 'customer', 'ticket', 'site', 'directions'].join(',')]
  for (const v of doneVisits.value) {
    lines.push(
      [
        v.completed_at || '',
        v.expand?.assignee?.name || '',
        v.expand?.ticket?.expand?.customer?.name || '',
        v.expand?.ticket?.number ?? '',
        v.expand?.ticket?.expand?.location?.name || '',
        v.location || '',
      ]
        .map(csvEscape)
        .join(','),
    )
  }
  download(`visits-detail-${suffix()}.csv`, lines)
}

// Switching to a specific customer drops a location scope it no longer owns
// (clearing retriggers this watcher, which then loads once).
watch([from, to, customerFilter, locationFilter], (cur, prev) => {
  const [, , c] = cur
  const pc = prev[2]
  const l = locationFilter.value
  if (c !== pc && c && l && !locations.value.some((loc) => loc.id === l && loc.customer === c)) {
    locationFilter.value = ''
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
    <div class="flex flex-col sm:flex-row justify-between items-start sm:items-center gap-2">
      <h1 class="text-2xl font-bold">Reports</h1>
      <div class="dropdown dropdown-end">
        <div tabindex="0" role="button" class="btn btn-sm btn-primary">Export ▾</div>
        <ul tabindex="0" class="dropdown-content menu menu-sm bg-base-100 rounded-box shadow-lg border border-base-300 w-60 p-1 z-50">
          <li><a @click="exportAll">All reports (CSV)</a></li>
          <li class="menu-title px-2 pt-2 pb-1 text-xs">Detail rows</li>
          <li><a @click="exportTime">Time entries — detail</a></li>
          <li><a @click="exportVisits">Completed visits — detail</a></li>
        </ul>
      </div>
    </div>

    <!-- Filters: date range + customer/location scope (applies to every rollup). -->
    <div class="flex flex-col sm:flex-row sm:flex-wrap gap-2 sm:items-end">
      <div class="form-control">
        <label class="label py-1"><span class="label-text text-xs">From</span></label>
        <input v-model="from" type="date" class="input input-bordered input-sm" />
      </div>
      <div class="form-control">
        <label class="label py-1"><span class="label-text text-xs">To</span></label>
        <input v-model="to" type="date" class="input input-bordered input-sm" />
      </div>
      <div class="form-control w-full sm:w-52">
        <label class="label py-1"><span class="label-text text-xs">Customer</span></label>
        <SearchSelect v-model="customerFilter" :options="customerOptions" size="sm" empty-label="All customers" placeholder="Any customer…" />
      </div>
      <div class="form-control w-full sm:w-52">
        <label class="label py-1"><span class="label-text text-xs">Location</span></label>
        <SearchSelect v-model="locationFilter" :options="locationOptions" size="sm" empty-label="All locations" placeholder="Any location…" />
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
          <div class="stat-title">Visits completed</div>
          <div class="stat-value text-2xl tabular-nums">{{ totalVisits }}</div>
        </div>
        <div class="stat">
          <div class="stat-title">Tickets created</div>
          <div class="stat-value text-2xl tabular-nums">{{ totalTickets }}</div>
        </div>
      </div>

      <div class="grid grid-cols-1 lg:grid-cols-2 gap-4">
        <!-- By person -->
        <div class="card bg-base-100 shadow-sm">
          <div class="card-body p-4 space-y-2">
            <div class="flex items-center justify-between gap-2">
              <h2 class="font-semibold text-sm">By staff / technician</h2>
              <button class="btn btn-ghost btn-xs" @click="exportOne('staff')">CSV</button>
            </div>
            <div class="overflow-x-auto">
              <table class="table table-sm">
                <thead><tr><th>Name</th><th class="text-right">Time</th><th class="text-right">Field</th><th class="text-right">Visits</th></tr></thead>
                <tbody>
                  <tr v-for="r in byPerson" :key="r.label">
                    <td :class="{ 'text-base-content/50': r.label === '—' }">{{ r.label === '—' ? 'Unattributed' : r.label }}</td>
                    <td class="text-right font-mono tabular-nums">{{ fmtHours(r.minutes) }}</td>
                    <td class="text-right font-mono tabular-nums">{{ fmtHours(r.fieldMinutes) }}</td>
                    <td class="text-right font-mono tabular-nums">{{ r.visits || '—' }}</td>
                  </tr>
                  <tr v-if="byPerson.length === 0"><td colspan="4" class="text-base-content/50">No activity in range.</td></tr>
                </tbody>
              </table>
            </div>
          </div>
        </div>

        <!-- By customer -->
        <div class="card bg-base-100 shadow-sm">
          <div class="card-body p-4 space-y-2">
            <div class="flex items-center justify-between gap-2">
              <h2 class="font-semibold text-sm">By customer</h2>
              <button class="btn btn-ghost btn-xs" @click="exportOne('customer')">CSV</button>
            </div>
            <div class="overflow-x-auto">
              <table class="table table-sm">
                <thead><tr><th>Customer</th><th class="text-right">Time</th><th class="text-right">Field</th><th class="text-right">Visits</th></tr></thead>
                <tbody>
                  <tr v-for="r in byCustomer" :key="r.label">
                    <td :class="{ 'text-base-content/50': r.label === '—' }">{{ r.label === '—' ? 'None' : r.label }}</td>
                    <td class="text-right font-mono tabular-nums">{{ fmtHours(r.minutes) }}</td>
                    <td class="text-right font-mono tabular-nums">{{ fmtHours(r.fieldMinutes) }}</td>
                    <td class="text-right font-mono tabular-nums">{{ r.visits || '—' }}</td>
                  </tr>
                  <tr v-if="byCustomer.length === 0"><td colspan="4" class="text-base-content/50">No activity in range.</td></tr>
                </tbody>
              </table>
            </div>
          </div>
        </div>
      </div>

      <!-- By location: the reporting axis the ticket→location relation adds. -->
      <div class="card bg-base-100 shadow-sm">
        <div class="card-body p-4 space-y-2">
          <div class="flex items-center justify-between gap-2">
            <h2 class="font-semibold text-sm">By location</h2>
            <button class="btn btn-ghost btn-xs" @click="exportOne('location')">CSV</button>
          </div>
          <div class="overflow-x-auto">
            <table class="table table-sm">
              <thead><tr><th>Location</th><th class="text-right">Tickets</th><th class="text-right">Installs</th><th class="text-right">Time</th><th class="text-right">Visits</th></tr></thead>
              <tbody>
                <tr v-for="r in byLocation" :key="r.label">
                  <td :class="{ 'text-base-content/50': r.label === '—' }">{{ r.label === '—' ? 'No location' : r.label }}</td>
                  <td class="text-right font-mono tabular-nums">{{ r.tickets || '—' }}</td>
                  <td class="text-right font-mono tabular-nums">{{ r.installs || '—' }}</td>
                  <td class="text-right font-mono tabular-nums">{{ fmtHours(r.minutes) }}</td>
                  <td class="text-right font-mono tabular-nums">{{ r.visits || '—' }}</td>
                </tr>
                <tr v-if="byLocation.length === 0"><td colspan="5" class="text-base-content/50">No activity in range.</td></tr>
              </tbody>
            </table>
          </div>
        </div>
      </div>

      <!-- Ticket volume: what came in during the range, by category and by
           channel. Counts tickets created in the range. -->
      <div class="grid grid-cols-1 lg:grid-cols-2 gap-4">
        <div class="card bg-base-100 shadow-sm">
          <div class="card-body p-4 space-y-2">
            <div class="flex items-center justify-between gap-2">
              <h2 class="font-semibold text-sm">Tickets by category</h2>
              <button class="btn btn-ghost btn-xs" @click="exportOne('category')">CSV</button>
            </div>
            <div class="overflow-x-auto">
              <table class="table table-sm">
                <thead><tr><th>Category</th><th class="text-right">Total</th><th class="text-right">Open</th></tr></thead>
                <tbody>
                  <tr v-for="r in byCategory" :key="r.label">
                    <td><CategoryBadge v-if="r.label !== 'Uncategorized'" :name="r.label" :color="r.color" /><span v-else class="text-base-content/50">Uncategorized</span></td>
                    <td class="text-right font-mono tabular-nums">{{ r.count }}</td>
                    <td class="text-right font-mono tabular-nums">{{ r.open || '—' }}</td>
                  </tr>
                  <tr v-if="byCategory.length === 0"><td colspan="3" class="text-base-content/50">No tickets in range.</td></tr>
                </tbody>
              </table>
            </div>
          </div>
        </div>

        <div class="card bg-base-100 shadow-sm">
          <div class="card-body p-4 space-y-2">
            <div class="flex items-center justify-between gap-2">
              <h2 class="font-semibold text-sm">Tickets by source</h2>
              <button class="btn btn-ghost btn-xs" @click="exportOne('source')">CSV</button>
            </div>
            <div class="overflow-x-auto">
              <table class="table table-sm">
                <thead><tr><th>Source</th><th class="text-right">Count</th><th class="text-right">Share</th></tr></thead>
                <tbody>
                  <tr v-for="r in bySource" :key="r.source">
                    <td class="capitalize">{{ r.source }}</td>
                    <td class="text-right font-mono tabular-nums">{{ r.count }}</td>
                    <td class="text-right font-mono tabular-nums">{{ r.pct }}%</td>
                  </tr>
                  <tr v-if="bySource.length === 0"><td colspan="3" class="text-base-content/50">No tickets in range.</td></tr>
                </tbody>
              </table>
            </div>
          </div>
        </div>
      </div>
    </template>
  </div>
</template>
