<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { pb } from '@/pb'
import { useAuthStore } from '@/stores/auth'
import { useToastStore } from '@/stores/toast'
import { useQuerySync } from '@/composables/useQuerySync'
import type { Location, Thing, Ticket, TicketPriority, TicketStatus } from '@/types'
import { TICKET_PRIORITIES, TICKET_STATUSES } from '@/types'
import TicketBadges from '@/components/TicketBadges.vue'
import ResponsiveList, { type Column } from '@/components/ResponsiveList.vue'
import Pager from '@/components/Pager.vue'
import { formatDistanceToNow } from 'date-fns'

const route = useRoute()
const router = useRouter()
const auth = useAuthStore()
const toast = useToastStore()

// The collection rules already scope this list to the requester's customer;
// we page and filter server-side so a long-lived company's history stays fast.
const columns: Column<Ticket>[] = [
  { key: 'number', label: '#', class: 'w-16' },
  { key: 'title', label: 'Title', hideOnMobile: true },
  { key: 'status', label: 'Status' },
  { key: 'priority', label: 'Priority' },
  { key: 'created', label: 'Age', class: 'whitespace-nowrap text-base-content/60', format: (v) => formatDistanceToNow(new Date(v)) },
]

const tickets = ref<Ticket[]>([])
const loading = ref(true)
const error = ref('')

const page = ref(1)
const totalPages = ref(1)
const perPage = 30

// Filters. Status defaults to "active" (everything not resolved/closed);
// initial values may come from the URL query (dashboard tiles link here).
//
// "All statuses" is the explicit string 'all', not the empty string it reads as
// in the markup. Empty is what an omitted filter looks like both to `q()` and
// to useQuerySync's omit-defaults rule, so an empty sentinel could never
// round-trip through the URL — a "View all N tickets" link off a location or a
// thing would silently land on the active-only default it was written to escape.
const q = (k: string) => (typeof route.query[k] === 'string' ? (route.query[k] as string) : '')
const status = ref<'active' | 'all' | TicketStatus>((q('status') as any) || 'active')
const priority = ref<TicketPriority | ''>((q('priority') as any) || '')
const search = ref('')
const mineOnly = ref(false)
// Tickets whose last public reply was from support — the requester's court.
// Seedable from the dashboard "needs your reply" tile via ?awaiting=1.
const awaitingOnly = ref(q('awaiting') === '1')
// Location / thing, seeded from the query string so "View all →" on a ticket and
// on the Locations page deep-links straight into a filtered history. These are the
// whole reason location and thing became relations — free text could never
// answer "everything at this location" or "everything on this box".
const location = ref(q('location'))
const thing = ref(q('thing'))

// The catalogs backing the two selects; the collection rules scope both to this
// requester's customer. A select is only rendered once its catalog has rows.
const locations = ref<Location[]>([])
const things = ref<Thing[]>([])

// Mirror the filters into the URL so a filtered list can be linked, survives a
// reload, and comes back when you open a ticket out of it and press Back — the
// everyday cost, since reading the list means opening tickets out of it. Same
// composable and same three rules as the staff boards: replace not push,
// defaults omitted, outbound only. `search`, `mineOnly` and page stay out: the
// first is a keystroke stream and the other two are momentary toggles.
// `awaiting` rides the URL as the '1' the dashboard tile already links with,
// not as a raw boolean — String(false) is 'false', which is truthy to the
// omit-defaults rule and would pin ?awaiting=false onto every unfiltered link.
const awaitingParam = computed(() => (awaitingOnly.value ? '1' : ''))
useQuerySync({ status, priority, location, thing, awaiting: awaitingParam }, { status: 'active' })

// Qualified by PARENT, not by address. The names that actually collide are the
// children — "Second Floor" under two different buildings — and the parent is
// the word that separates them. An address is the other obvious qualifier and
// is wrong here: it is long enough to widen the select past the rest of the
// filter row, and every child of one building repeats the same one.
const locationLabel = (l: Location) => (l.expand?.parent?.name ? `${l.name} — ${l.expand.parent.name}` : l.name)
const thingLabel = (t: Thing) => {
  const qualifier = t.code || t.expand?.location?.name
  return qualifier ? `${t.name} (${qualifier})` : t.name
}

function buildFilter(): string {
  const parts: string[] = []
  if (status.value === 'active') parts.push(`status != 'resolved' && status != 'closed'`)
  else if (status.value !== 'all') parts.push(`status = '${status.value}'`)
  if (priority.value) parts.push(`priority = '${priority.value}'`)
  if (awaitingOnly.value) parts.push(`awaiting_requester = true`)
  if (location.value) parts.push(`location = '${location.value}'`)
  if (thing.value) parts.push(`thing = '${thing.value}'`)
  if (mineOnly.value && auth.record?.id) parts.push(`requester = '${auth.record.id}'`)
  if (search.value.trim()) {
    const raw = search.value.trim().replace(/'/g, "\\'")
    parts.push(`(title ~ '${raw}' || body ~ '${raw}')`)
  }
  return parts.join(' && ')
}

// quiet=true refreshes in place without the spinner swap (realtime updates).
async function load(quiet = false) {
  if (!quiet) loading.value = true
  error.value = ''
  try {
    const res = await pb.collection('tickets').getList<Ticket>(page.value, perPage, {
      filter: buildFilter(),
      sort: '-created',
    })
    tickets.value = res.items
    totalPages.value = res.totalPages
  } catch (err: any) {
    error.value = err?.message || 'Failed to load tickets'
  } finally {
    if (!quiet) loading.value = false
  }
}

watch([status, priority, mineOnly, awaitingOnly, location, thing], () => {
  page.value = 1
  load()
})
watch(page, () => load())

let searchTimer: ReturnType<typeof setTimeout> | undefined
watch(search, () => {
  clearTimeout(searchTimer)
  searchTimer = setTimeout(() => {
    page.value = 1
    load()
  }, 300)
})

// --- CSV export of the current filter (all pages) — the portal "report" ---
function csvEscape(v: unknown): string {
  const s = String(v ?? '')
  return /[",\n]/.test(s) ? `"${s.replace(/"/g, '""')}"` : s
}
const exporting = ref(false)
async function exportCsv() {
  exporting.value = true
  try {
    // Only the export expands — the list itself doesn't render location/thing, and
    // an expand on every page load would be paid for nothing.
    const rows = await pb.collection('tickets').getFullList<Ticket>({
      filter: buildFilter(),
      sort: '-created',
      expand: 'location,thing',
    })
    const header = ['number', 'title', 'status', 'priority', 'location', 'thing', 'created', 'updated']
    const lines = [header.join(',')]
    for (const t of rows) {
      const loc = t.expand?.location?.name || t.location_note || ''
      const thg = t.expand?.thing?.name || t.thing_note || ''
      lines.push(
        [t.number, t.title, t.status, t.priority, loc, thg, t.created, t.updated || '']
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
    // Toast, not the inline alert — a finished download shouldn't replace the list.
    toast.success(`Exported ${rows.length} ticket${rows.length === 1 ? '' : 's'}`)
  } catch (err: any) {
    toast.error(err?.message || 'Export failed')
  } finally {
    exporting.value = false
  }
}

let reloadTimer: ReturnType<typeof setTimeout> | undefined
let unsubscribe: (() => void) | null = null

// Re-seed from the URL when only the query changes: vue-router reuses the
// component for /portal/tickets?location=A → ?location=B, so setup() doesn't
// run again and the filter would silently ignore the new deep link.
//
// Every key useQuerySync writes is re-seeded here, and with the SAME defaults
// the initial read uses. Two bugs live in getting that wrong. `status` and
// `awaiting` used to be watched but never assigned, so arriving from the
// dashboard's "Needs reply" tile and then clicking the sidebar's Tickets left
// the toggle on while the URL said otherwise. And a missing default is worse
// than a missing assignment: useQuerySync omits `status=active` from the URL,
// so a bare `q('status')` would re-seed the empty string and filter on
// `status = ''`, which matches nothing.
//
// This does not loop against useQuerySync. Its write lands in the query first,
// so every value read back here is the one already held — and Vue watchers
// only fire on change, so the assignments are no-ops.
watch(
  () => [route.query.location, route.query.thing, route.query.status, route.query.awaiting, route.query.priority],
  () => {
    location.value = q('location')
    thing.value = q('thing')
    status.value = (q('status') as any) || 'active'
    priority.value = (q('priority') as any) || ''
    awaitingOnly.value = q('awaiting') === '1'
  },
)

onMounted(async () => {
  await load()
  // Catalogs are best-effort: a failure just leaves the selects unrendered, and
  // a deep-linked filter still applies because it filters on the id, not a name.
  //
  // Retired things are deliberately INCLUDED here, unlike the intake form which
  // filters them out. Filing a new ticket against decommissioned gear is a
  // mistake; reading the history of it is the whole point of keeping the row.
  const [locs, thgs] = await Promise.allSettled([
    pb.collection('locations').getFullList<Location>({ sort: 'name', expand: 'parent' }),
    pb.collection('things').getFullList<Thing>({ sort: 'name', expand: 'location' }),
  ])
  if (locs.status === 'fulfilled') locations.value = locs.value
  if (thgs.status === 'fulfilled') things.value = thgs.value
  try {
    unsubscribe = await pb.collection('tickets').subscribe('*', () => {
      clearTimeout(reloadTimer)
      reloadTimer = setTimeout(() => load(true), 800)
    })
  } catch {
    // Realtime is progressive enhancement.
  }
})

onUnmounted(() => {
  clearTimeout(reloadTimer)
  unsubscribe?.()
})
</script>

<template>
  <div class="space-y-4">
    <div class="flex flex-col sm:flex-row justify-between items-start sm:items-center gap-2">
      <h1 class="text-2xl font-bold">Tickets</h1>
      <router-link to="/portal/tickets/new" class="btn btn-primary btn-sm w-full sm:w-auto">New Ticket</router-link>
    </div>

    <div class="flex flex-col sm:flex-row sm:flex-wrap gap-2">
      <input v-model="search" type="search" placeholder="Search title or body…" class="input input-bordered input-sm w-full sm:w-64" />
      <select v-model="status" class="select select-bordered select-sm w-full sm:w-auto">
        <option value="active">Active</option>
        <option value="all">All statuses</option>
        <option v-for="s in TICKET_STATUSES" :key="s" :value="s">{{ s.replace('_', ' ') }}</option>
      </select>
      <select v-model="priority" class="select select-bordered select-sm w-full sm:w-auto">
        <option value="">All priorities</option>
        <option v-for="p in TICKET_PRIORITIES" :key="p" :value="p">{{ p }}</option>
      </select>
      <!-- Both options carry a qualifier, because names alone routinely
           collide: a customer with four buildings has four "Front Door"
           readers, and a bare name picks one of them at random as far as the
           reader is concerned. Location falls back to its address, thing to its
           code or the location it sits at. -->
      <select v-if="locations.length" v-model="location" class="select select-bordered select-sm w-full sm:w-auto">
        <option value="">All locations</option>
        <option v-for="l in locations" :key="l.id" :value="l.id">{{ locationLabel(l) }}</option>
      </select>
      <select v-if="things.length" v-model="thing" class="select select-bordered select-sm w-full sm:w-auto">
        <option value="">All things</option>
        <option v-for="t in things" :key="t.id" :value="t.id">{{ thingLabel(t) }}</option>
      </select>
      <div class="flex gap-2">
        <button class="btn btn-sm flex-1 sm:flex-none" :class="awaitingOnly ? 'btn-primary' : 'btn-ghost'" @click="awaitingOnly = !awaitingOnly">
          Needs my reply
        </button>
        <button class="btn btn-sm flex-1 sm:flex-none" :class="mineOnly ? 'btn-primary' : 'btn-ghost'" @click="mineOnly = !mineOnly">
          Created by me
        </button>
        <button class="btn btn-sm btn-ghost flex-1 sm:flex-none" :disabled="exporting || tickets.length === 0" @click="exportCsv">
          <span v-if="exporting" class="loading loading-spinner loading-xs"></span>
          Export CSV
        </button>
      </div>
    </div>

    <div v-if="loading" class="flex justify-center p-12"><span class="loading loading-spinner loading-lg"></span></div>
    <div v-else-if="error" class="alert alert-error">{{ error }}</div>

    <ResponsiveList
      v-else
      :items="tickets"
      :columns="columns"
      @row-click="(t) => router.push(`/portal/tickets/${t.id}`)"
    >
      <template #cell-number="{ value }">
        <span class="font-mono text-sm">{{ value }}</span>
      </template>
      <template #cell-title="{ value, item }">
        <span class="flex items-center gap-2 max-w-md">
          <span class="truncate font-medium text-sm">{{ value }}</span>
          <span v-if="item.awaiting_requester" class="badge-soft badge-soft-info shrink-0 whitespace-nowrap">Needs reply</span>
        </span>
      </template>
      <template #card-number="{ item }">
        <div class="flex items-start gap-2">
          <div class="text-sm font-bold truncate flex-1">
            <span class="font-mono text-base-content/60">#{{ item.number }}</span>
            {{ item.title }}
          </div>
          <span v-if="item.awaiting_requester" class="badge-soft badge-soft-info shrink-0 whitespace-nowrap">Needs reply</span>
        </div>
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
