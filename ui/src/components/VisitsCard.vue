<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { pb } from '@/pb'
import type { Staff, Visit } from '@/types'
import VisitScheduleForm from '@/components/VisitScheduleForm.vue'
import VisitDetailDrawer from '@/components/VisitDetailDrawer.vue'
import { format } from 'date-fns'

const props = defineProps<{ ticketId: string; staff: Staff[] }>()

const visits = ref<Visit[]>([])
// '' = closed, 'request' = promote-to-on-site, 'schedule' = time+tech form.
const mode = ref<'' | 'request' | 'schedule'>('')
const location = ref('')
const notes = ref('')
const saving = ref(false)
const error = ref('')
const openVisitId = ref<string | null>(null)

async function load() {
  try {
    visits.value = await pb.collection('visits').getFullList<Visit>({
      filter: `ticket = '${props.ticketId}'`,
      sort: 'scheduled_at',
      expand: 'assignee',
    })
  } catch {
    // Optional context card.
  }
}

// Active = still needs a dispatcher or is on the calendar; history = done/gone.
const active = computed(() =>
  visits.value
    .filter((v) => v.status === 'requested' || v.status === 'scheduled')
    .sort((a, b) => {
      if (a.status !== b.status) return a.status === 'requested' ? -1 : 1
      return (a.scheduled_at || '').localeCompare(b.scheduled_at || '')
    }),
)
const history = computed(() =>
  visits.value
    .filter((v) => v.status === 'completed' || v.status === 'canceled')
    .sort((a, b) => (b.completed_at || b.updated || '').localeCompare(a.completed_at || a.updated || '')),
)
const summary = computed(() => {
  const c: Record<string, number> = {}
  for (const v of visits.value) c[v.status] = (c[v.status] || 0) + 1
  return ['requested', 'scheduled', 'completed', 'canceled']
    .filter((s) => c[s])
    .map((s) => `${c[s]} ${s}`)
    .join(' · ')
})

const statusClass: Record<string, string> = {
  requested: 'badge-warning',
  scheduled: 'badge-info',
  completed: 'badge-success',
  canceled: 'badge-ghost',
}
function rowLabel(v: Visit): string {
  if (v.status === 'requested') return 'needs scheduling'
  if (v.scheduled_at) return format(new Date(v.scheduled_at), 'MMM d, HH:mm')
  return v.status
}

function closeForm() {
  mode.value = ''
  location.value = ''
  notes.value = ''
}
async function submitRequest() {
  saving.value = true
  error.value = ''
  try {
    await pb.collection('visits').create({
      ticket: props.ticketId,
      status: 'requested',
      location: location.value.trim(),
      notes: notes.value.trim(),
    })
    closeForm()
    await load()
  } catch (err: any) {
    error.value = err?.message || 'Failed to request visit'
  } finally {
    saving.value = false
  }
}
async function submitSchedule(fields: Record<string, any>) {
  saving.value = true
  error.value = ''
  try {
    await pb.collection('visits').create({ ticket: props.ticketId, ...fields })
    closeForm()
    await load()
  } catch (err: any) {
    error.value = err?.message || 'Failed to schedule visit'
  } finally {
    saving.value = false
  }
}

// Realtime so the card reflects drawer edits (and other agents) without a
// manual refresh. Progressive enhancement — the card works without it.
let reloadTimer: ReturnType<typeof setTimeout> | undefined
let unsub: (() => void) | null = null
onMounted(async () => {
  await load()
  try {
    unsub = await pb.collection('visits').subscribe('*', () => {
      clearTimeout(reloadTimer)
      reloadTimer = setTimeout(load, 300)
    })
  } catch {
    // no realtime; fine.
  }
})
onUnmounted(() => {
  clearTimeout(reloadTimer)
  unsub?.()
})
</script>

<template>
  <div class="card bg-base-100 shadow-sm">
    <div class="card-body py-4 px-4 space-y-2">
      <div class="flex justify-between items-center">
        <h2 class="font-semibold text-sm">Site Visits</h2>
        <div class="flex gap-1">
          <template v-if="mode === ''">
            <button class="btn btn-ghost btn-xs" @click="mode = 'request'">+ Request</button>
            <button class="btn btn-ghost btn-xs" @click="mode = 'schedule'">+ Schedule</button>
          </template>
          <button v-else class="btn btn-ghost btn-xs" @click="closeForm">Cancel</button>
        </div>
      </div>

      <div v-if="error" class="text-error text-xs">{{ error }}</div>

      <!-- Create: request (no tech/time yet) -->
      <div v-if="mode === 'request'" class="space-y-1">
        <p class="text-xs text-base-content/60">Flag this ticket for on-site work — a dispatcher assigns the tech and time later.</p>
        <input v-model="location" type="text" placeholder="location (optional)" class="input input-bordered input-sm w-full" :disabled="saving" />
        <textarea v-model="notes" rows="2" placeholder="notes (optional)" class="textarea textarea-bordered textarea-sm w-full" :disabled="saving"></textarea>
        <button class="btn btn-primary btn-sm w-full" :disabled="saving" @click="submitRequest">Request visit</button>
      </div>

      <!-- Create: scheduled directly -->
      <VisitScheduleForm v-else-if="mode === 'schedule'" :staff="staff" :saving="saving" @submit="submitSchedule" @cancel="closeForm" />

      <!-- List -->
      <template v-else>
        <p v-if="summary" class="text-xs text-base-content/50">{{ summary }}</p>

        <ul class="space-y-1">
          <li
            v-for="v in active"
            :key="v.id"
            class="flex items-center gap-2 text-sm rounded px-1 py-0.5 hover:bg-base-200 cursor-pointer"
            @click="openVisitId = v.id"
          >
            <span class="badge badge-xs" :class="statusClass[v.status]">{{ v.status }}</span>
            <span :class="v.status === 'requested' ? 'italic text-base-content/60' : 'font-medium'">{{ rowLabel(v) }}</span>
            <span class="flex-1 truncate text-base-content/70">{{ v.expand?.assignee?.name }}</span>
            <span v-if="v.notes" title="has notes">📝</span>
          </li>
        </ul>
        <p v-if="visits.length === 0" class="text-xs text-base-content/50">No visits yet.</p>

        <details v-if="history.length" class="group">
          <summary class="list-none cursor-pointer text-xs text-base-content/60 flex items-center gap-1 [&::-webkit-details-marker]:hidden">
            <span class="transition-transform group-open:rotate-90">▸</span> History ({{ history.length }})
          </summary>
          <ul class="space-y-1 pt-1">
            <li
              v-for="v in history"
              :key="v.id"
              class="flex items-center gap-2 text-sm rounded px-1 py-0.5 hover:bg-base-200 cursor-pointer"
              @click="openVisitId = v.id"
            >
              <span class="badge badge-xs" :class="statusClass[v.status]">{{ v.status }}</span>
              <span class="text-base-content/70">{{ v.completed_at ? format(new Date(v.completed_at), 'MMM d') : rowLabel(v) }}</span>
              <span class="flex-1 truncate text-base-content/70">{{ v.expand?.assignee?.name }}</span>
              <span v-if="v.notes" title="has notes">📝</span>
            </li>
          </ul>
        </details>
      </template>
    </div>
  </div>

  <VisitDetailDrawer :visit-id="openVisitId" :staff="staff" @close="openVisitId = null" @changed="load" />
</template>
