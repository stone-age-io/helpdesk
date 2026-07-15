<script setup lang="ts">
// Right slide-over showing one visit in full: window, technician, location,
// full notes, lifecycle actions, and the labor logged against it. Staff-only —
// used by VisitsCard (ticket view) and the Dispatch board so a visit is finally
// a viewable object, not just an inline row. Fetches its own data by id and
// emits `changed` so the opener can refresh its list.
import { computed, onMounted, onUnmounted, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import { pb } from '@/pb'
import { useAuthStore } from '@/stores/auth'
import { useTimerStore } from '@/stores/timer'
import type { Staff, TimeEntry, Visit } from '@/types'
import VisitScheduleForm from '@/components/VisitScheduleForm.vue'
import { format } from 'date-fns'

const props = defineProps<{ visitId: string | null; staff: Staff[] }>()
const emit = defineEmits<{ close: []; changed: [] }>()
const auth = useAuthStore()
const router = useRouter()
const timer = useTimerStore()

const visit = ref<Visit | null>(null)
const entries = ref<TimeEntry[]>([])
const loading = ref(false)
const error = ref('')
const saving = ref(false)

// '' = read view; 'reschedule' shows the form; 'complete' shows the optional
// on-site time capture before confirming.
const mode = ref<'' | 'reschedule' | 'complete'>('')
const completeMinutes = ref<number | null>(null)

// Log-time-to-this-visit form.
const logMinutes = ref<number | null>(null)
const logNote = ref('')

const statusClass: Record<string, string> = {
  requested: 'badge-soft-warning',
  scheduled: 'badge-soft-info',
  completed: 'badge-soft-success',
  canceled: 'badge-soft-neutral',
}

const totalMinutes = computed(() => entries.value.reduce((s, e) => s + e.minutes, 0))

function fmtMinutes(m: number): string {
  const h = Math.floor(m / 60)
  return h > 0 ? `${h}h ${m % 60}m` : `${m}m`
}
const windowLabel = computed(() => {
  const v = visit.value
  if (!v?.scheduled_at) return 'Not scheduled'
  const start = new Date(v.scheduled_at)
  const base = format(start, 'EEE, MMM d · HH:mm')
  if (!v.duration_minutes) return base
  const end = new Date(start.getTime() + v.duration_minutes * 60000)
  return `${base}–${format(end, 'HH:mm')} (${fmtMinutes(v.duration_minutes)})`
})

async function load() {
  if (!props.visitId) return
  loading.value = true
  error.value = ''
  mode.value = ''
  try {
    visit.value = await pb.collection('visits').getOne<Visit>(props.visitId, {
      expand: 'ticket,assignee',
    })
    entries.value = await pb.collection('time_entries').getFullList<TimeEntry>({
      filter: `visit = '${props.visitId}'`,
      sort: '-work_date',
      expand: 'staff',
    })
  } catch (err: any) {
    error.value = err?.message || 'Failed to load visit'
  } finally {
    loading.value = false
  }
}
watch(() => props.visitId, load)

async function setStatus(status: string) {
  if (!visit.value) return
  saving.value = true
  error.value = ''
  try {
    await pb.collection('visits').update(visit.value.id, { status })
    await load()
    emit('changed')
  } catch (err: any) {
    error.value = err?.message || 'Failed to update visit'
  } finally {
    saving.value = false
  }
}

async function submitReschedule(fields: Record<string, any>) {
  if (!visit.value) return
  saving.value = true
  error.value = ''
  try {
    await pb.collection('visits').update(visit.value.id, fields)
    await load()
    emit('changed')
  } catch (err: any) {
    error.value = err?.message || 'Failed to reschedule'
  } finally {
    saving.value = false
  }
}

// Completion optionally captures the on-site time in the same step — the moment
// the field minutes are actually known — tagging it to this visit.
async function confirmComplete() {
  if (!visit.value) return
  saving.value = true
  error.value = ''
  try {
    await pb.collection('visits').update(visit.value.id, { status: 'completed' })
    if (completeMinutes.value && completeMinutes.value > 0) {
      await pb.collection('time_entries').create({
        ticket: visit.value.ticket,
        staff: auth.record?.id,
        minutes: completeMinutes.value,
        work_date: new Date().toISOString(),
        visit: visit.value.id,
      })
    }
    completeMinutes.value = null
    await load()
    emit('changed')
  } catch (err: any) {
    error.value = err?.message || 'Failed to complete visit'
  } finally {
    saving.value = false
  }
}

async function logTime() {
  if (!visit.value || !logMinutes.value || logMinutes.value < 1) return
  saving.value = true
  error.value = ''
  try {
    await pb.collection('time_entries').create({
      ticket: visit.value.ticket,
      staff: auth.record?.id,
      minutes: logMinutes.value,
      work_date: new Date().toISOString(),
      visit: visit.value.id,
      note: logNote.value.trim(),
    })
    logMinutes.value = null
    logNote.value = ''
    await load()
    emit('changed')
  } catch (err: any) {
    error.value = err?.message || 'Failed to log time'
  } finally {
    saving.value = false
  }
}

async function removeEntry(e: TimeEntry) {
  try {
    await pb.collection('time_entries').delete(e.id)
    await load()
    emit('changed')
  } catch (err: any) {
    error.value = err?.message || 'Failed to delete entry'
  }
}

function openTicket() {
  if (!visit.value) return
  const t = visit.value.ticket
  emit('close')
  router.push(`/staff/tickets/${t}`)
}

// The full-screen field flow (Arrive → Complete). The drawer is the desktop
// entry point; the work view is where a tech on-site actually lives.
function openWorkView() {
  if (!visit.value) return
  emit('close')
  router.push(`/staff/visits/${visit.value.id}/work`)
}

// Time this visit right from the drawer (desktop convenience). Stopping is done
// from the persistent bar; the entry lands back in this drawer's list.
async function startTimer() {
  if (!visit.value) return
  error.value = ''
  try {
    await timer.start(visit.value.ticket, { visit: visit.value.id })
  } catch (err: any) {
    error.value = err?.message || 'A timer is already running — stop it first'
    await timer.load()
  }
}

function onKey(e: KeyboardEvent) {
  if (e.key === 'Escape' && props.visitId) emit('close')
}
onMounted(() => window.addEventListener('keydown', onKey))
onUnmounted(() => window.removeEventListener('keydown', onKey))
</script>

<template>
  <Teleport to="body">
    <div v-if="visitId" class="fixed inset-0 z-40">
      <div class="absolute inset-0 bg-black/40" @click="emit('close')"></div>
      <aside class="absolute right-0 top-0 h-full w-full max-w-md bg-base-100 shadow-xl overflow-y-auto flex flex-col">
        <div class="flex items-center gap-2 p-4 border-b border-base-300 sticky top-0 bg-base-100 z-10">
          <h2 class="font-semibold flex-1 truncate">
            Visit
            <span v-if="visit?.expand?.ticket" class="font-normal text-base-content/60">
              · #{{ visit.expand.ticket.number }} {{ visit.expand.ticket.title }}
            </span>
          </h2>
          <span v-if="visit" class="badge-soft" :class="statusClass[visit.status]">{{ visit.status }}</span>
          <button class="btn btn-ghost btn-sm btn-circle" @click="emit('close')">✕</button>
        </div>

        <div v-if="loading" class="flex justify-center p-12"><span class="loading loading-spinner loading-lg"></span></div>

        <div v-else-if="visit" class="p-4 space-y-4 flex-1">
          <div v-if="error" class="alert alert-error py-2 text-sm">{{ error }}</div>

          <!-- Facts -->
          <div class="space-y-2 text-sm">
            <div class="flex items-start gap-2">
              <span class="text-base-content/60 w-24 shrink-0">When</span>
              <span class="font-medium">{{ windowLabel }}</span>
            </div>
            <div class="flex items-start gap-2">
              <span class="text-base-content/60 w-24 shrink-0">Technician</span>
              <span>{{ visit.expand?.assignee?.name || '—' }}</span>
            </div>
            <div v-if="visit.location" class="flex items-start gap-2">
              <span class="text-base-content/60 w-24 shrink-0">Location</span>
              <span class="flex-1">📍 {{ visit.location }}</span>
            </div>
            <div v-if="visit.notes" class="flex items-start gap-2">
              <span class="text-base-content/60 w-24 shrink-0">Notes</span>
              <span class="flex-1 whitespace-pre-wrap">{{ visit.notes }}</span>
            </div>
          </div>

          <!-- Reschedule form -->
          <div v-if="mode === 'reschedule'" class="border-t border-base-300 pt-3">
            <p class="text-xs text-base-content/60 mb-1">{{ visit.status === 'requested' ? 'Schedule this visit' : 'Reschedule' }}</p>
            <VisitScheduleForm :staff="staff" :visit="visit" :saving="saving" @submit="submitReschedule" @cancel="mode = ''" />
          </div>

          <!-- Complete + optional on-site time capture -->
          <div v-else-if="mode === 'complete'" class="border-t border-base-300 pt-3 space-y-2">
            <p class="text-sm font-medium">Mark completed</p>
            <label class="text-xs text-base-content/60">On-site time (optional) — logged against this visit</label>
            <div class="flex gap-1">
              <input v-model.number="completeMinutes" type="number" min="1" placeholder="min" class="input input-bordered input-sm w-24" :disabled="saving" />
              <button class="btn btn-ghost btn-sm" :disabled="saving" @click="mode = ''">Cancel</button>
              <button class="btn btn-success btn-sm flex-1" :disabled="saving" @click="confirmComplete">
                <span v-if="saving" class="loading loading-spinner loading-xs"></span>
                Confirm complete
              </button>
            </div>
          </div>

          <!-- Action buttons (read view) -->
          <div v-else class="flex flex-wrap gap-2 border-t border-base-300 pt-3">
            <button class="btn btn-sm btn-primary" :disabled="saving" @click="mode = 'reschedule'">
              {{ visit.status === 'requested' ? 'Schedule' : 'Reschedule' }}
            </button>
            <button v-if="visit.status !== 'canceled'" class="btn btn-sm" :disabled="saving" @click="openWorkView">🛠 Work view</button>
            <button
              v-if="visit.status !== 'canceled' && !timer.isTimingVisit(visit.id)"
              class="btn btn-sm btn-ghost"
              :disabled="saving || !!timer.active"
              :title="timer.active ? 'Stop your running timer first' : 'Start timing this visit'"
              @click="startTimer"
            >▶ Start timer</button>
            <span v-else-if="timer.isTimingVisit(visit.id)" class="inline-flex items-center gap-1 self-center text-xs text-success">
              <span class="inline-flex h-2 w-2 rounded-full bg-success animate-pulse"></span> timing
            </span>
            <button v-if="visit.status === 'scheduled'" class="btn btn-sm" :disabled="saving" @click="mode = 'complete'">Complete</button>
            <button v-if="visit.status !== 'canceled'" class="btn btn-sm btn-ghost text-error" :disabled="saving" @click="setStatus('canceled')">Cancel visit</button>
          </div>

          <!-- Time logged on this visit -->
          <div class="border-t border-base-300 pt-3 space-y-2">
            <div class="flex items-center justify-between">
              <h3 class="font-semibold text-sm">Time on this visit</h3>
              <span class="badge badge-ghost badge-sm">{{ fmtMinutes(totalMinutes) }}</span>
            </div>
            <ul class="space-y-1">
              <li v-for="e in entries" :key="e.id" class="flex items-center justify-between text-sm gap-2">
                <span class="text-base-content/70 whitespace-nowrap">{{ format(new Date(e.work_date), 'MMM d') }}</span>
                <span class="flex-1 truncate" :title="e.note">{{ e.note || e.expand?.staff?.name || '' }}</span>
                <span class="font-mono whitespace-nowrap">{{ fmtMinutes(e.minutes) }}</span>
                <button v-if="e.staff === auth.record?.id || auth.isAdmin" class="btn btn-ghost btn-xs text-error" @click="removeEntry(e)">✕</button>
              </li>
              <li v-if="entries.length === 0" class="text-xs text-base-content/50">No time logged yet.</li>
            </ul>
            <div class="flex gap-1 min-w-0">
              <input v-model.number="logMinutes" type="number" min="1" placeholder="min" class="input input-bordered input-sm w-16 shrink-0" :disabled="saving" />
              <input v-model="logNote" type="text" placeholder="note" class="input input-bordered input-sm flex-1 min-w-0" :disabled="saving" />
              <button class="btn btn-sm btn-primary shrink-0" :disabled="saving || !logMinutes" @click="logTime">Log</button>
            </div>
          </div>

          <button class="btn btn-ghost btn-sm w-full" @click="openTicket">Open ticket →</button>
        </div>
      </aside>
    </div>
  </Teleport>
</template>
