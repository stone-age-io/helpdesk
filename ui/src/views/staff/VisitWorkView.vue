<script setup lang="ts">
// Full-screen, phone-first field flow for one visit: Arrive → live timer →
// Complete. Complete logs the on-site time tagged to this visit AND flips the
// visit to completed in one action (the timer store's stop route), collapsing
// the drawer's old log-then-complete two-step. Large touch targets, one
// decision on screen at a time.
import { computed, onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { pb } from '@/pb'
import { useTimerStore } from '@/stores/timer'
import type { Visit } from '@/types'
import { format } from 'date-fns'

const route = useRoute()
const router = useRouter()
const timer = useTimerStore()

const visit = ref<Visit | null>(null)
const loading = ref(true)
const error = ref('')

const completing = ref(false)
const editMinutes = ref<number | null>(null)
const editNote = ref('')
const done = ref(false)

const visitId = computed(() => route.params.id as string)
const ticket = computed(() => visit.value?.expand?.ticket)
const timingThis = computed(() => (visit.value ? timer.isTimingVisit(visit.value.id) : false))
const otherTimer = computed(() => !!timer.active && !timingThis.value)
const closed = computed(() => visit.value?.status === 'completed' || visit.value?.status === 'canceled')

const windowLabel = computed(() => {
  const v = visit.value
  if (!v?.scheduled_at) return 'Not scheduled'
  const start = new Date(v.scheduled_at)
  const base = format(start, 'EEE, MMM d · HH:mm')
  if (!v.duration_minutes) return base
  const end = new Date(start.getTime() + v.duration_minutes * 60000)
  return `${base}–${format(end, 'HH:mm')}`
})
const mapsHref = computed(() =>
  visit.value?.location ? `https://maps.google.com/?q=${encodeURIComponent(visit.value.location)}` : '',
)

function fmtElapsed(s: number): string {
  const h = Math.floor(s / 3600)
  const m = Math.floor((s % 3600) / 60)
  const sec = s % 60
  const pad = (n: number) => String(n).padStart(2, '0')
  return h > 0 ? `${h}:${pad(m)}:${pad(sec)}` : `${m}:${pad(sec)}`
}

async function load() {
  loading.value = true
  error.value = ''
  try {
    visit.value = await pb.collection('visits').getOne<Visit>(visitId.value, { expand: 'ticket' })
  } catch (e: any) {
    error.value = e?.message || 'Failed to load visit'
  } finally {
    loading.value = false
  }
}

async function arrive() {
  if (!visit.value) return
  error.value = ''
  try {
    await timer.start(visit.value.ticket, { visit: visit.value.id })
  } catch (e: any) {
    error.value = e?.message || 'A timer is already running — stop it first'
    await timer.load()
  }
}

function beginComplete() {
  // Prefill the rounded elapsed; forgot to start on time? Just edit it.
  editMinutes.value = Math.max(1, Math.round(timer.elapsedSeconds / 60 / 5) * 5)
  editNote.value = timer.active?.note || ''
  error.value = ''
  completing.value = true
}

async function confirmComplete() {
  error.value = ''
  try {
    await timer.stop({
      minutes: editMinutes.value || undefined,
      note: editNote.value.trim(),
      completeVisit: true,
    })
    completing.value = false
    done.value = true
    await load()
  } catch (e: any) {
    error.value = e?.message || 'Failed to complete visit'
  }
}

async function logKeepOpen() {
  error.value = ''
  try {
    await timer.stop({ completeVisit: false })
    await load()
  } catch (e: any) {
    error.value = e?.message || 'Failed to log time'
  }
}

async function discard() {
  try {
    await timer.cancel()
  } catch {
    /* already gone */
  }
}

function openTicket() {
  if (visit.value) router.push(`/staff/tickets/${visit.value.ticket}`)
}

onMounted(load)
</script>

<template>
  <div class="mx-auto w-full max-w-md space-y-4">
    <button class="btn btn-ghost btn-sm -ml-2" @click="router.back()">← Back</button>

    <div v-if="loading" class="flex justify-center p-12"><span class="loading loading-spinner loading-lg"></span></div>
    <div v-else-if="error && !visit" class="alert alert-error text-sm">{{ error }}</div>

    <template v-else-if="visit">
      <!-- Job facts -->
      <div class="card bg-base-100 shadow-sm">
        <div class="card-body gap-2 p-4">
          <div class="text-xs uppercase tracking-wide text-base-content/50">On-site visit</div>
          <h1 class="text-lg font-semibold leading-tight">
            <span v-if="ticket">#{{ ticket.number }} · </span>{{ ticket?.title || 'Ticket' }}
          </h1>
          <div class="text-sm text-base-content/70">{{ windowLabel }}</div>
          <a v-if="visit.location" :href="mapsHref" target="_blank" rel="noopener" class="link link-primary text-sm break-words">
            📍 {{ visit.location }}
          </a>
          <p v-if="visit.notes" class="whitespace-pre-wrap text-sm text-base-content/80">{{ visit.notes }}</p>
        </div>
      </div>

      <div v-if="error" class="alert alert-error py-2 text-sm">{{ error }}</div>

      <!-- Done -->
      <div v-if="done" class="card bg-success/10 shadow-sm">
        <div class="card-body items-center gap-3 p-6 text-center">
          <div class="text-4xl">✓</div>
          <p class="font-medium">Visit completed and time logged.</p>
          <button class="btn btn-primary w-full" @click="openTicket">Open ticket</button>
          <button class="btn btn-ghost w-full" @click="router.push('/staff/dispatch')">Back to dispatch</button>
        </div>
      </div>

      <!-- Already closed -->
      <div v-else-if="closed" class="card bg-base-100 shadow-sm">
        <div class="card-body items-center gap-3 p-6 text-center">
          <span class="badge-soft" :class="visit.status === 'completed' ? 'badge-soft-success' : 'badge-soft-neutral'">{{ visit.status }}</span>
          <p class="text-sm text-base-content/70">This visit is {{ visit.status }}.</p>
          <button class="btn btn-primary w-full" @click="openTicket">Open ticket</button>
        </div>
      </div>

      <!-- Timing this visit -->
      <div v-else-if="timingThis" class="card bg-base-100 shadow-sm">
        <div class="card-body items-center gap-4 p-6">
          <div class="flex items-center gap-2 text-sm text-success">
            <span class="inline-flex h-2.5 w-2.5 rounded-full bg-success animate-pulse"></span> On the clock
          </div>
          <div class="font-mono text-5xl tabular-nums">{{ fmtElapsed(timer.elapsedSeconds) }}</div>

          <template v-if="!completing">
            <button class="btn btn-success btn-lg w-full" :disabled="timer.busy" @click="beginComplete">✓ Complete visit</button>
            <button class="btn btn-ghost w-full" :disabled="timer.busy" @click="logKeepOpen">Log time, keep visit open</button>
            <button class="btn btn-ghost btn-sm w-full text-error" :disabled="timer.busy" @click="discard">Discard timer</button>
          </template>

          <!-- Complete confirm: editable minutes + note -->
          <div v-else class="w-full space-y-3">
            <label class="block text-sm">
              <span class="text-base-content/60">Time on site</span>
              <div class="mt-1 flex items-center gap-2">
                <input v-model.number="editMinutes" type="number" min="1" class="input input-bordered w-24" :disabled="timer.busy" />
                <span class="text-sm text-base-content/60">minutes</span>
              </div>
            </label>
            <label class="block text-sm">
              <span class="text-base-content/60">Note (optional)</span>
              <textarea v-model="editNote" rows="2" class="textarea textarea-bordered mt-1 w-full" :disabled="timer.busy"></textarea>
            </label>
            <button class="btn btn-success btn-lg w-full" :disabled="timer.busy" @click="confirmComplete">
              <span v-if="timer.busy" class="loading loading-spinner loading-sm"></span>
              Log {{ editMinutes || 0 }} min & complete
            </button>
            <button class="btn btn-ghost w-full" :disabled="timer.busy" @click="completing = false">Back</button>
          </div>
        </div>
      </div>

      <!-- Not timing: arrive -->
      <div v-else class="card bg-base-100 shadow-sm">
        <div class="card-body items-center gap-3 p-6">
          <div v-if="otherTimer" class="alert alert-warning py-2 text-sm">
            A timer is already running on something else. Stop it from the bar first.
          </div>
          <button class="btn btn-primary btn-lg w-full" :disabled="otherTimer" @click="arrive">▶ Arrive &amp; start timer</button>
          <p class="text-center text-xs text-base-content/50">Starts the clock now. You can adjust the minutes when you complete.</p>
        </div>
      </div>
    </template>
  </div>
</template>
