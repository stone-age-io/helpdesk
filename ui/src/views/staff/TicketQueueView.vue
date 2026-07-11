<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import { pb } from '@/pb'
import { useAuthStore } from '@/stores/auth'
import type { Customer, Staff, Ticket, TicketStatus, TicketPriority } from '@/types'
import { TICKET_PRIORITIES, TICKET_STATUSES } from '@/types'
import TicketBadges from '@/components/TicketBadges.vue'
import SearchSelect from '@/components/SearchSelect.vue'
import { formatDistanceToNow } from 'date-fns'

const route = useRoute()
const auth = useAuthStore()

const tickets = ref<Ticket[]>([])
const customers = ref<Customer[]>([])
const staff = ref<Staff[]>([])
const loading = ref(false)
const error = ref('')

const page = ref(1)
const totalPages = ref(1)
const perPage = 30

// Filters. Status defaults to "active" (everything not resolved/closed).
// Initial values may come from the URL query (dashboard tiles link here).
const q = (k: string) => (typeof route.query[k] === 'string' ? (route.query[k] as string) : '')
const status = ref<'active' | TicketStatus | ''>((q('status') as any) || 'active')
const priority = ref<TicketPriority | ''>((q('priority') as any) || '')
const customer = ref(q('customer'))
const assignee = ref(q('assignee'))
const search = ref('')

const customerOptions = computed(() => customers.value.map((c) => ({ id: c.id, label: c.name })))
const staffOptions = computed(() => [
  { id: 'unassigned', label: 'Unassigned' },
  ...staff.value.map((s) => ({ id: s.id, label: s.name, sublabel: s.email })),
])

const mineActive = computed(() => assignee.value === auth.record?.id)
function toggleMine() {
  assignee.value = mineActive.value ? '' : auth.record?.id || ''
}

// --- bulk selection ---
const selected = ref(new Set<string>())
const allOnPageSelected = computed(
  () => tickets.value.length > 0 && tickets.value.every((t) => selected.value.has(t.id)),
)
function toggleSelect(id: string) {
  selected.value.has(id) ? selected.value.delete(id) : selected.value.add(id)
}
function toggleSelectAll() {
  if (allOnPageSelected.value) selected.value.clear()
  else tickets.value.forEach((t) => selected.value.add(t.id))
}

const bulkBusy = ref(false)
const bulkAssignee = ref('')
// Note: each update fires its normal notification emails (assignment /
// status change) — that's the designed behavior, not a bulk-path special.
async function applyBulk(fields: Record<string, string>) {
  if (selected.value.size === 0) return
  bulkBusy.value = true
  error.value = ''
  const results = await Promise.allSettled(
    [...selected.value].map((id) => pb.collection('tickets').update(id, fields)),
  )
  const failed = results.filter((r) => r.status === 'rejected').length
  if (failed > 0) error.value = `${failed} of ${results.length} updates failed.`
  selected.value.clear()
  bulkAssignee.value = ''
  bulkBusy.value = false
  await load()
}

// --- CSV export of the CURRENT filter (all pages) ---
const exporting = ref(false)
function csvEscape(v: unknown): string {
  const s = String(v ?? '')
  return /[",\n]/.test(s) ? `"${s.replace(/"/g, '""')}"` : s
}
async function exportCsv() {
  exporting.value = true
  error.value = ''
  try {
    const rows = await pb.collection('tickets').getFullList<Ticket>({
      filter: buildFilter(),
      sort: '-created',
      expand: 'customer,assignee,requester',
    })
    const header = ['number', 'title', 'customer', 'status', 'priority', 'assignee', 'requester', 'source', 'created', 'updated']
    const lines = [header.join(',')]
    for (const t of rows) {
      lines.push(
        [
          t.number,
          t.title,
          t.expand?.customer?.name || '',
          t.status,
          t.priority,
          t.expand?.assignee?.name || '',
          t.expand?.requester?.email || '',
          t.source,
          t.created,
          t.updated || '',
        ]
          .map(csvEscape)
          .join(','),
      )
    }
    const blob = new Blob([lines.join('\n')], { type: 'text/csv;charset=utf-8' })
    const a = document.createElement('a')
    a.href = URL.createObjectURL(blob)
    a.download = `tickets-${new Date().toISOString().slice(0, 10)}.csv`
    a.click()
    URL.revokeObjectURL(a.href)
  } catch (err: any) {
    error.value = err?.message || 'Export failed'
  } finally {
    exporting.value = false
  }
}

// --- live updates: any ticket change (agent, portal, NATS, webhook)
// refreshes the visible page after a short collapse window ---
let reloadTimer: ReturnType<typeof setTimeout> | undefined
function scheduleReload() {
  clearTimeout(reloadTimer)
  reloadTimer = setTimeout(() => load(true), 800)
}

// '/' shortcut (StaffLayout) focuses the search box.
const searchEl = ref<HTMLInputElement | null>(null)
function focusSearch() {
  searchEl.value?.focus()
}

function buildFilter(): string {
  const parts: string[] = []
  if (status.value === 'active') parts.push(`status != 'resolved' && status != 'closed'`)
  else if (status.value) parts.push(`status = '${status.value}'`)
  if (priority.value) parts.push(`priority = '${priority.value}'`)
  if (customer.value) parts.push(`customer = '${customer.value}'`)
  if (assignee.value === 'unassigned') parts.push(`assignee = ''`)
  else if (assignee.value) parts.push(`assignee = '${assignee.value}'`)
  if (search.value.trim()) {
    const q = search.value.trim().replace(/'/g, "\\'")
    parts.push(`(title ~ '${q}' || body ~ '${q}')`)
  }
  return parts.join(' && ')
}

// quiet=true refreshes in place without the spinner swap — used by the
// realtime subscription so live updates don't flash the table away.
async function load(quiet = false) {
  if (!quiet) loading.value = true
  error.value = ''
  try {
    const result = await pb.collection('tickets').getList<Ticket>(page.value, perPage, {
      filter: buildFilter(),
      sort: '-created',
      expand: 'customer,assignee',
    })
    tickets.value = result.items
    totalPages.value = result.totalPages
  } catch (err: any) {
    error.value = err?.message || 'Failed to load tickets'
  } finally {
    if (!quiet) loading.value = false
  }
}

async function loadFilterOptions() {
  try {
    customers.value = await pb.collection('customers').getFullList<Customer>({ sort: 'name' })
    staff.value = await pb.collection('staff').getFullList<Staff>({ sort: 'name', filter: 'active = true' })
  } catch {
    // Filter dropdowns degrade gracefully; the queue itself still loads.
  }
}

watch([status, priority, customer, assignee], () => {
  page.value = 1
  // Filter changes drop the selection — bulk-acting on rows that are no
  // longer visible would be a footgun. Paging keeps it (cross-page select).
  selected.value.clear()
  load()
})

let searchTimer: ReturnType<typeof setTimeout> | undefined
watch(search, () => {
  clearTimeout(searchTimer)
  searchTimer = setTimeout(() => {
    page.value = 1
    load()
  }, 300)
})

watch(page, () => load())

let unsubscribe: (() => void) | null = null

onMounted(async () => {
  load()
  loadFilterOptions()
  window.addEventListener('helpdesk:focus-search', focusSearch)
  try {
    unsubscribe = await pb.collection('tickets').subscribe('*', scheduleReload)
  } catch {
    // Realtime is progressive enhancement; the queue works without it.
  }
})

onUnmounted(() => {
  window.removeEventListener('helpdesk:focus-search', focusSearch)
  clearTimeout(reloadTimer)
  unsubscribe?.()
})
</script>

<template>
  <div class="space-y-4">
    <div class="flex flex-col sm:flex-row justify-between items-start sm:items-center gap-2">
      <h1 class="text-2xl font-bold">Tickets</h1>
      <router-link to="/staff/tickets/new" class="btn btn-primary btn-sm w-full sm:w-auto">New Ticket</router-link>
    </div>

    <div class="flex flex-wrap gap-2">
      <input ref="searchEl" v-model="search" type="search" placeholder="Search title or body…  ( / )" class="input input-bordered input-sm w-full sm:w-64" />
      <select v-model="status" class="select select-bordered select-sm">
        <option value="active">Active</option>
        <option value="">All statuses</option>
        <option v-for="s in TICKET_STATUSES" :key="s" :value="s">{{ s.replace('_', ' ') }}</option>
      </select>
      <select v-model="priority" class="select select-bordered select-sm">
        <option value="">All priorities</option>
        <option v-for="p in TICKET_PRIORITIES" :key="p" :value="p">{{ p }}</option>
      </select>
      <div class="w-full sm:w-52">
        <SearchSelect v-model="customer" :options="customerOptions" size="sm" empty-label="All customers" placeholder="Customer…" />
      </div>
      <div class="w-full sm:w-52">
        <SearchSelect v-model="assignee" :options="staffOptions" size="sm" empty-label="Anyone" placeholder="Assignee…" />
      </div>
      <button class="btn btn-sm" :class="mineActive ? 'btn-primary' : 'btn-ghost'" @click="toggleMine">My tickets</button>
      <button class="btn btn-sm btn-ghost" :disabled="exporting" @click="exportCsv">
        <span v-if="exporting" class="loading loading-spinner loading-xs"></span>
        Export CSV
      </button>
    </div>

    <!-- Bulk action bar: appears while rows are selected. -->
    <div v-if="selected.size > 0" class="flex flex-wrap items-center gap-2 bg-base-100 rounded-lg shadow-sm px-3 py-2">
      <span class="text-sm font-medium">{{ selected.size }} selected</span>
      <div class="w-52">
        <SearchSelect v-model="bulkAssignee" :options="staffOptions.filter((o) => o.id !== 'unassigned')" size="sm" placeholder="Assign to…" :disabled="bulkBusy" />
      </div>
      <button class="btn btn-sm btn-primary" :disabled="bulkBusy || !bulkAssignee" @click="applyBulk({ assignee: bulkAssignee })">Assign</button>
      <div class="divider divider-horizontal m-0"></div>
      <button class="btn btn-sm" :disabled="bulkBusy" @click="applyBulk({ status: 'resolved' })">Mark resolved</button>
      <button class="btn btn-sm" :disabled="bulkBusy" @click="applyBulk({ status: 'closed' })">Close</button>
      <button class="btn btn-sm btn-ghost ml-auto" :disabled="bulkBusy" @click="selected.clear()">Clear</button>
      <span v-if="bulkBusy" class="loading loading-spinner loading-sm"></span>
    </div>

    <div v-if="loading" class="flex justify-center p-12"><span class="loading loading-spinner loading-lg"></span></div>
    <div v-else-if="error" class="alert alert-error">{{ error }}</div>
    <div v-else-if="tickets.length === 0" class="text-center p-12 text-base-content/60">No tickets match.</div>

    <div v-else class="overflow-x-auto bg-base-100 rounded-lg shadow-sm">
      <table class="table table-sm">
        <thead>
          <tr>
            <th class="w-8">
              <input type="checkbox" class="checkbox checkbox-xs" :checked="allOnPageSelected" @change="toggleSelectAll" />
            </th>
            <th>#</th>
            <th>Title</th>
            <th>Customer</th>
            <th>Status</th>
            <th>Priority</th>
            <th>Assignee</th>
            <th>Age</th>
          </tr>
        </thead>
        <tbody>
          <tr
            v-for="t in tickets"
            :key="t.id"
            class="hover cursor-pointer"
            @click="$router.push(`/staff/tickets/${t.id}`)"
          >
            <td @click.stop>
              <input type="checkbox" class="checkbox checkbox-xs" :checked="selected.has(t.id)" @change="toggleSelect(t.id)" />
            </td>
            <td class="font-mono">{{ t.number }}</td>
            <td class="max-w-md truncate font-medium">{{ t.title }}</td>
            <td>{{ t.expand?.customer?.name || '—' }}</td>
            <td><TicketBadges :status="t.status" /></td>
            <td><TicketBadges :priority="t.priority" /></td>
            <td>{{ t.expand?.assignee?.name || '—' }}</td>
            <td class="whitespace-nowrap text-base-content/60">{{ formatDistanceToNow(new Date(t.created)) }}</td>
          </tr>
        </tbody>
      </table>
    </div>

    <div v-if="totalPages > 1" class="flex justify-center gap-2">
      <button class="btn btn-sm" :disabled="page <= 1" @click="page--">«</button>
      <span class="btn btn-sm btn-ghost no-animation">{{ page }} / {{ totalPages }}</span>
      <button class="btn btn-sm" :disabled="page >= totalPages" @click="page++">»</button>
    </div>
  </div>
</template>
