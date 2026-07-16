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
import ResponsiveList, { type Column } from '@/components/ResponsiveList.vue'
import { format, startOfToday } from 'date-fns'

const router = useRouter()

const visits = ref<Visit[]>([])
const locations = ref<Location[]>([])
const loading = ref(true)
const error = ref('')

// Filters — deliberately fewer than Dispatch (no technician/customer: it's one
// customer, and the tech is hidden). `when` defaults to what a requester cares
// about most: what's coming up. `site` only appears for multi-site customers.
type When = 'upcoming' | 'past' | 'all'
const when = ref<When>('upcoming')
const site = ref('')

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
  if (site.value) parts.push(`ticket.location = '${site.value}'`)
  return parts.join(' && ')
}

// quiet=true refreshes in place without the spinner swap (realtime updates).
async function load(quiet = false) {
  if (!quiet) loading.value = true
  error.value = ''
  try {
    visits.value = await pb.collection('visits').getFullList<Visit>({
      filter: buildFilter(),
      // Soonest-first while looking ahead; most-recent-first when looking back.
      sort: when.value === 'upcoming' ? 'scheduled_at' : '-scheduled_at',
      expand: 'ticket,ticket.location',
    })
  } catch (e: any) {
    error.value = e?.message || 'Failed to load visits'
  } finally {
    if (!quiet) loading.value = false
  }
}

const ticketLabel = (v: Visit) => `#${v.expand?.ticket?.number ?? '?'} — ${v.expand?.ticket?.title ?? ''}`
const siteName = (v: Visit) => v.expand?.ticket?.expand?.location?.name || ''
const fmtWhen = (v?: string) => (v ? format(new Date(v), 'EEE, MMM d · HH:mm') : '—')

// Column keys stay dot-free (dots break `#cell-{key}` slot names); ticket/site
// values live on the expanded ticket and resolve through format(_, item).
const columns: Column<Visit>[] = [
  { key: 'ticket', label: 'Ticket', format: (_, item) => ticketLabel(item) },
  { key: 'scheduled_at', label: 'When', class: 'whitespace-nowrap', format: (v) => fmtWhen(v) },
  { key: 'site', label: 'Site', class: 'max-w-40 truncate', format: (_, item) => siteName(item) || '—' },
  { key: 'status', label: 'Status' },
]

const emptyLabel = computed(() =>
  when.value === 'upcoming' ? 'No upcoming visits scheduled.' : when.value === 'past' ? 'No past visits.' : 'No visits yet.',
)

watch([when, site], () => load())

let reloadTimer: ReturnType<typeof setTimeout> | undefined
let unsubscribe: (() => void) | null = null

onMounted(async () => {
  await load()
  // Site options for the filter — scoped to the requester's customer by the
  // locations portal-read rule. Degrades to no site filter on failure.
  try {
    locations.value = await pb.collection('locations').getFullList<Location>({ sort: 'name' })
  } catch {
    // fine — the site filter just won't render.
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
      <select v-if="locations.length > 1" v-model="site" class="select select-bordered select-sm w-full sm:w-auto">
        <option value="">All sites</option>
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
  </div>
</template>
