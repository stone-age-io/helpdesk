<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import { pb } from '@/pb'
import type { Customer, Staff, Visit, VisitStatus } from '@/types'
import TicketBadges from '@/components/TicketBadges.vue'
import SearchSelect from '@/components/SearchSelect.vue'
import ResponsiveList, { type Column } from '@/components/ResponsiveList.vue'
import VisitDetailDrawer from '@/components/VisitDetailDrawer.vue'
import { format, formatDistanceToNow } from 'date-fns'

const route = useRoute()

const requested = ref<Visit[]>([])
const visits = ref<Visit[]>([])
const staff = ref<Staff[]>([])
const customers = ref<Customer[]>([])
const loading = ref(false)
const error = ref('')

// Filters (initial values may come from the URL query). Requested visits
// live in their own bucket above, so the status filter never includes them.
const q = (k: string) => (typeof route.query[k] === 'string' ? (route.query[k] as string) : '')
const technician = ref(q('technician'))
const customer = ref(q('customer'))
const status = ref<VisitStatus | ''>((q('status') as any) || 'scheduled')
const from = ref(q('from'))
const to = ref(q('to'))

const staffOptions = computed(() => staff.value.map((s) => ({ id: s.id, label: s.name, sublabel: s.email })))
const customerOptions = computed(() => customers.value.map((c) => ({ id: c.id, label: c.name })))

// Ticket priority drives the dispatch order of the requested bucket.
// Sorted client-side: a PocketBase relation-hop sort on a select field
// would order alphabetically (high < low < normal < urgent).
const priorityRank: Record<string, number> = { urgent: 0, high: 1, normal: 2, low: 3 }
const requestedSorted = computed(() =>
  [...requested.value].sort((a, b) => {
    const ra = priorityRank[a.expand?.ticket?.priority] ?? 9
    const rb = priorityRank[b.expand?.ticket?.priority] ?? 9
    return ra !== rb ? ra - rb : a.created.localeCompare(b.created)
  }),
)

// Scheduled visits grouped by local day, chronological within each.
const dayGroups = computed(() => {
  const groups = new Map<string, { label: string; items: Visit[] }>()
  for (const v of visits.value) {
    if (!v.scheduled_at) continue
    const d = new Date(v.scheduled_at)
    const key = format(d, 'yyyy-MM-dd')
    if (!groups.has(key)) groups.set(key, { label: format(d, 'EEEE, MMM d'), items: [] })
    groups.get(key)!.items.push(v)
  }
  return [...groups.entries()].sort(([a], [b]) => a.localeCompare(b)).map(([, g]) => g)
})

// Column keys stay dot-free (dots break `#cell-{key}` slot names); values
// that live on the expanded ticket resolve through format(_, item).
const ticketLabel = (v: Visit) =>
  `#${v.expand?.ticket?.number ?? '?'} — ${v.expand?.ticket?.title ?? ''}`
const customerName = (v: Visit) => v.expand?.ticket?.expand?.customer?.name || '—'

const requestedColumns: Column<Visit>[] = [
  { key: 'ticket', label: 'Ticket', format: (_, item) => ticketLabel(item) },
  { key: 'customer', label: 'Customer', format: (_, item) => customerName(item) },
  { key: 'priority', label: 'Priority' },
  { key: 'location', label: 'Location', class: 'max-w-48 truncate' },
  { key: 'created', label: 'Waiting', class: 'whitespace-nowrap text-base-content/60', format: (v) => formatDistanceToNow(new Date(v)) },
]

function fmtDuration(min?: number): string {
  if (!min) return '—'
  const h = Math.floor(min / 60)
  const m = min % 60
  return h > 0 ? (m ? `${h}h ${m}m` : `${h}h`) : `${m}m`
}

const visitColumns: Column<Visit>[] = [
  { key: 'scheduled_at', label: 'Time', class: 'whitespace-nowrap font-medium', format: (v) => (v ? format(new Date(v), 'HH:mm') : '—') },
  { key: 'duration_minutes', label: 'Duration', class: 'whitespace-nowrap text-base-content/60', format: (v) => fmtDuration(v as number) },
  { key: 'ticket', label: 'Ticket', format: (_, item) => ticketLabel(item) },
  { key: 'customer', label: 'Customer', format: (_, item) => customerName(item) },
  { key: 'assignee', label: 'Technician', format: (_, item) => item.expand?.assignee?.name || '—' },
  { key: 'location', label: 'Location', class: 'max-w-48 truncate' },
  { key: 'completed_at', label: 'Completed', class: 'whitespace-nowrap text-base-content/60', format: (v) => (v ? format(new Date(v), 'MMM d HH:mm') : '—') },
  { key: 'status', label: 'Status' },
]

const statusBadge: Record<string, string> = {
  scheduled: 'badge-info',
  completed: 'badge-success',
  canceled: 'badge-ghost',
}

// PocketBase stores datetimes as "YYYY-MM-DD HH:MM:SS.sssZ"; convert the
// local date-input boundary to that form so string comparison is correct.
function pbTime(localDate: string, endOfDay: boolean): string {
  return new Date(`${localDate}T${endOfDay ? '23:59:59' : '00:00:00'}`).toISOString().replace('T', ' ')
}

function buildFilter(): string {
  const parts: string[] = []
  if (status.value) parts.push(`status = '${status.value}'`)
  else parts.push(`status != 'requested'`)
  if (technician.value) parts.push(`assignee = '${technician.value}'`)
  if (customer.value) parts.push(`ticket.customer = '${customer.value}'`)
  if (from.value) parts.push(`scheduled_at >= '${pbTime(from.value, false)}'`)
  if (to.value) parts.push(`scheduled_at <= '${pbTime(to.value, true)}'`)
  return parts.join(' && ')
}

async function load(quiet = false) {
  if (!quiet) loading.value = true
  error.value = ''
  try {
    ;[requested.value, visits.value] = await Promise.all([
      pb.collection('visits').getFullList<Visit>({
        filter: `status = 'requested'`,
        sort: 'created',
        expand: 'ticket,ticket.customer',
      }),
      pb.collection('visits').getFullList<Visit>({
        filter: buildFilter(),
        sort: 'scheduled_at',
        expand: 'ticket,ticket.customer,assignee',
      }),
    ])
  } catch (err: any) {
    error.value = err?.message || 'Failed to load visits'
  } finally {
    if (!quiet) loading.value = false
  }
}

async function loadFilterOptions() {
  try {
    staff.value = await pb.collection('staff').getFullList<Staff>({ sort: 'name', filter: 'active = true' })
    customers.value = await pb.collection('customers').getFullList<Customer>({ sort: 'name' })
  } catch {
    // Filter dropdowns degrade gracefully; the lists still load.
  }
}

// --- CSV export of the current visit filter (all matching rows) ---
const exporting = ref(false)
function csvEscape(v: unknown): string {
  const s = String(v ?? '')
  return /[",\n]/.test(s) ? `"${s.replace(/"/g, '""')}"` : s
}
function exportCsv() {
  exporting.value = true
  try {
    const header = ['ticket', 'customer', 'technician', 'scheduled_at', 'duration_minutes', 'completed_at', 'status', 'location', 'notes']
    const lines = [header.join(',')]
    for (const v of visits.value) {
      lines.push(
        [
          v.expand?.ticket?.number ?? '',
          customerName(v),
          v.expand?.assignee?.name || '',
          v.scheduled_at || '',
          v.duration_minutes ?? '',
          v.completed_at || '',
          v.status,
          v.location || '',
          v.notes || '',
        ]
          .map(csvEscape)
          .join(','),
      )
    }
    const blob = new Blob([lines.join('\n')], { type: 'text/csv;charset=utf-8' })
    const a = document.createElement('a')
    a.href = URL.createObjectURL(blob)
    a.download = `visits-${new Date().toISOString().slice(0, 10)}.csv`
    a.click()
    URL.revokeObjectURL(a.href)
  } finally {
    exporting.value = false
  }
}

// Rows open the visit drawer — a requested visit is scheduled there, a
// scheduled one rescheduled/completed, all without leaving the board.
const openVisitId = ref<string | null>(null)

watch([technician, customer, status, from, to], () => load())

// Live updates: visit changes anywhere (ticket card, this view, another
// agent) refresh both lists after a short collapse window.
let reloadTimer: ReturnType<typeof setTimeout> | undefined
function scheduleReload() {
  clearTimeout(reloadTimer)
  reloadTimer = setTimeout(() => load(true), 800)
}

let unsubscribe: (() => void) | null = null

onMounted(async () => {
  load()
  loadFilterOptions()
  try {
    unsubscribe = await pb.collection('visits').subscribe('*', scheduleReload)
  } catch {
    // Realtime is progressive enhancement; the view works without it.
  }
})

onUnmounted(() => {
  clearTimeout(reloadTimer)
  unsubscribe?.()
})
</script>

<template>
  <div class="space-y-4">
    <h1 class="text-2xl font-bold">Dispatch</h1>

    <div v-if="error" class="alert alert-error">{{ error }}</div>
    <div v-if="loading" class="flex justify-center p-12"><span class="loading loading-spinner loading-lg"></span></div>

    <template v-else>
      <!-- Visits an agent promoted but nobody has scheduled yet: the
           dispatcher's inbox, ordered by ticket priority then age. Click a
           row to schedule it in place. -->
      <section class="space-y-2">
        <h2 class="font-semibold text-sm uppercase tracking-wide text-base-content/60">
          Needs scheduling
          <span v-if="requestedSorted.length" class="badge badge-warning badge-sm align-middle">{{ requestedSorted.length }}</span>
        </h2>
        <ResponsiveList v-if="requestedSorted.length" :items="requestedSorted" :columns="requestedColumns" @row-click="(v: Visit) => (openVisitId = v.id)">
          <template #cell-priority="{ item }"><TicketBadges :priority="item.expand?.ticket?.priority" /></template>
        </ResponsiveList>
        <p v-else class="text-sm text-base-content/50">Nothing waiting on a dispatcher.</p>
      </section>

      <!-- Scheduled work, grouped by day. -->
      <section class="space-y-2">
        <div class="flex flex-col sm:flex-row sm:flex-wrap gap-2 sm:items-center">
          <h2 class="font-semibold text-sm uppercase tracking-wide text-base-content/60 sm:mr-auto">Visits</h2>
          <div class="w-full sm:w-52">
            <SearchSelect v-model="technician" :options="staffOptions" size="sm" empty-label="All technicians" placeholder="Technician…" />
          </div>
          <div class="w-full sm:w-52">
            <SearchSelect v-model="customer" :options="customerOptions" size="sm" empty-label="All customers" placeholder="Customer…" />
          </div>
          <select v-model="status" class="select select-bordered select-sm w-full sm:w-auto">
            <option value="scheduled">Scheduled</option>
            <option value="completed">Completed</option>
            <option value="canceled">Canceled</option>
            <option value="">All</option>
          </select>
          <input v-model="from" type="date" class="input input-bordered input-sm w-full sm:w-auto" title="From" />
          <input v-model="to" type="date" class="input input-bordered input-sm w-full sm:w-auto" title="To" />
          <button class="btn btn-sm btn-ghost w-full sm:w-auto" :disabled="exporting || visits.length === 0" @click="exportCsv">
            <span v-if="exporting" class="loading loading-spinner loading-xs"></span>
            Export CSV
          </button>
        </div>

        <p v-if="dayGroups.length === 0" class="text-sm text-base-content/50">No visits match.</p>
        <div v-for="group in dayGroups" :key="group.label" class="space-y-1">
          <h3 class="font-medium text-sm">{{ group.label }}</h3>
          <ResponsiveList :items="group.items" :columns="visitColumns" @row-click="(v: Visit) => (openVisitId = v.id)">
            <template #card-scheduled_at="{ item }">
              <div class="text-sm font-bold truncate">
                {{ item.scheduled_at ? format(new Date(item.scheduled_at), 'HH:mm') : '' }}
                <span class="font-mono text-base-content/60">#{{ item.expand?.ticket?.number }}</span>
                {{ item.expand?.ticket?.title }}
              </div>
            </template>
            <template #cell-status="{ value }">
              <span class="badge badge-sm" :class="statusBadge[value]">{{ value }}</span>
            </template>
          </ResponsiveList>
        </div>
      </section>
    </template>

    <VisitDetailDrawer :visit-id="openVisitId" :staff="staff" @close="openVisitId = null" @changed="load" />
  </div>
</template>
