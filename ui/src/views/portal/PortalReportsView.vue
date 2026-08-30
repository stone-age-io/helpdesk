<script setup lang="ts">
// The customer's own service summary: what they filed, what got done, where,
// and on what — over a date range they choose.
//
// SCOPE IS THE COLLECTION RULES, NOT A CLIENT FILTER. Every query here is
// unqualified by customer because `tickets`, `visits`, `locations` and `things`
// already scope to @request.auth.customer server-side. Nothing on this page
// could be widened by editing the request.
//
// WHAT IT DELIBERATELY DOES NOT SHOW
//   · technicians — same roster-hiding as the portal visit and project views
//   · non-billable time — the hours figure is what they would be invoiced
//     against, never internal rework or goodwill
//   · anything at all about hours unless their customer opted in
//
// HOURS COME FROM ONE SERVER ROUTE, not from time_entries — that collection is
// staff-only and always will be. /api/helpdesk/reports/time-by-ticket returns
// billable minutes per ticket id under exactly the policy the per-ticket
// time-total route already enforces, and answers `enabled: false` when the
// customer has not opted into show_time_to_requester, which is what hides the
// whole section. Rolling those minutes up by location or thing happens here,
// client-side, against tickets this page already loaded — which is why the
// route stays a flat map and grows no grouping logic of its own.
import { computed, onMounted, ref, watch } from 'vue'
import { pb } from '@/pb'
import type { Ticket, Visit } from '@/types'
import CategoryBadge from '@/components/CategoryBadge.vue'
import ReportTable, { type ReportColumn } from '@/components/ReportTable.vue'
import { useQuerySync, useQueryValue } from '@/composables/useQuerySync'

const tickets = ref<Ticket[]>([])
const doneVisits = ref<Visit[]>([])
// Billable minutes per ticket id; empty (and `hoursEnabled` false) unless the
// customer has opted into seeing time.
const minutesByTicket = ref<Record<string, number>>({})
const hoursEnabled = ref(false)
const loading = ref(true)
const error = ref('')

// A quarter, not a month: a customer reads this for a trend, and a single month
// of a quiet location is often two tickets and no visits.
function isoDate(offsetDays: number): string {
  const d = new Date()
  d.setDate(d.getDate() + offsetDays)
  return d.toISOString().slice(0, 10)
}
// The range rides the URL, like every other filtered board in the app. Both
// ends are ALWAYS written rather than omitted at their default — the same
// exception staff Reports takes, and for the same reason: "the trailing 90
// days" means something different the day after you send the link, and this is
// the one portal page whose whole purpose is being sent to somebody.
const q = useQueryValue()
const from = ref(q('from') || isoDate(-90))
const to = ref(q('to') || isoDate(0))
useQuerySync({ from, to })

function pbTime(localDate: string, endOfDay: boolean): string {
  return new Date(`${localDate}T${endOfDay ? '23:59:59' : '00:00:00'}`).toISOString().replace('T', ' ')
}
function rangeFilter(field: string): string {
  return `${field} >= '${pbTime(from.value, false)}' && ${field} <= '${pbTime(to.value, true)}'`
}

async function load() {
  loading.value = true
  error.value = ''
  try {
    const [tix, vis] = await Promise.all([
      pb.collection('tickets').getFullList<Ticket>({
        filter: rangeFilter('created'),
        sort: '-created',
        expand: 'category,location,thing',
      }),
      // `completed_at` is the trustworthy "work actually happened then" stamp —
      // `updated` bumps on any edit (see the visits guard hook).
      pb.collection('visits').getFullList<Visit>({
        filter: `status = 'completed' && ${rangeFilter('completed_at')}`,
        sort: '-completed_at',
        expand: 'ticket,ticket.location,ticket.thing',
      }),
    ])
    tickets.value = tix
    doneVisits.value = vis
    await loadHours()
  } catch (err: any) {
    error.value = err?.message || 'Failed to load your summary'
  } finally {
    loading.value = false
  }
}

// An hours failure is not a page failure: the activity half stands on its own,
// so this degrades to "no hours section" rather than an error banner.
async function loadHours() {
  try {
    const res: any = await pb.send('/api/helpdesk/reports/time-by-ticket', {
      method: 'GET',
      query: { from: from.value, to: to.value },
    })
    hoursEnabled.value = !!res?.enabled
    minutesByTicket.value = res?.minutes || {}
  } catch {
    hoursEnabled.value = false
    minutesByTicket.value = {}
  }
}

// --- totals ---
const totalTickets = computed(() => tickets.value.length)
const openTickets = computed(
  () => tickets.value.filter((t) => t.status !== 'resolved' && t.status !== 'closed').length,
)
const closedTickets = computed(() => totalTickets.value - openTickets.value)
const totalVisits = computed(() => doneVisits.value.length)
// Summed from the route's own response, not from the per-axis rows: the route
// bounded by work_date, so it legitimately includes time logged in range
// against an older ticket that the ticket query (bounded by `created`) misses.
const totalMinutes = computed(() =>
  Object.values(minutesByTicket.value).reduce((s, m) => s + m, 0),
)

// --- axes ---
// Location and thing roll up identically: tickets/planned/open from what was filed
// in range, visits from work completed in range, hours from the per-ticket
// minutes. The "—" bucket is work that named no location / no thing.
//
// Each row carries the relation `id` as well as the label so the table can link
// into /portal/tickets with that filter applied — the same deep link the Locations
// page and the ticket detail use.
interface AxisRow {
  id: string
  label: string
  tickets: number
  planned: number
  open: number
  visits: number
  minutes: number
}
function byTicketAxis(idOf: (t: any) => string, labelOf: (t: any) => string): AxisRow[] {
  const map = new Map<string, AxisRow>()
  const row = (id: string, label: string) => {
    const k = label || '—'
    if (!map.has(k)) map.set(k, { id, label: k, tickets: 0, planned: 0, open: 0, visits: 0, minutes: 0 })
    return map.get(k)!
  }
  for (const t of tickets.value) {
    const r = row(idOf(t), labelOf(t))
    r.tickets += 1
    if (t.type === 'planned') r.planned += 1
    if (t.status !== 'resolved' && t.status !== 'closed') r.open += 1
    r.minutes += minutesByTicket.value[t.id] || 0
  }
  // A completed visit whose ticket falls outside the range still counts toward
  // its location — the work happened there, in range.
  for (const v of doneVisits.value) {
    const t = v.expand?.ticket
    row(idOf(t), labelOf(t)).visits += 1
  }
  // "—" (no location / no thing named) sinks to the bottom however big it is —
  // it is not an answer to "where is our work going".
  return [...map.values()].sort((a, b) => {
    if ((a.label === '—') !== (b.label === '—')) return a.label === '—' ? 1 : -1
    return b.tickets - a.tickets || b.minutes - a.minutes
  })
}

const bySite = computed(() =>
  byTicketAxis((t) => t?.location || '', (t) => t?.expand?.location?.name || ''),
)
const byDevice = computed(() =>
  byTicketAxis((t) => t?.thing || '', (t) => t?.expand?.thing?.name || ''),
)

// Only worth a table when something in it is actually named — a customer with
// no location catalog should not see a "By location" table holding one "No location" row.
const showSites = computed(() => bySite.value.some((r) => r.label !== '—'))
const showDevices = computed(() => byDevice.value.some((r) => r.label !== '—'))

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

function fmtHours(m: number): string {
  if (!m) return '—'
  const h = Math.floor(m / 60)
  return h > 0 ? `${h}h ${m % 60}m` : `${m}m`
}

// --- column specs ---
// Every rollup here renders through the same ReportTable the staff Reports view
// uses, so the two pages format a count, an hours figure and the unattributed
// row identically — and this page stops carrying its own copy of the
// proportional bar, the "—" styling and the numeric alignment. The measure
// carrying `bar` is the one each rollup is sorted by; pointing it anywhere else
// would fight the row order rather than reinforce it.
//
// The hours column is appended, never blanked: a customer whose account has not
// opted into seeing time gets a table with one fewer column, not a column of
// dashes that hints at a figure being withheld.
const H = { numeric: true, hours: true } as const
const N = { numeric: true } as const
function axisColumns(dimension: string): ReportColumn[] {
  return [
    { key: 'label', label: dimension },
    { key: 'tickets', label: 'Tickets', ...N, bar: true },
    { key: 'open', label: 'Open', ...N },
    { key: 'visits', label: 'Visits', ...N },
    ...(hoursEnabled.value ? [{ key: 'minutes', label: 'Billable time', ...H }] : []),
  ]
}
const siteColumns = computed(() => axisColumns('Location'))
const deviceColumns = computed(() => axisColumns('Thing'))
const categoryColumns: ReportColumn[] = [
  { key: 'label', label: 'Category' },
  { key: 'count', label: 'Tickets', ...N, bar: true },
  { key: 'open', label: 'Still open', ...N },
]

// --- CSV ---
// One file, mirroring the staff "Export all": range header, totals, then a
// titled section per table. The hours column is omitted entirely when hours are
// off, rather than exported as a column of zeros.
function csvEscape(v: unknown): string {
  const s = String(v ?? '')
  return /[",\n]/.test(s) ? `"${s.replace(/"/g, '""')}"` : s
}
function exportCsv() {
  const lines: string[] = [
    `Service summary,${from.value} to ${to.value}`,
    '',
    'Totals',
    'metric,value',
    `tickets_opened,${totalTickets.value}`,
    `tickets_still_open,${openTickets.value}`,
    `tickets_closed,${closedTickets.value}`,
    `visits_completed,${totalVisits.value}`,
  ]
  if (hoursEnabled.value) lines.push(`billable_minutes,${totalMinutes.value}`)
  lines.push('')

  const axis = (title: string, rows: AxisRow[], head: string) => {
    lines.push(
      title,
      [head, 'tickets', 'planned', 'still_open', 'visits', ...(hoursEnabled.value ? ['billable_minutes'] : [])].join(','),
    )
    for (const r of rows) {
      lines.push(
        [
          r.label === '—' ? `(no ${head})` : r.label,
          r.tickets,
          r.planned,
          r.open,
          r.visits,
          ...(hoursEnabled.value ? [r.minutes] : []),
        ]
          .map(csvEscape)
          .join(','),
      )
    }
    lines.push('')
  }
  if (showSites.value) axis('By location', bySite.value, 'location')
  if (showDevices.value) axis('By thing', byDevice.value, 'thing')

  lines.push('By category', 'category,tickets,still_open')
  for (const r of byCategory.value) lines.push([r.label, r.count, r.open].map(csvEscape).join(','))

  const blob = new Blob([lines.join('\n')], { type: 'text/csv;charset=utf-8' })
  const a = document.createElement('a')
  a.href = URL.createObjectURL(blob)
  a.download = `service-summary-${from.value}_${to.value}.csv`
  a.click()
  URL.revokeObjectURL(a.href)
}

watch([from, to], load)
onMounted(load)
</script>

<template>
  <div class="space-y-4">
    <div class="flex flex-row justify-between items-center gap-2">
      <h1 class="text-2xl font-bold">Service Summary</h1>
      <button class="btn btn-sm btn-primary" :disabled="loading" @click="exportCsv">Export CSV</button>
    </div>
    <p class="text-sm text-base-content/60">
      What you filed, what we completed, and where — over the range you pick.
    </p>

    <div class="flex flex-col sm:flex-row sm:flex-wrap gap-2 sm:items-end">
      <div class="form-control w-full sm:w-auto">
        <label class="label py-1"><span class="label-text text-xs">From</span></label>
        <input v-model="from" type="date" class="input input-bordered input-sm w-full sm:w-auto" />
      </div>
      <div class="form-control w-full sm:w-auto">
        <label class="label py-1"><span class="label-text text-xs">To</span></label>
        <input v-model="to" type="date" class="input input-bordered input-sm w-full sm:w-auto" />
      </div>
    </div>

    <div v-if="error" class="alert alert-error">{{ error }}</div>
    <div v-if="loading" class="flex justify-center p-12"><span class="loading loading-spinner loading-lg"></span></div>

    <template v-else>
      <div class="stats stats-vertical sm:stats-horizontal shadow-sm bg-base-100 w-full">
        <div class="stat">
          <div class="stat-title">Tickets opened</div>
          <div class="stat-value text-2xl tabular-nums">{{ totalTickets }}</div>
          <div v-if="openTickets" class="stat-desc">{{ openTickets }} still open</div>
        </div>
        <div class="stat">
          <div class="stat-title">Closed out</div>
          <div class="stat-value text-2xl tabular-nums">{{ closedTickets }}</div>
        </div>
        <div class="stat">
          <div class="stat-title">On-site visits</div>
          <div class="stat-value text-2xl tabular-nums">{{ totalVisits }}</div>
        </div>
        <div v-if="hoursEnabled" class="stat">
          <div class="stat-title">Billable time</div>
          <div class="stat-value text-2xl tabular-nums">{{ fmtHours(totalMinutes) }}</div>
          <div class="stat-desc">logged in this range</div>
        </div>
      </div>

      <!-- Stacked, single column — the one place this deliberately parts company
           with staff Reports, which tabs its rollups. That view has seven of
           which six are noise at any moment; this one has three, two of them
           conditional, and a quarterly summary is read top to bottom and
           exported whole. Side by side was also where the heights disagreed. -->
      <div v-if="showSites" class="card bg-base-100 shadow-sm">
        <div class="card-body p-4 space-y-2">
          <h2 class="font-semibold text-sm">By location</h2>
          <ReportTable :columns="siteColumns" :rows="bySite" null-label="No location" empty="Nothing filed in this range.">
            <template #label="{ row }">
              <router-link v-if="row.id" :to="`/portal/tickets?location=${row.id}`" class="link link-hover">{{ row.label }}</router-link>
              <span v-else class="text-base-content/50">No location</span>
            </template>
          </ReportTable>
        </div>
      </div>

      <div v-if="showDevices" class="card bg-base-100 shadow-sm">
        <div class="card-body p-4 space-y-2">
          <h2 class="font-semibold text-sm">By thing</h2>
          <ReportTable :columns="deviceColumns" :rows="byDevice" null-label="No thing" empty="Nothing filed in this range.">
            <template #label="{ row }">
              <router-link v-if="row.id" :to="`/portal/tickets?thing=${row.id}`" class="link link-hover">{{ row.label }}</router-link>
              <span v-else class="text-base-content/50">No thing</span>
            </template>
          </ReportTable>
        </div>
      </div>

      <div class="card bg-base-100 shadow-sm">
        <div class="card-body p-4 space-y-2">
          <h2 class="font-semibold text-sm">By category</h2>
          <ReportTable :columns="categoryColumns" :rows="byCategory" empty="Nothing filed in this range.">
            <template #label="{ row }">
              <CategoryBadge v-if="row.label !== 'Uncategorized'" :name="row.label" :color="row.color" />
              <span v-else class="text-base-content/50">Uncategorized</span>
            </template>
          </ReportTable>
        </div>
      </div>
    </template>
  </div>
</template>
