<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import { pb } from '@/pb'
import { useAuthStore } from '@/stores/auth'
import type { Customer, Location, Staff, Thing, Ticket, TicketCategory, TicketStatus, TicketPriority, TicketType } from '@/types'
import { TICKET_PRIORITIES, TICKET_STATUSES, TICKET_TYPES } from '@/types'
import TicketBadges from '@/components/TicketBadges.vue'
import CategoryBadge from '@/components/CategoryBadge.vue'
import SearchSelect from '@/components/SearchSelect.vue'
import ResponsiveList, { type Column } from '@/components/ResponsiveList.vue'
import Pager from '@/components/Pager.vue'
import { useQuerySync, useQueryValue } from '@/composables/useQuerySync'
import { DUE_BUCKETS, dueClause, formatDay, isPastDue } from '@/due'
import { formatDistanceToNow } from 'date-fns'

const router = useRouter()
const auth = useAuthStore()

const tickets = ref<Ticket[]>([])
const customers = ref<Customer[]>([])
const staff = ref<Staff[]>([])
const categories = ref<TicketCategory[]>([])
const locations = ref<Location[]>([])
const things = ref<Thing[]>([])
const loading = ref(false)
const error = ref('')

const page = ref(1)
const totalPages = ref(1)
const perPage = 30

// Filters. Status defaults to "active" (everything not resolved/closed).
// Initial values may come from the URL query (dashboard tiles link here).
const q = useQueryValue()
const status = ref<'active' | TicketStatus | ''>((q('status') as any) || 'active')
const priority = ref<TicketPriority | ''>((q('priority') as any) || '')
const customer = ref(q('customer'))
// Field agents care about their own queue first; desk staff see everyone.
const assignee = ref(q('assignee') || (auth.isField ? auth.record?.id || '' : ''))
const category = ref(q('category'))
const location = ref(q('location'))
// Deep-linked from a thing's detail card (View all →).
const thing = ref(q('thing'))
const type = ref<TicketType | ''>((q('type') as any) || '')
// Backlog age since created. Deep-linked from the dashboard's Backlog age card.
const age = ref(q('age'))
// Target date. Deep-linked from the dashboard's Due card. Unlike `age` its
// buckets live in @/due, shared with that card so the two provably agree.
const due = ref(q('due'))
const search = ref(q('search'))

// The boundaries mirror the dashboard's Backlog age tiles exactly, so a tile's
// count and the queue it opens agree. Evaluated per query rather than once at
// setup: a queue left open overnight would otherwise keep measuring "2 days"
// from the moment the tab was loaded.
const AGE_BUCKETS = [
  { value: '0-2', label: '0–2 days' },
  { value: '2-7', label: '2–7 days' },
  { value: '7plus', label: 'Over 7 days' },
]
function ageClause(bucket: string): string {
  const ago = (days: number) => new Date(Date.now() - days * 864e5).toISOString().replace('T', ' ')
  switch (bucket) {
    case '0-2':
      return `created >= '${ago(2)}'`
    case '2-7':
      return `created < '${ago(2)}' && created >= '${ago(7)}'`
    case '7plus':
      return `created < '${ago(7)}'`
    default:
      return ''
  }
}

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
// Already customer-scoped by loadThings, so the code disambiguates instead.
const thingOptions = computed(() =>
  things.value.map((t) => ({ id: t.id, label: t.name, sublabel: t.code || undefined })),
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
  // Scalar, so sortable — and worth sorting: "what's due next" is the question
  // a due date exists to answer. Blank for the many tickets that have none.
  { key: 'due_at', label: 'Due', class: 'whitespace-nowrap', sortable: true },
  { key: 'created', label: 'Age', class: 'whitespace-nowrap text-base-content/60', sortable: true, format: (v) => formatDistanceToNow(new Date(v)) },
]

// A due date reads as overdue only while the work is still live: resolved and
// closed tickets are done, however late they were. Mirrors activeDueFilter in
// @/due, which the dashboard counts with; isPastDue is shared so the row colour
// and the filter agree on which day it is.
function isOverdue(t: Ticket): boolean {
  if (t.status === 'resolved' || t.status === 'closed') return false
  return isPastDue(t.due_at)
}

// Sort state → PocketBase sort string. Clicking a column sets it; clicking
// the active column flips direction.
const sortKey = ref(q('sort', 'created'))
const sortDir = ref<'asc' | 'desc'>(q('dir') === 'asc' ? 'asc' : 'desc')

// Sort travels with the filters: a queue shared as "oldest urgent first" that
// arrives newest-first is a different queue. `assignee` is included even though
// its default is dynamic (field techs land on their own work) — the watcher only
// fires on change, so nobody's own id gets written into the URL just by arriving.
useQuerySync(
  { status, priority, customer, category, location, thing, type, age, due, assignee, search, sort: sortKey, dir: sortDir },
  { status: 'active', sort: 'created', dir: 'desc' },
)
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
      expand: 'customer,assignee,requester,category,location,thing',
    })
    const header = ['number', 'title', 'customer', 'category', 'type', 'location', 'thing', 'estimated_minutes', 'status', 'priority', 'assignee', 'requester', 'source', 'due_at', 'created', 'updated']
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
          t.expand?.thing?.name || '',
          t.estimated_minutes ?? '',
          t.status,
          t.priority,
          t.expand?.assignee?.name || '',
          t.expand?.requester?.email || '',
          t.source,
          t.due_at || '',
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
  if (thing.value) parts.push(`thing = '${thing.value}'`)
  if (type.value) parts.push(`type = '${type.value}'`)
  if (age.value) {
    const clause = ageClause(age.value)
    if (clause) parts.push(`(${clause})`)
  }
  if (due.value) {
    const clause = dueClause(due.value)
    if (clause) parts.push(`(${clause})`)
  }
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

// Things are loaded only once a customer is picked. Unlike locations (tens of
// rows, loaded globally), the mirror can run to thousands, and an unscoped
// cross-customer thing picker isn't a useful control anyway — the same gate the
// ticket form applies to its own pickers.
async function loadThings(customerId: string) {
  if (!customerId) {
    things.value = []
    return
  }
  try {
    things.value = await pb.collection('things').getFullList<Thing>({
      filter: `customer = '${customerId}'`,
      sort: 'name',
    })
  } catch {
    // The picker stays empty; the queue is unaffected.
  }
}

// A thing id is meaningless outside its customer, so switching customers clears
// it rather than leaving a filter that matches nothing.
watch(customer, (value) => {
  if (thing.value) thing.value = ''
  loadThings(value)
})

watch([status, priority, customer, category, location, thing, type, age, due, assignee, sortKey, sortDir], () => {
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
  // Optional: views saved before things existed have no such key, which is why
  // applyView defaults every field with `|| ''` rather than assigning directly.
  thing?: string
  type: string
  // Optional for the same reason as `thing` — views saved before the age and
  // due filters existed have no such key.
  age?: string
  due?: string
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
    thing: thing.value,
    type: type.value,
    age: age.value,
    due: due.value,
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
  thing.value = v.thing || ''
  type.value = (v.type as any) || ''
  age.value = v.age || ''
  due.value = v.due || ''
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
  // A deep link may arrive with ?customer=…&thing=… already set; populate the
  // picker so the active filter is visible rather than silently applied.
  if (customer.value) loadThings(customer.value)
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
      <!-- Gated on the customer filter: see loadThings. -->
      <div class="w-full sm:w-52">
        <SearchSelect
          v-model="thing"
          :options="thingOptions"
          size="sm"
          empty-label="All things"
          :placeholder="customer ? 'Thing…' : 'Pick a customer first'"
          :disabled="!customer"
        />
      </div>
      <select v-model="type" class="select select-bordered select-sm w-full sm:w-auto">
        <option value="">All types</option>
        <option v-for="t in TICKET_TYPES" :key="t" :value="t">{{ t }}</option>
      </select>
      <select v-model="age" class="select select-bordered select-sm w-full sm:w-auto">
        <option value="">Any age</option>
        <option v-for="a in AGE_BUCKETS" :key="a.value" :value="a.value">{{ a.label }}</option>
      </select>
      <select v-model="due" class="select select-bordered select-sm w-full sm:w-auto">
        <option value="">Any due date</option>
        <option v-for="d in DUE_BUCKETS" :key="d.value" :value="d.value">{{ d.label }}</option>
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
      <!-- Blank rather than a dash for the many tickets with no due date: a
           column of dashes reads as data. Overdue is coloured, not badged —
           the row already carries status and priority chips. -->
      <template #cell-due_at="{ value, item }">
        <span v-if="value" :class="isOverdue(item) ? 'text-error font-medium' : 'text-base-content/60'">
          {{ formatDay(value) }}
        </span>
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
