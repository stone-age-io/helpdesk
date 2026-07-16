<script setup lang="ts">
// My logged time: the time_entries I recorded, grouped by day with a running
// total. A read-only reflection of the ledger so a field agent can see the
// shift's work without opening each ticket.
import { computed, onMounted, ref } from 'vue'
import { pb } from '@/pb'
import { useAuthStore } from '@/stores/auth'
import type { TimeEntry } from '@/types'
import { format, startOfMonth, startOfWeek } from 'date-fns'

const auth = useAuthStore()
const entries = ref<TimeEntry[]>([])
const loading = ref(true)
const error = ref('')

// Default to this week — a field agent thinks in shifts/pay periods, not
// all-time. Filtered client-side over the loaded list (MSP-scale volumes).
type Range = 'week' | 'month' | 'all'
const range = ref<Range>('week')
const rangeStart = computed<Date | null>(() => {
  if (range.value === 'all') return null
  const now = new Date()
  return range.value === 'week' ? startOfWeek(now, { weekStartsOn: 1 }) : startOfMonth(now)
})
const inRange = computed(() => {
  const start = rangeStart.value
  if (!start) return entries.value
  return entries.value.filter((e) => e.work_date && new Date(e.work_date) >= start)
})

async function load() {
  const me = auth.record?.id
  if (!me) return
  error.value = ''
  try {
    entries.value = await pb.collection('time_entries').getFullList<TimeEntry>({
      filter: `staff = '${me}'`,
      sort: '-work_date',
      expand: 'ticket',
    })
  } catch (e: any) {
    error.value = e?.message || 'Failed to load time'
  } finally {
    loading.value = false
  }
}

function fmt(min: number): string {
  const h = Math.floor(min / 60)
  return h > 0 ? `${h}h ${min % 60}m` : `${min}m`
}

// Group by local day of work_date, newest first.
const groups = computed(() => {
  const m = new Map<string, { label: string; items: TimeEntry[]; total: number }>()
  for (const e of inRange.value) {
    if (!e.work_date) continue
    const d = new Date(e.work_date)
    const key = format(d, 'yyyy-MM-dd')
    if (!m.has(key)) m.set(key, { label: format(d, 'EEEE, MMM d'), items: [], total: 0 })
    const g = m.get(key)!
    g.items.push(e)
    g.total += e.minutes
  }
  return [...m.entries()].sort(([a], [b]) => b.localeCompare(a)).map(([, g]) => g)
})

const grandTotal = computed(() => inRange.value.reduce((s, e) => s + e.minutes, 0))

onMounted(load)
</script>

<template>
  <div class="space-y-4">
    <div class="flex flex-wrap items-baseline gap-2">
      <h1 class="text-2xl font-bold mr-auto">My time</h1>
      <span class="text-sm text-base-content/60">{{ fmt(grandTotal) }} total</span>
    </div>

    <div class="join">
      <button class="btn btn-xs join-item" :class="range === 'week' ? 'btn-active' : ''" @click="range = 'week'">This week</button>
      <button class="btn btn-xs join-item" :class="range === 'month' ? 'btn-active' : ''" @click="range = 'month'">This month</button>
      <button class="btn btn-xs join-item" :class="range === 'all' ? 'btn-active' : ''" @click="range = 'all'">All</button>
    </div>

    <div v-if="loading" class="flex justify-center p-12"><span class="loading loading-spinner loading-lg"></span></div>
    <div v-else-if="error" class="alert alert-error text-sm">{{ error }}</div>
    <p v-else-if="!groups.length" class="text-sm text-base-content/50">
      {{ entries.length ? 'No time logged in this range.' : 'No time logged yet. Start a timer from a visit to begin.' }}
    </p>

    <template v-else>
      <section v-for="g in groups" :key="g.label" class="space-y-1">
        <div class="flex items-baseline gap-2">
          <h2 class="font-medium text-sm">{{ g.label }}</h2>
          <span class="text-xs text-base-content/50 ml-auto font-mono">{{ fmt(g.total) }}</span>
        </div>
        <ul class="rounded-box border border-base-300 divide-y divide-base-200 bg-base-100">
          <li v-for="e in g.items" :key="e.id" class="flex items-center gap-2 p-2 text-sm">
            <span class="font-mono text-base-content/50 shrink-0">#{{ e.expand?.ticket?.number ?? '—' }}</span>
            <span class="flex-1 truncate" :title="e.note">{{ e.note || e.expand?.ticket?.title || '' }}</span>
            <span class="font-mono whitespace-nowrap">{{ fmt(e.minutes) }}</span>
          </li>
        </ul>
      </section>
    </template>
  </div>
</template>
