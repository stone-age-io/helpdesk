<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { pb } from '@/pb'
import { useAuthStore } from '@/stores/auth'
import type { TimeEntry, Visit } from '@/types'
import { format } from 'date-fns'

const props = defineProps<{ ticketId: string }>()
const auth = useAuthStore()

const entries = ref<TimeEntry[]>([])
const visits = ref<Visit[]>([])
const minutes = ref<number | null>(null)
const note = ref('')
const visitId = ref('') // '' = desk work
const saving = ref(false)
const error = ref('')

const totalMinutes = computed(() => entries.value.reduce((sum, e) => sum + e.minutes, 0))
// Field time = anything attributed to a visit; the rest is desk work. Both
// still roll up to totalMinutes — the split is just a lens on the same ledger.
const fieldMinutes = computed(() =>
  entries.value.filter((e) => e.visit).reduce((sum, e) => sum + e.minutes, 0),
)

function fmtTotal(m: number): string {
  const h = Math.floor(m / 60)
  return h > 0 ? `${h}h ${m % 60}m` : `${m}m`
}

// A visit is worth tagging time to unless it was canceled.
function visitLabel(v: Visit): string {
  const when = v.scheduled_at ? format(new Date(v.scheduled_at), 'MMM d') : 'unscheduled'
  return `${when} · ${v.status}`
}

async function load() {
  try {
    entries.value = await pb.collection('time_entries').getFullList<TimeEntry>({
      filter: `ticket = '${props.ticketId}'`,
      sort: '-work_date',
      expand: 'staff',
    })
    visits.value = await pb.collection('visits').getFullList<Visit>({
      filter: `ticket = '${props.ticketId}' && status != 'canceled'`,
      sort: 'scheduled_at',
    })
  } catch {
    // Card is optional context; the ticket view stays usable.
  }
}

async function add() {
  if (!minutes.value || minutes.value < 1) return
  saving.value = true
  error.value = ''
  try {
    await pb.collection('time_entries').create({
      ticket: props.ticketId,
      staff: auth.record?.id,
      minutes: minutes.value,
      work_date: new Date().toISOString(),
      note: note.value.trim(),
      visit: visitId.value || null,
    })
    minutes.value = null
    note.value = ''
    visitId.value = ''
    await load()
  } catch (err: any) {
    error.value = err?.message || 'Failed to log time'
  } finally {
    saving.value = false
  }
}

async function remove(entry: TimeEntry) {
  try {
    await pb.collection('time_entries').delete(entry.id)
    await load()
  } catch (err: any) {
    error.value = err?.message || 'Failed to delete entry'
  }
}

onMounted(load)
</script>

<template>
  <div class="card bg-base-100 shadow-sm">
    <div class="card-body py-4 px-4 space-y-2">
      <div class="flex justify-between items-center">
        <h2 class="font-semibold text-sm">Time</h2>
        <div class="flex items-center gap-1">
          <span v-if="fieldMinutes > 0" class="badge badge-ghost badge-sm">{{ fmtTotal(fieldMinutes) }} field</span>
          <span class="badge badge-ghost badge-sm">{{ fmtTotal(totalMinutes) }}</span>
        </div>
      </div>

      <div v-if="error" class="text-error text-xs">{{ error }}</div>

      <ul class="space-y-1">
        <li v-for="e in entries" :key="e.id" class="flex items-center justify-between text-sm gap-2">
          <span class="text-base-content/70 whitespace-nowrap">{{ format(new Date(e.work_date), 'MMM d') }}</span>
          <span class="flex-1 truncate" :title="e.note">
            <span v-if="e.visit" title="on-site">📍</span>{{ e.note || e.expand?.staff?.name || '' }}
          </span>
          <span class="font-mono whitespace-nowrap">{{ fmtTotal(e.minutes) }}</span>
          <button
            v-if="e.staff === auth.record?.id || auth.isAdmin"
            class="btn btn-ghost btn-xs text-error"
            @click="remove(e)"
          >✕</button>
        </li>
      </ul>

      <div class="space-y-1">
        <select v-if="visits.length" v-model="visitId" class="select select-bordered select-sm w-full" :disabled="saving">
          <option value="">Desk work</option>
          <option v-for="v in visits" :key="v.id" :value="v.id">On-site — {{ visitLabel(v) }}</option>
        </select>
        <div class="flex gap-1 min-w-0">
          <input v-model.number="minutes" type="number" min="1" placeholder="min" class="input input-bordered input-sm w-16 shrink-0" :disabled="saving" />
          <input v-model="note" type="text" placeholder="note" class="input input-bordered input-sm flex-1 min-w-0" :disabled="saving" />
          <button class="btn btn-sm btn-primary shrink-0" :disabled="saving || !minutes" @click="add">Log</button>
        </div>
      </div>
    </div>
  </div>
</template>
