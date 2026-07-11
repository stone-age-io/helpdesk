<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { pb } from '@/pb'
import type { TimeEntry, Visit } from '@/types'

// Aggregate the data the app already captures — logged time and completed
// visits — over a date range. No new storage; just rollups the per-ticket
// cards never surfaced. Handy for month-end billing and utilization.

const entries = ref<TimeEntry[]>([])
const doneVisits = ref<Visit[]>([])
const loading = ref(false)
const error = ref('')

// Default to the trailing 30 days.
function isoDate(offsetDays: number): string {
  const d = new Date()
  d.setDate(d.getDate() + offsetDays)
  return d.toISOString().slice(0, 10)
}
const from = ref(isoDate(-30))
const to = ref(isoDate(0))

function pbTime(localDate: string, endOfDay: boolean): string {
  return new Date(`${localDate}T${endOfDay ? '23:59:59' : '00:00:00'}`).toISOString().replace('T', ' ')
}
function rangeFilter(field: string): string {
  return `${field} >= '${pbTime(from.value, false)}' && ${field} <= '${pbTime(to.value, true)}'`
}

async function load() {
  loading.value = true
  error.value = ''
  try {
    ;[entries.value, doneVisits.value] = await Promise.all([
      pb.collection('time_entries').getFullList<TimeEntry>({
        filter: rangeFilter('work_date'),
        sort: '-work_date',
        expand: 'staff,ticket,ticket.customer',
      }),
      pb.collection('visits').getFullList<Visit>({
        filter: `status = 'completed' && ${rangeFilter('completed_at')}`,
        sort: '-completed_at',
        expand: 'assignee,ticket,ticket.customer',
      }),
    ])
  } catch (err: any) {
    error.value = err?.message || 'Failed to load reports'
  } finally {
    loading.value = false
  }
}

// --- rollups ---
interface Row {
  label: string
  minutes: number
  visits: number
}
function group(keyer: (fromEntry: boolean, rec: any) => string): Row[] {
  const map = new Map<string, Row>()
  const row = (label: string) => {
    const k = label || '—'
    if (!map.has(k)) map.set(k, { label: k, minutes: 0, visits: 0 })
    return map.get(k)!
  }
  for (const e of entries.value) row(keyer(true, e)).minutes += e.minutes
  for (const v of doneVisits.value) row(keyer(false, v)).visits += 1
  return [...map.values()].sort((a, b) => b.minutes - a.minutes || b.visits - a.visits)
}

const byPerson = computed(() =>
  group((isEntry, rec) =>
    isEntry ? rec.expand?.staff?.name || '' : rec.expand?.assignee?.name || '',
  ),
)
const byCustomer = computed(() =>
  group((_isEntry, rec) => rec.expand?.ticket?.expand?.customer?.name || ''),
)

const totalMinutes = computed(() => entries.value.reduce((s, e) => s + e.minutes, 0))
const totalVisits = computed(() => doneVisits.value.length)

function fmtHours(m: number): string {
  if (!m) return '—'
  const h = Math.floor(m / 60)
  return h > 0 ? `${h}h ${m % 60}m` : `${m}m`
}

// --- CSV export of the detailed time + visit rows ---
function csvEscape(v: unknown): string {
  const s = String(v ?? '')
  return /[",\n]/.test(s) ? `"${s.replace(/"/g, '""')}"` : s
}
function download(name: string, lines: string[]) {
  const blob = new Blob([lines.join('\n')], { type: 'text/csv;charset=utf-8' })
  const a = document.createElement('a')
  a.href = URL.createObjectURL(blob)
  a.download = name
  a.click()
  URL.revokeObjectURL(a.href)
}
function exportTime() {
  const lines = [['work_date', 'staff', 'customer', 'ticket', 'minutes', 'note'].join(',')]
  for (const e of entries.value) {
    lines.push(
      [
        e.work_date,
        e.expand?.staff?.name || '',
        e.expand?.ticket?.expand?.customer?.name || '',
        e.expand?.ticket?.number ?? '',
        e.minutes,
        e.note || '',
      ]
        .map(csvEscape)
        .join(','),
    )
  }
  download(`time-${from.value}_${to.value}.csv`, lines)
}
function exportVisits() {
  const lines = [['completed_at', 'technician', 'customer', 'ticket', 'location'].join(',')]
  for (const v of doneVisits.value) {
    lines.push(
      [
        v.completed_at || '',
        v.expand?.assignee?.name || '',
        v.expand?.ticket?.expand?.customer?.name || '',
        v.expand?.ticket?.number ?? '',
        v.location || '',
      ]
        .map(csvEscape)
        .join(','),
    )
  }
  download(`completed-visits-${from.value}_${to.value}.csv`, lines)
}

watch([from, to], () => load())
onMounted(load)
</script>

<template>
  <div class="space-y-4">
    <h1 class="text-2xl font-bold">Reports</h1>

    <div class="flex flex-col sm:flex-row sm:flex-wrap gap-2 sm:items-center">
      <label class="text-sm">From</label>
      <input v-model="from" type="date" class="input input-bordered input-sm" />
      <label class="text-sm">To</label>
      <input v-model="to" type="date" class="input input-bordered input-sm" />
    </div>

    <div v-if="error" class="alert alert-error">{{ error }}</div>
    <div v-if="loading" class="flex justify-center p-12"><span class="loading loading-spinner loading-lg"></span></div>

    <template v-else>
      <!-- Totals -->
      <div class="stats stats-vertical sm:stats-horizontal shadow-sm bg-base-100 w-full">
        <div class="stat">
          <div class="stat-title">Time logged</div>
          <div class="stat-value text-2xl">{{ fmtHours(totalMinutes) }}</div>
        </div>
        <div class="stat">
          <div class="stat-title">Visits completed</div>
          <div class="stat-value text-2xl">{{ totalVisits }}</div>
        </div>
      </div>

      <div class="grid grid-cols-1 lg:grid-cols-2 gap-4">
        <!-- By person -->
        <div class="card bg-base-100 shadow-sm">
          <div class="card-body p-4 space-y-2">
            <div class="flex items-center justify-between">
              <h2 class="font-semibold text-sm">By staff / technician</h2>
              <div class="flex gap-1">
                <button class="btn btn-ghost btn-xs" @click="exportTime">Time CSV</button>
                <button class="btn btn-ghost btn-xs" @click="exportVisits">Visits CSV</button>
              </div>
            </div>
            <table class="table table-sm">
              <thead><tr><th>Name</th><th class="text-right">Time</th><th class="text-right">Visits</th></tr></thead>
              <tbody>
                <tr v-for="r in byPerson" :key="r.label">
                  <td>{{ r.label }}</td>
                  <td class="text-right font-mono">{{ fmtHours(r.minutes) }}</td>
                  <td class="text-right font-mono">{{ r.visits || '—' }}</td>
                </tr>
                <tr v-if="byPerson.length === 0"><td colspan="3" class="text-base-content/50">No activity in range.</td></tr>
              </tbody>
            </table>
          </div>
        </div>

        <!-- By customer -->
        <div class="card bg-base-100 shadow-sm">
          <div class="card-body p-4 space-y-2">
            <h2 class="font-semibold text-sm">By customer</h2>
            <table class="table table-sm">
              <thead><tr><th>Customer</th><th class="text-right">Time</th><th class="text-right">Visits</th></tr></thead>
              <tbody>
                <tr v-for="r in byCustomer" :key="r.label">
                  <td>{{ r.label }}</td>
                  <td class="text-right font-mono">{{ fmtHours(r.minutes) }}</td>
                  <td class="text-right font-mono">{{ r.visits || '—' }}</td>
                </tr>
                <tr v-if="byCustomer.length === 0"><td colspan="3" class="text-base-content/50">No activity in range.</td></tr>
              </tbody>
            </table>
          </div>
        </div>
      </div>
    </template>
  </div>
</template>
