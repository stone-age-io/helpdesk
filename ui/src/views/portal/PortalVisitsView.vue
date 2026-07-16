<script setup lang="ts">
// Read-only, cross-project view of the requester's on-site visits. Collection
// rules scope `visits` by ticket.customer, so this only ever returns the
// requester's own company's work; we expand ticket + ticket.location but never
// `assignee`, so the MSP technician stays hidden (same roster-hiding as the
// portal project view). Upcoming-first agenda grouped by day; past visits sit
// behind a toggle. Rows drill into the owning ticket — the requester's natural
// detail surface (there's no field work view for them).
import { computed, onMounted, onUnmounted, ref } from 'vue'
import type { Visit, VisitStatus } from '@/types'
import { pb } from '@/pb'
import { format, isSameDay, isToday, isTomorrow, startOfToday } from 'date-fns'

const visits = ref<Visit[]>([])
const loading = ref(true)
const error = ref('')
const showPast = ref(false)

const visitBadge: Record<VisitStatus, string> = {
  requested: 'badge-soft-neutral',
  scheduled: 'badge-soft-info',
  completed: 'badge-soft-success',
  canceled: 'badge-soft-neutral opacity-60',
}

// quiet=true refreshes in place without the spinner swap (realtime updates).
async function load(quiet = false) {
  if (!quiet) loading.value = true
  error.value = ''
  try {
    // Only visits with a real time on them matter to a requester: a scheduled
    // block ahead, or a completed one behind. `requested` (no time yet) and
    // `canceled` are noise here.
    visits.value = await pb.collection('visits').getFullList<Visit>({
      filter: `(status = 'scheduled' || status = 'completed')`,
      sort: 'scheduled_at',
      expand: 'ticket,ticket.location',
    })
  } catch (e: any) {
    error.value = e?.message || 'Failed to load visits'
  } finally {
    if (!quiet) loading.value = false
  }
}

// A visit is "upcoming" if it's still scheduled and dated today or later;
// everything else (completed, or a scheduled block whose day has passed) is past.
const dayStart = startOfToday()
const isUpcoming = (v: Visit) =>
  v.status === 'scheduled' && !!v.scheduled_at && new Date(v.scheduled_at) >= dayStart

const upcomingByDay = computed(() => {
  const groups: { day: Date; items: Visit[] }[] = []
  for (const v of visits.value) {
    if (!isUpcoming(v) || !v.scheduled_at) continue
    const d = new Date(v.scheduled_at)
    const g = groups.find((x) => isSameDay(x.day, d))
    if (g) g.items.push(v)
    else groups.push({ day: d, items: [v] })
  }
  return groups // already time-ascending from the server sort
})

const past = computed(() =>
  visits.value
    .filter((v) => !isUpcoming(v))
    .sort((a, b) => (b.scheduled_at || '').localeCompare(a.scheduled_at || '')),
)

const upcomingCount = computed(() => upcomingByDay.value.reduce((n, g) => n + g.items.length, 0))

const siteOf = (v: Visit) => v.expand?.ticket?.expand?.location?.name || ''
const fmtTime = (v: Visit) => (v.scheduled_at ? format(new Date(v.scheduled_at), 'HH:mm') : '—')
function fmtDayHeader(d: Date): string {
  if (isToday(d)) return `Today · ${format(d, 'EEE, MMM d')}`
  if (isTomorrow(d)) return `Tomorrow · ${format(d, 'EEE, MMM d')}`
  return format(d, 'EEEE, MMM d')
}
const fmtPastWhen = (v: Visit) => (v.scheduled_at ? format(new Date(v.scheduled_at), 'MMM d, yyyy · HH:mm') : '')

let reloadTimer: ReturnType<typeof setTimeout> | undefined
let unsubscribe: (() => void) | null = null

onMounted(async () => {
  await load()
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
  <div class="space-y-6">
    <div>
      <h1 class="text-2xl font-bold">Visits</h1>
      <p class="text-sm text-base-content/60">On-site visits scheduled for your team.</p>
    </div>

    <div v-if="loading" class="flex justify-center p-12"><span class="loading loading-spinner loading-lg"></span></div>
    <div v-else-if="error" class="alert alert-error text-sm">{{ error }}</div>

    <template v-else>
      <!-- Upcoming, grouped by day -->
      <div class="card bg-base-100 shadow-sm">
        <div class="card-body">
          <h2 class="card-title text-base">Upcoming <span class="text-base-content/50 font-normal">({{ upcomingCount }})</span></h2>

          <p v-if="upcomingCount === 0" class="py-2 text-sm text-base-content/50">No upcoming visits scheduled.</p>

          <div v-for="group in upcomingByDay" :key="group.day.toISOString()" class="mt-2 first:mt-0">
            <div class="text-xs font-semibold text-base-content/50 uppercase tracking-wide mb-1">{{ fmtDayHeader(group.day) }}</div>
            <div class="divide-y divide-base-200">
              <router-link
                v-for="v in group.items"
                :key="v.id"
                :to="`/portal/tickets/${v.ticket}`"
                class="flex items-center gap-3 py-2.5 -mx-2 px-2 rounded hover:bg-base-200/50"
              >
                <div class="w-12 shrink-0 text-center font-semibold text-sm">{{ fmtTime(v) }}</div>
                <div class="flex-1 min-w-0">
                  <div class="truncate text-sm font-medium">
                    <span class="font-mono text-xs text-base-content/60">#{{ v.expand?.ticket?.number }}</span>
                    {{ v.expand?.ticket?.title }}
                  </div>
                  <div v-if="siteOf(v)" class="text-xs text-base-content/60 truncate">📍 {{ siteOf(v) }}</div>
                </div>
                <span class="badge-soft" :class="visitBadge[v.status]">{{ v.status }}</span>
              </router-link>
            </div>
          </div>
        </div>
      </div>

      <!-- Past, behind a toggle -->
      <div v-if="past.length" class="card bg-base-100 shadow-sm">
        <div class="card-body">
          <button class="flex items-center justify-between w-full" @click="showPast = !showPast">
            <span class="card-title text-base">Past visits <span class="text-base-content/50 font-normal">({{ past.length }})</span></span>
            <span class="text-base-content/50" aria-hidden="true">{{ showPast ? '▾' : '▸' }}</span>
          </button>

          <div v-if="showPast" class="divide-y divide-base-200 mt-1">
            <router-link
              v-for="v in past"
              :key="v.id"
              :to="`/portal/tickets/${v.ticket}`"
              class="flex items-center gap-3 py-2.5 -mx-2 px-2 rounded hover:bg-base-200/50"
            >
              <div class="flex-1 min-w-0">
                <div class="truncate text-sm font-medium">
                  <span class="font-mono text-xs text-base-content/60">#{{ v.expand?.ticket?.number }}</span>
                  {{ v.expand?.ticket?.title }}
                </div>
                <div class="text-xs text-base-content/60 truncate">
                  {{ fmtPastWhen(v) }}<template v-if="siteOf(v)"> · 📍 {{ siteOf(v) }}</template>
                </div>
              </div>
              <span class="badge-soft" :class="visitBadge[v.status]">{{ v.status }}</span>
            </router-link>
          </div>
        </div>
      </div>
    </template>
  </div>
</template>
