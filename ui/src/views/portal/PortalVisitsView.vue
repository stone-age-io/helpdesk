<script setup lang="ts">
// Read-only, cross-project view of the requester's on-site visits, as a single
// filtered ResponsiveList (the Dispatch board, trimmed to what a single-customer
// requester needs). Collection rules scope `visits` by ticket.customer, so this
// only ever returns their own company's work; we expand ticket + ticket.location
// but never `assignee`, so the MSP technician stays hidden (same roster-hiding as
// the portal project view). Rows drill into the owning ticket — the requester's
// natural detail surface (there's no field work view for them).
import { computed, onMounted, onUnmounted, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import type { Location, Visit, VisitStatus } from '@/types'
import { pb } from '@/pb'
import { useQuerySync, useQueryValue } from '@/composables/useQuerySync'
import ResponsiveList, { type Column } from '@/components/ResponsiveList.vue'
import Pager from '@/components/Pager.vue'
import { format, startOfToday } from 'date-fns'

const router = useRouter()
const q = useQueryValue()

const visits = ref<Visit[]>([])
const locations = ref<Location[]>([])
const loading = ref(true)
const error = ref('')

// Paged rather than a full list. "Upcoming" is bounded by the dispatch board,
// but Past and All are the whole history of a long-lived customer — the same
// reason the ticket list next door pages.
const page = ref(1)
const totalPages = ref(1)
const perPage = 50

// Filters — deliberately fewer than Dispatch (no technician/customer: it's one
// customer, and the tech is hidden). `when` defaults to what a requester cares
// about most: what's coming up. `location` only appears for multi-location customers.
type When = 'upcoming' | 'past' | 'all'
const when = ref<When>((q('when') as When) || 'upcoming')
// Seedable from the Locations page and the location detail, whose "Visits →"
// links land here.
const location = ref(q('location'))

// Both filters ride the URL, so a filtered board can be linked, survives a
// reload, and comes back when you open a ticket out of it and press Back —
// the same composable and rules as the staff boards. `when` declares its
// default so the everyday view produces a bare /portal/visits link.
useQuerySync({ when, location }, { when: 'upcoming' })

const statusBadge: Record<VisitStatus, string> = {
  requested: 'badge-soft-neutral',
  scheduled: 'badge-soft-info',
  completed: 'badge-soft-success',
  canceled: 'badge-soft-neutral opacity-60',
}

// Only visits with a real time on them matter to a requester: a scheduled block
// ahead, or a completed one behind. `requested` (no time yet) and `canceled` are
// noise. "Upcoming" = still scheduled and dated today-or-later; "past" = the rest.
function buildFilter(): string {
  const parts = [`(status = 'scheduled' || status = 'completed')`]
  const todayStart = startOfToday().toISOString().replace('T', ' ')
  if (when.value === 'upcoming') parts.push(`status = 'scheduled'`, `scheduled_at >= '${todayStart}'`)
  else if (when.value === 'past') parts.push(`(status = 'completed' || scheduled_at < '${todayStart}')`)
  if (location.value) parts.push(`ticket.location = '${location.value}'`)
  return parts.join(' && ')
}

// quiet=true refreshes in place without the spinner swap (realtime updates).
async function load(quiet = false) {
  if (!quiet) loading.value = true
  error.value = ''
  try {
    const res = await pb.collection('visits').getList<Visit>(page.value, perPage, {
      filter: buildFilter(),
      // Soonest-first while looking ahead; most-recent-first when looking back.
      sort: when.value === 'upcoming' ? 'scheduled_at' : '-scheduled_at',
      expand: 'ticket,ticket.location',
    })
    visits.value = res.items
    totalPages.value = res.totalPages
  } catch (e: any) {
    error.value = e?.message || 'Failed to load visits'
  } finally {
    if (!quiet) loading.value = false
  }
}

const ticketLabel = (v: Visit) => `#${v.expand?.ticket?.number ?? '?'} — ${v.expand?.ticket?.title ?? ''}`
const locationName = (v: Visit) => v.expand?.ticket?.expand?.location?.name || ''
const fmtWhen = (v?: string) => (v ? format(new Date(v), 'EEE, MMM d · HH:mm') : '—')

// Column keys stay dot-free (dots break `#cell-{key}` slot names); ticket/location
// values live on the expanded ticket and resolve through format(_, item).
const columns: Column<Visit>[] = [
  { key: 'ticket', label: 'Ticket', format: (_, item) => ticketLabel(item) },
  { key: 'scheduled_at', label: 'When', class: 'whitespace-nowrap', format: (v) => fmtWhen(v) },
  { key: 'location', label: 'Location', class: 'max-w-40 truncate', format: (_, item) => locationName(item) || '—' },
  { key: 'status', label: 'Status' },
]

const emptyLabel = computed(() =>
  when.value === 'upcoming' ? 'No upcoming visits scheduled.' : when.value === 'past' ? 'No past visits.' : 'No visits yet.',
)

watch([when, location], () => {
  page.value = 1
  load()
})
watch(page, () => load())

let reloadTimer: ReturnType<typeof setTimeout> | undefined
let unsubscribe: (() => void) | null = null

onMounted(async () => {
  await load()
  // Location options for the filter — scoped to the requester's customer by the
  // locations portal-read rule. Degrades to no location filter on failure.
  try {
    locations.value = await pb.collection('locations').getFullList<Location>({ sort: 'name' })
  } catch {
    // fine — the location filter just won't render.
  }
  try {
    unsubscribe = await pb.collection('visits').subscribe('*', () => {
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
    <div>
      <h1 class="text-2xl font-bold">Visits</h1>
      <p class="text-sm text-base-content/60">On-site visits scheduled for your team.</p>
    </div>

    <div class="flex flex-col sm:flex-row sm:flex-wrap gap-2 sm:items-center">
      <div class="join">
        <button class="btn btn-sm join-item" :class="when === 'upcoming' ? 'btn-active' : ''" @click="when = 'upcoming'">Upcoming</button>
        <button class="btn btn-sm join-item" :class="when === 'past' ? 'btn-active' : ''" @click="when = 'past'">Past</button>
        <button class="btn btn-sm join-item" :class="when === 'all' ? 'btn-active' : ''" @click="when = 'all'">All</button>
      </div>
      <select v-if="locations.length > 1" v-model="location" class="select select-bordered select-sm w-full sm:w-auto">
        <option value="">All locations</option>
        <option v-for="l in locations" :key="l.id" :value="l.id">{{ l.name }}</option>
      </select>
    </div>

    <div v-if="loading" class="flex justify-center p-12"><span class="loading loading-spinner loading-lg"></span></div>
    <div v-else-if="error" class="alert alert-error text-sm">{{ error }}</div>

    <ResponsiveList v-else :items="visits" :columns="columns" @row-click="(v: Visit) => router.push(`/portal/tickets/${v.ticket}`)">
      <template #cell-ticket="{ item }">
        <span class="text-sm"><span class="font-mono text-base-content/60">#{{ item.expand?.ticket?.number }}</span> {{ item.expand?.ticket?.title }}</span>
      </template>
      <template #card-ticket="{ item }">
        <div class="text-sm font-bold truncate">
          <span class="font-mono text-base-content/60">#{{ item.expand?.ticket?.number }}</span> {{ item.expand?.ticket?.title }}
        </div>
      </template>
      <template #cell-status="{ value }"><span class="badge-soft" :class="statusBadge[value as VisitStatus]">{{ value }}</span></template>
      <template #empty><span class="text-base-content/60">{{ emptyLabel }}</span></template>
    </ResponsiveList>

    <Pager v-model:page="page" :total-pages="totalPages" />
  </div>
</template>
