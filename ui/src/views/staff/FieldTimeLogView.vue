<script setup lang="ts">
// My logged time: the time_entries I recorded, grouped by day with a running
// total. A read-only reflection of the ledger so a field agent can see the
// shift's work without opening each ticket.
import { computed, onMounted, ref } from 'vue'
import { pb } from '@/pb'
import { useAuthStore } from '@/stores/auth'
import type { TimeEntry } from '@/types'
import { format } from 'date-fns'

const auth = useAuthStore()
const entries = ref<TimeEntry[]>([])
const loading = ref(true)
const error = ref('')

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
  for (const e of entries.value) {
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

const grandTotal = computed(() => entries.value.reduce((s, e) => s + e.minutes, 0))

onMounted(load)
</script>

<template>
  <div class="space-y-4">
    <div class="flex items-baseline gap-2">
      <h1 class="text-2xl font-bold mr-auto">My time</h1>
      <span v-if="entries.length" class="text-sm text-base-content/60">{{ fmt(grandTotal) }} total</span>
    </div>

    <div v-if="loading" class="flex justify-center p-12"><span class="loading loading-spinner loading-lg"></span></div>
    <div v-else-if="error" class="alert alert-error text-sm">{{ error }}</div>
    <p v-else-if="!entries.length" class="text-sm text-base-content/50">No time logged yet. Start a timer from a visit to begin.</p>

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
