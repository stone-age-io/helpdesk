<script setup lang="ts">
// Full-screen, phone-first field flow for one visit: Arrive → live timer →
// Complete. Complete logs the on-site time tagged to this visit AND flips the
// visit to completed in one action (the timer store's stop route), collapsing
// the drawer's old log-then-complete two-step. Large touch targets, one
// decision on screen at a time.
import { computed, onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { pb } from '@/pb'
import { useAuthStore } from '@/stores/auth'
import { useTimerStore } from '@/stores/timer'
import type { Location, Visit } from '@/types'
import MinuteChips from '@/components/MinuteChips.vue'
import { format } from 'date-fns'

const route = useRoute()
const router = useRouter()
const auth = useAuthStore()
const timer = useTimerStore()

// Field agents return to their Today list; desk staff to Dispatch (no field tab).
const backTo = computed(() => (auth.isField ? '/staff/today' : '/staff/dispatch'))
const backLabel = computed(() => (auth.isField ? 'Back to today' : 'Back to dispatch'))

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
// The structured site (ticket.location relation, migration 1812/1813) carries
// address, access notes, contact, and coordinates. Prefer it; fall back to the
// visit's free-text dispatch directions.
const site = computed(() => visit.value?.expand?.ticket?.expand?.location as Location | undefined)
const mapsHref = computed(() => {
  const s = site.value
  if (s?.lat != null && s?.lng != null) return `https://maps.google.com/?q=${s.lat},${s.lng}`
  if (s?.address) return `https://maps.google.com/?q=${encodeURIComponent(s.address)}`
  return visit.value?.location ? `https://maps.google.com/?q=${encodeURIComponent(visit.value.location)}` : ''
})

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
    visit.value = await pb.collection('visits').getOne<Visit>(visitId.value, { expand: 'ticket,ticket.location,ticket.customer' })
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

// On-site photo → appended to the ticket's attachments (PB's `field+` append).
// Attachments don't change status/assignee/priority, so no notification fires.
const photoInput = ref<HTMLInputElement | null>(null)
const photoCount = ref(0)
const photoBusy = ref(false)
async function onPhoto(e: Event) {
  const input = e.target as HTMLInputElement
  const file = input.files?.[0]
  if (!file || !visit.value) return
  photoBusy.value = true
  error.value = ''
  try {
    const fd = new FormData()
    fd.append('attachments+', file)
    await pb.collection('tickets').update(visit.value.ticket, fd)
    photoCount.value++
  } catch (e: any) {
    error.value = e?.message || 'Failed to add photo (max 6 per ticket)'
  } finally {
    photoBusy.value = false
    input.value = ''
  }
}

// No-timer quick log: forgot to hit Arrive, or a two-minute job. Chips pick the
// minutes; optionally complete the visit in the same tap. Only offered when no
// timer is running here, so it can't race the timer's own stop route.
const manualOpen = ref(false)
const manualMinutes = ref<number | null>(null)
const manualNote = ref('')
const manualBusy = ref(false)
async function logManual(complete: boolean) {
  if (!visit.value || !manualMinutes.value) return
  manualBusy.value = true
  error.value = ''
  try {
    await pb.collection('time_entries').create({
      ticket: visit.value.ticket,
      staff: auth.record?.id,
      minutes: manualMinutes.value,
      work_date: new Date().toISOString(),
      note: manualNote.value.trim(),
      visit: visit.value.id,
    })
    if (complete) {
      // Completion is a silent transition (no visit notification); the guard
      // hook stamps completed_at.
      await pb.collection('visits').update(visit.value.id, { status: 'completed' })
      done.value = true
    }
    manualOpen.value = false
    manualMinutes.value = null
    manualNote.value = ''
    await load()
  } catch (e: any) {
    error.value = e?.message || 'Failed to log time'
  } finally {
    manualBusy.value = false
  }
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
          <div v-if="ticket?.expand?.customer?.name" class="text-sm text-base-content/70">{{ ticket.expand.customer.name }}</div>
          <div class="text-sm text-base-content/60">{{ windowLabel }}</div>

          <!-- Location: structured site preferred, with a Navigate button; the
               free-text dispatch note falls back when there's no linked site. -->
          <div v-if="site || visit.location" class="flex items-start gap-2 pt-1">
            <span aria-hidden="true">📍</span>
            <div class="flex-1 min-w-0">
              <div class="text-sm font-medium break-words">{{ site?.name || visit.location }}</div>
              <div v-if="site?.address" class="text-sm text-base-content/70 break-words">{{ site.address }}</div>
            </div>
            <a v-if="mapsHref" :href="mapsHref" target="_blank" rel="noopener" class="btn btn-outline btn-xs shrink-0">Navigate</a>
          </div>

          <p v-if="site?.notes" class="whitespace-pre-wrap text-sm text-base-content/80">
            <span class="text-base-content/50">Access:</span> {{ site.notes }}
          </p>
          <p v-if="visit.location && site" class="text-sm text-base-content/70 break-words">
            <span class="text-base-content/50">Directions:</span> {{ visit.location }}
          </p>

          <!-- Contact: name and number both visible, with a Call button. -->
          <div v-if="site?.contact || site?.contact_phone" class="flex items-center gap-2">
            <span aria-hidden="true">📞</span>
            <div class="flex-1 min-w-0 text-sm break-words">
              <span v-if="site?.contact" class="font-medium">{{ site.contact }}</span>
              <span v-if="site?.contact_phone" class="text-base-content/70"><template v-if="site?.contact"> · </template>{{ site.contact_phone }}</span>
            </div>
            <a v-if="site?.contact_phone" :href="`tel:${site.contact_phone}`" class="btn btn-outline btn-xs shrink-0">Call</a>
          </div>

          <p v-if="visit.notes" class="whitespace-pre-wrap text-sm text-base-content/80">{{ visit.notes }}</p>

          <!-- Actions: open the ticket, or snap an on-site photo onto it (either
               works whether or not the timer is running). -->
          <div class="flex flex-wrap gap-2 pt-1">
            <button class="btn btn-outline btn-sm" @click="openTicket">🎫 Open ticket</button>
            <input ref="photoInput" type="file" accept="image/*" capture="environment" class="hidden" @change="onPhoto" />
            <button class="btn btn-outline btn-sm" :disabled="photoBusy" @click="photoInput?.click()">
              <span v-if="photoBusy" class="loading loading-spinner loading-xs"></span>
              📷 Add photo<span v-if="photoCount"> · {{ photoCount }} added</span>
            </button>
          </div>
        </div>
      </div>

      <div v-if="error" class="alert alert-error py-2 text-sm">{{ error }}</div>

      <!-- Done -->
      <div v-if="done" class="card bg-success/10 shadow-sm">
        <div class="card-body items-center gap-3 p-6 text-center">
          <div class="text-4xl">✓</div>
          <p class="font-medium">Visit completed and time logged.</p>
          <button class="btn btn-primary w-full" @click="openTicket">Open ticket</button>
          <button class="btn btn-ghost w-full" @click="router.push(backTo)">{{ backLabel }}</button>
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
            <div class="text-sm">
              <span class="text-base-content/60">Time on site</span>
              <MinuteChips v-model="editMinutes" :disabled="timer.busy" class="mt-2" />
              <div class="mt-2 flex items-center gap-2">
                <input v-model.number="editMinutes" type="number" min="1" class="input input-bordered w-24" :disabled="timer.busy" />
                <span class="text-sm text-base-content/60">minutes</span>
              </div>
            </div>
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

      <!-- Not timing: arrive, or log without the timer -->
      <div v-else class="card bg-base-100 shadow-sm">
        <div class="card-body gap-3 p-6">
          <div v-if="otherTimer" class="alert alert-warning py-2 text-sm">
            A timer is already running on something else. Stop it from the bar first.
          </div>
          <button class="btn btn-primary btn-lg w-full" :disabled="otherTimer" @click="arrive">▶ Arrive &amp; start timer</button>
          <p class="text-center text-xs text-base-content/50">Starts the clock now. You can adjust the minutes when you complete.</p>

          <div class="divider my-1 text-xs text-base-content/40">or</div>

          <button v-if="!manualOpen" class="btn btn-ghost btn-sm w-full" @click="manualOpen = true">Log time without the timer</button>
          <div v-else class="space-y-3">
            <div class="text-sm text-base-content/60">Time on site</div>
            <MinuteChips v-model="manualMinutes" :disabled="manualBusy" />
            <input v-model.number="manualMinutes" type="number" min="1" placeholder="minutes" class="input input-bordered input-sm w-full" :disabled="manualBusy" />
            <textarea v-model="manualNote" rows="2" placeholder="Note (optional)" class="textarea textarea-bordered textarea-sm w-full" :disabled="manualBusy"></textarea>
            <button class="btn btn-success w-full" :disabled="manualBusy || !manualMinutes" @click="logManual(true)">
              <span v-if="manualBusy" class="loading loading-spinner loading-sm"></span>
              Log {{ manualMinutes || 0 }} min &amp; complete
            </button>
            <button class="btn btn-ghost btn-sm w-full" :disabled="manualBusy || !manualMinutes" @click="logManual(false)">Log only, keep open</button>
            <button class="btn btn-ghost btn-sm w-full" :disabled="manualBusy" @click="manualOpen = false">Cancel</button>
          </div>
        </div>
      </div>
    </template>
  </div>
</template>
