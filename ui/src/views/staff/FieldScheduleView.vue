<script setup lang="ts">
// Field agent's own week/month schedule, locked to my visits. Uses mobile-native
// components (a stacked day agenda + a fits-width dot grid) instead of the desk
// Dispatch boards, so there's no horizontal scroll. Tapping a visit goes to the
// work view; tapping a month day drills into that week.
import { computed, onMounted, onUnmounted, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import { pb } from '@/pb'
import { useAuthStore } from '@/stores/auth'
import type { Visit } from '@/types'
import FieldWeekAgenda from '@/components/FieldWeekAgenda.vue'
import FieldMonthGrid from '@/components/FieldMonthGrid.vue'
import { addDays, addMonths, addWeeks, endOfMonth, endOfWeek, format, startOfMonth, startOfWeek } from 'date-fns'

const router = useRouter()
const auth = useAuthStore()

const visits = ref<Visit[]>([])
const error = ref('')

type ViewMode = 'week' | 'month'
const view = ref<ViewMode>('week')
const focusDate = ref<Date>(new Date())
const weekStart = computed(() => startOfWeek(focusDate.value, { weekStartsOn: 1 }))

const rangeLabel = computed(() =>
  view.value === 'month'
    ? format(focusDate.value, 'MMMM yyyy')
    : `${format(weekStart.value, 'MMM d')} – ${format(addDays(weekStart.value, 6), 'MMM d, yyyy')}`,
)

function shift(dir: -1 | 1) {
  focusDate.value = view.value === 'month' ? addMonths(focusDate.value, dir) : addWeeks(focusDate.value, dir)
}
function goToday() {
  focusDate.value = new Date()
}

const pbTime = (d: Date) => d.toISOString().replace('T', ' ')

function buildFilter(): string {
  const me = auth.record?.id
  const start = view.value === 'month' ? startOfWeek(startOfMonth(focusDate.value), { weekStartsOn: 1 }) : weekStart.value
  const endExclusive =
    view.value === 'month'
      ? addDays(endOfWeek(endOfMonth(focusDate.value), { weekStartsOn: 1 }), 1)
      : addDays(weekStart.value, 7)
  return [
    `assignee = '${me}'`,
    `status != 'requested'`,
    `status != 'canceled'`,
    `scheduled_at >= '${pbTime(start)}'`,
    `scheduled_at < '${pbTime(endExclusive)}'`,
  ].join(' && ')
}

async function load() {
  if (!auth.record?.id) return
  error.value = ''
  try {
    visits.value = await pb.collection('visits').getFullList<Visit>({
      filter: buildFilter(),
      sort: 'scheduled_at',
      expand: 'ticket,ticket.customer',
    })
  } catch (e: any) {
    error.value = e?.message || 'Failed to load schedule'
  }
}

function openVisit(id: string) {
  router.push(`/staff/visits/${id}/work`)
}
function openDay(day: Date) {
  focusDate.value = day
  view.value = 'week'
}

watch([view, focusDate], load)

let reloadTimer: ReturnType<typeof setTimeout> | undefined
let unsub: (() => void) | null = null
function scheduleReload() {
  clearTimeout(reloadTimer)
  reloadTimer = setTimeout(load, 500)
}
onMounted(async () => {
  await load()
  try {
    unsub = await pb.collection('visits').subscribe('*', scheduleReload)
  } catch {
    // fine.
  }
})
onUnmounted(() => {
  clearTimeout(reloadTimer)
  unsub?.()
})
</script>

<template>
  <div class="space-y-4 max-w-2xl mx-auto">
    <div class="flex items-center gap-2">
      <h1 class="text-2xl font-bold mr-auto">Schedule</h1>
      <div class="join">
        <button class="btn btn-sm join-item" :class="view === 'week' ? 'btn-active' : ''" @click="view = 'week'">Week</button>
        <button class="btn btn-sm join-item" :class="view === 'month' ? 'btn-active' : ''" @click="view = 'month'">Month</button>
      </div>
    </div>

    <!-- Shared navigator: prev · Today · next, plus the current range. -->
    <div class="flex items-center gap-2">
      <div class="join">
        <button class="btn btn-sm join-item" aria-label="Previous" @click="shift(-1)">‹</button>
        <button class="btn btn-sm join-item" @click="goToday">Today</button>
        <button class="btn btn-sm join-item" aria-label="Next" @click="shift(1)">›</button>
      </div>
      <span class="font-medium text-sm">{{ rangeLabel }}</span>
    </div>

    <div v-if="error" class="alert alert-error text-sm">{{ error }}</div>

    <FieldWeekAgenda v-if="view === 'week'" :visits="visits" :week-start="weekStart" @select="openVisit" />
    <FieldMonthGrid v-else :visits="visits" :focus-date="focusDate" @open-day="openDay" />
  </div>
</template>
