<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { pb } from '@/pb'
import { useAuthStore } from '@/stores/auth'
import type { Customer, Location, Staff, Ticket, TicketCategory, TicketStatus, TicketPriority, TicketType } from '@/types'
import { TICKET_PRIORITIES, TICKET_STATUSES, TICKET_TYPES } from '@/types'
import TicketBadges from '@/components/TicketBadges.vue'
import CategoryBadge from '@/components/CategoryBadge.vue'
import SearchSelect from '@/components/SearchSelect.vue'
import ResponsiveList, { type Column } from '@/components/ResponsiveList.vue'
import Pager from '@/components/Pager.vue'
import { formatDistanceToNow } from 'date-fns'

const route = useRoute()
const router = useRouter()
const auth = useAuthStore()

const tickets = ref<Ticket[]>([])
const customers = ref<Customer[]>([])
const staff = ref<Staff[]>([])
const categories = ref<TicketCategory[]>([])
const locations = ref<Location[]>([])
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
// Field agents care about their own queue first; desk staff see everyone.
const assignee = ref(q('assignee') || (auth.isField ? auth.record?.id || '' : ''))
const category = ref(q('category'))
const location = ref(q('location'))
const type = ref<TicketType | ''>((q('type') as any) || '')
const search = ref(q('search'))

const customerOptions = computed(() => customers.value.map((c) => ({ id: c.id, label: c.name })))
const staffOptions = computed(() => [
  { id: 'unassigned', label: 'Unassigned' },
  ...staff.value.map((s) => ({ id: s.id, label: s.name, sublabel: s.email })),
])
const categoryOptions = computed(() => categories.value.map((c) => ({ id: c.id, label: c.name })))
// Locations span customers here, so disambiguate by customer name in the sublabel.
const locationOptions = computed(() =>
  locations.value.map((l) => ({ id: l.id, label: l.name, sublabel: l.expand?.customer?.name })),
)

const mineActive = computed(() => assignee.value === auth.record?.id)
function toggleMine() {
  assignee.value = mineActive.value ? '' : auth.record?.id || ''
}

// Queue columns: on mobile the card header (card-number slot) already shows
// "#N — title", so the title column is skipped in the card grid.
// Sortable columns map their key straight to a PocketBase sort field, so
// only direct scalar columns are sortable (relation-hop columns like
// customer/assignee are display-only).
const columns: Column<Ticket>[] = [
  { key: 'number', label: '#', class: 'w-16', sortable: true },
  { key: 'title', label: 'Title', hideOnMobile: true },
  { key: 'expand.customer.name', label: 'Customer' },
  { key: 'category', label: 'Category' },
  { key: 'status', label: 'Status', sortable: true },
  { key: 'priority', label: 'Priority', sortable: true },
  { key: 'expand.assignee.name', label: 'Assignee' },
  { key: 'created', label: 'Age', class: 'whitespace-nowrap text-base-content/60', sortable: true, format: (v) => formatDistanceToNow(new Date(v)) },
]

// Sort state → PocketBase sort string. Clicking a column sets it; clicking
// the active column flips direction.
const sortKey = ref('created')
const sortDir = ref<'asc' | 'desc'>('desc')
const buildSort = () => `${sortDir.value === 'desc' ? '-' : ''}${sortKey.value}`
function onSort(key: string) {
  if (sortKey.value === key) sortDir.value = sortDir.value === 'asc' ? 'desc' : 'asc'
  else {
    sortKey.value = key
    sortDir.value = 'desc'
  }
}

// --- bulk selection (v-model:selected on the list; can span pages) ---
const selected = ref<string[]>([])

const bulkBusy = ref(false)
const bulkAssignee = ref('')
// Note: each update fires its normal notification emails (assignment /
// status change) — that's the designed behavior, not a bulk-path special.
async function applyBulk(fields: Record<string, string>) {
  if (selected.value.length === 0) return
  bulkBusy.value = true
  error.value = ''
  const results = await Promise.allSettled(
    selected.value.map((id) => pb.collection('tickets').update(id, fields)),
  )
  const failed = results.filter((r) => r.status === 'rejected').length
  if (failed > 0) error.value = `${failed} of ${results.length} updates failed.`
  selected.value = []
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
      expand: 'customer,assignee,requester,category,location',
    })
    const header = ['number', 'title', 'customer', 'category', 'type', 'location', 'estimated_minutes', 'status', 'priority', 'assignee', 'requester', 'source', 'created', 'updated']
    const lines = [header.join(',')]
    for (const t of rows) {
      lines.push(
        [
          t.number,
          t.title,
          t.expand?.customer?.name || '',
          t.expand?.category?.name || '',
          t.type || '',
          t.expand?.location?.name || '',
          t.estimated_minutes ?? '',
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
  if (category.value) parts.push(`category = '${category.value}'`)
  if (location.value) parts.push(`location = '${location.value}'`)
  if (type.value) parts.push(`type = '${type.value}'`)
  if (assignee.value === 'unassigned') parts.push(`assignee = ''`)
  else if (assignee.value) parts.push(`assignee = '${assignee.value}'`)
  if (search.value.trim()) {
    const raw = search.value.trim()
    const q = raw.replace(/'/g, "\\'")
    const clauses = [
      `title ~ '${q}'`,
      `body ~ '${q}'`,
      `customer.name ~ '${q}'`,
      `requester.name ~ '${q}'`,
      `requester.email ~ '${q}'`,
    ]
    // A bare number matches the ticket number exactly — the most common
    // "pull up #142" lookup.
    if (/^\d+$/.test(raw)) clauses.push(`number = ${raw}`)
    parts.push(`(${clauses.join(' || ')})`)
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
      sort: buildSort(),
      expand: 'customer,assignee,category',
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
    categories.value = await pb.collection('ticket_categories').getFullList<TicketCategory>({ sort: 'sort_order,name', filter: 'active = true' })
    locations.value = await pb.collection('locations').getFullList<Location>({ sort: 'name', expand: 'customer' })
  } catch {
    // Filter dropdowns degrade gracefully; the queue itself still loads.
  }
}

watch([status, priority, customer, category, location, type, assignee, sortKey, sortDir], () => {
  page.value = 1
  // Filter changes drop the selection — bulk-acting on rows that are no
  // longer visible would be a footgun. Paging keeps it (cross-page select).
  selected.value = []
  load()
})

// --- saved views: named filter+sort sets, kept per-browser in localStorage ---
interface SavedView {
  name: string
  status: string
  priority: string
  customer: string
  category: string
  location: string
  type: string
  assignee: string
  search: string
  sortKey: string
  sortDir: 'asc' | 'desc'
}
const SAVED_VIEWS_KEY = 'helpdesk:ticketViews'
const savedViews = ref<SavedView[]>([])
function loadSavedViews() {
  try {
    savedViews.value = JSON.parse(localStorage.getItem(SAVED_VIEWS_KEY) || '[]')
  } catch {
    savedViews.value = []
  }
}
function persistViews() {
  localStorage.setItem(SAVED_VIEWS_KEY, JSON.stringify(savedViews.value))
}
function saveCurrentView() {
  const name = prompt('Name this view')?.trim()
  if (!name) return
  const view: SavedView = {
    name,
    status: status.value,
    priority: priority.value,
    customer: customer.value,
    category: category.value,
    location: location.value,
    type: type.value,
    assignee: assignee.value,
    search: search.value,
    sortKey: sortKey.value,
    sortDir: sortDir.value,
  }
  const i = savedViews.value.findIndex((v) => v.name === name)
  if (i >= 0) savedViews.value[i] = view
  else savedViews.value.push(view)
  persistViews()
}
function applyView(v: SavedView) {
  status.value = v.status as any
  priority.value = v.priority as any
  customer.value = v.customer
  category.value = v.category || ''
  location.value = v.location || ''
  type.value = (v.type as any) || ''
  assignee.value = v.assignee
  search.value = v.search
  sortKey.value = v.sortKey
  sortDir.value = v.sortDir
  // The filter/sort watchers trigger the reload.
}
function deleteView(name: string) {
  savedViews.value = savedViews.value.filter((v) => v.name !== name)
  persistViews()
}

let searchTimer: ReturnType<typeof setTimeout> | undefined
watch(search, () => {
  clearTimeout(searchTimer)
  searchTimer = setTimeout(() => {
    page.value = 1
    // Search is a filter too — same footgun rationale as above.
    selected.value = []
    load()
  }, 300)
})

watch(page, () => load())

let unsubscribe: (() => void) | null = null

onMounted(async () => {
  load()
  loadFilterOptions()
  loadSavedViews()
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

    <div class="flex flex-col sm:flex-row sm:flex-wrap gap-2">
      <input ref="searchEl" v-model="search" type="search" placeholder="Search #, title, customer, requester…  ( / )" class="input input-bordered input-sm w-full sm:w-64" />
      <select v-model="status" class="select select-bordered select-sm w-full sm:w-auto">
        <option value="active">Active</option>
        <option value="">All statuses</option>
        <option v-for="s in TICKET_STATUSES" :key="s" :value="s">{{ s.replace('_', ' ') }}</option>
      </select>
      <select v-model="priority" class="select select-bordered select-sm w-full sm:w-auto">
        <option value="">All priorities</option>
        <option v-for="p in TICKET_PRIORITIES" :key="p" :value="p">{{ p }}</option>
      </select>
      <div class="w-full sm:w-52">
        <SearchSelect v-model="customer" :options="customerOptions" size="sm" empty-label="All customers" placeholder="Customer…" />
      </div>
      <div class="w-full sm:w-52">
        <SearchSelect v-model="category" :options="categoryOptions" size="sm" empty-label="All categories" placeholder="Category…" />
      </div>
      <div class="w-full sm:w-52">
        <SearchSelect v-model="location" :options="locationOptions" size="sm" empty-label="All locations" placeholder="Location…" />
      </div>
      <select v-model="type" class="select select-bordered select-sm w-full sm:w-auto">
        <option value="">All types</option>
        <option v-for="t in TICKET_TYPES" :key="t" :value="t">{{ t }}</option>
      </select>
      <div class="w-full sm:w-52">
        <SearchSelect v-model="assignee" :options="staffOptions" size="sm" empty-label="Anyone" placeholder="Assignee…" />
      </div>
      <div class="flex gap-2">
        <button class="btn btn-sm flex-1 sm:flex-none" :class="mineActive ? 'btn-primary' : 'btn-ghost'" @click="toggleMine">My tickets</button>
        <!-- Saved views + CSV + bulk are desk power-tools; hidden for field. -->
        <template v-if="!auth.isField">
          <!-- Saved views: named filter+sort sets, per-browser. -->
          <div class="dropdown">
            <div tabindex="0" role="button" class="btn btn-sm btn-ghost">Views ▾</div>
            <ul tabindex="0" class="dropdown-content menu menu-sm bg-base-100 rounded-box shadow-lg border border-base-300 w-56 p-1 z-50">
              <li v-for="v in savedViews" :key="v.name">
                <div class="flex items-center justify-between gap-2">
                  <a class="flex-1 truncate" @click="applyView(v)">{{ v.name }}</a>
                  <button class="text-error text-xs" title="Delete view" @click.stop="deleteView(v.name)">✕</button>
                </div>
              </li>
              <li v-if="savedViews.length === 0" class="menu-title px-2 py-1 text-xs">No saved views</li>
              <li><a class="border-t border-base-200 mt-1 pt-1" @click="saveCurrentView">＋ Save current view…</a></li>
            </ul>
          </div>
          <button class="btn btn-sm btn-ghost flex-1 sm:flex-none" :disabled="exporting" @click="exportCsv">
            <span v-if="exporting" class="loading loading-spinner loading-xs"></span>
            Export CSV
          </button>
        </template>
      </div>
    </div>

    <!-- Bulk action bar: appears while rows are selected. -->
    <div v-if="selected.length > 0" class="flex flex-col sm:flex-row sm:flex-wrap sm:items-center gap-2 bg-base-100 rounded-lg shadow-sm px-3 py-2">
      <span class="text-sm font-medium">{{ selected.length }} selected</span>
      <div class="w-full sm:w-52">
        <SearchSelect v-model="bulkAssignee" :options="staffOptions.filter((o) => o.id !== 'unassigned')" size="sm" placeholder="Assign to…" :disabled="bulkBusy" />
      </div>
      <button class="btn btn-sm btn-primary" :disabled="bulkBusy || !bulkAssignee" @click="applyBulk({ assignee: bulkAssignee })">Assign</button>
      <div class="divider divider-horizontal m-0 hidden sm:flex"></div>
      <div class="flex gap-2">
        <button class="btn btn-sm flex-1 sm:flex-none" :disabled="bulkBusy" @click="applyBulk({ status: 'resolved' })">Mark resolved</button>
        <button class="btn btn-sm flex-1 sm:flex-none" :disabled="bulkBusy" @click="applyBulk({ status: 'closed' })">Close</button>
      </div>
      <button class="btn btn-sm btn-ghost sm:ml-auto" :disabled="bulkBusy" @click="selected = []">Clear</button>
      <span v-if="bulkBusy" class="loading loading-spinner loading-sm"></span>
    </div>

    <div v-if="loading" class="flex justify-center p-12"><span class="loading loading-spinner loading-lg"></span></div>
    <div v-else-if="error" class="alert alert-error">{{ error }}</div>

    <ResponsiveList
      v-else
      v-model:selected="selected"
      :items="tickets"
      :columns="columns"
      :sort-key="sortKey"
      :sort-dir="sortDir"
      :selectable="!auth.isField"
      @sort="onSort"
      @row-click="(t) => router.push(`/staff/tickets/${t.id}`)"
    >
      <template #cell-number="{ value }">
        <span class="font-mono text-sm">{{ value }}</span>
      </template>
      <template #cell-title="{ value }">
        <span class="block max-w-md truncate font-medium text-sm">{{ value }}</span>
      </template>
      <template #card-number="{ item }">
        <div class="text-sm font-bold truncate">
          <span class="font-mono text-base-content/60">#{{ item.number }}</span>
          {{ item.title }}
        </div>
      </template>
      <template #cell-category="{ item }">
        <CategoryBadge :name="item.expand?.category?.name" :color="item.expand?.category?.color" />
      </template>
      <template #cell-status="{ value }"><TicketBadges :status="value" /></template>
      <template #cell-priority="{ value }"><TicketBadges :priority="value" /></template>
      <template #empty>
        <span class="text-base-content/60">No tickets match.</span>
      </template>
    </ResponsiveList>

    <Pager v-model:page="page" :total-pages="totalPages" />
  </div>
</template>
