<script setup lang="ts">
// Field agent's own week/month calendar: the same boards Dispatch uses, but
// locked to my visits, and tapping a chip goes straight to the work view
// instead of the dispatcher drawer.
import { computed, onMounted, onUnmounted, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import { pb } from '@/pb'
import { useAuthStore } from '@/stores/auth'
import type { Staff, Visit } from '@/types'
import VisitWeekCalendar from '@/components/VisitWeekCalendar.vue'
import VisitMonthCalendar from '@/components/VisitMonthCalendar.vue'
import { addDays, addMonths, addWeeks, endOfMonth, endOfWeek, startOfMonth, startOfWeek } from 'date-fns'

const router = useRouter()
const auth = useAuthStore()

const visits = ref<Visit[]>([])
const error = ref('')

type ViewMode = 'week' | 'month'
const view = ref<ViewMode>('week')
const focusDate = ref<Date>(new Date())
const weekStart = computed(() => startOfWeek(focusDate.value, { weekStartsOn: 1 }))

// The week board resolves assignee names from a staff list; one tech here.
const meAsStaff = computed<Staff[]>(() => (auth.record ? [auth.record as Staff] : []))

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
      expand: 'ticket,ticket.customer,ticket.location,assignee',
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
  <div class="space-y-4">
    <div class="flex items-center gap-2">
      <h1 class="text-2xl font-bold mr-auto">Schedule</h1>
      <div class="join">
        <button class="btn btn-sm join-item" :class="view === 'week' ? 'btn-active' : ''" @click="view = 'week'">Week</button>
        <button class="btn btn-sm join-item" :class="view === 'month' ? 'btn-active' : ''" @click="view = 'month'">Month</button>
      </div>
    </div>

    <div v-if="error" class="alert alert-error text-sm">{{ error }}</div>

    <VisitWeekCalendar
      v-if="view === 'week'"
      :visits="visits"
      :week-start="weekStart"
      :staff="meAsStaff"
      @select="openVisit"
      @prev="shift(-1)"
      @next="shift(1)"
      @today="goToday"
    />
    <VisitMonthCalendar
      v-else
      :visits="visits"
      :focus-date="focusDate"
      @select="openVisit"
      @open-day="openDay"
      @prev="shift(-1)"
      @next="shift(1)"
      @today="goToday"
    />
  </div>
</template>
