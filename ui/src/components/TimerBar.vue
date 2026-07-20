<script setup lang="ts">
// Persistent running-timer strip, rendered by StaffLayout above the scrolling
// content so it stays put across navigation. Shows the live elapsed clock and
// what's being worked; Stop opens an inline confirm with an editable minutes
// value (pre-filled with the rounded elapsed) so a forgotten timer is a trivial
// correction, plus Discard (log nothing) and, for a visit timer, complete-visit.
import { computed, ref } from 'vue'
import { useRouter } from 'vue-router'
import { useTimerStore } from '@/stores/timer'

const timer = useTimerStore()
const router = useRouter()

const confirming = ref(false)
const editMinutes = ref<number | null>(null)
const editNote = ref('')
const completeVisit = ref(false)
const nonBillable = ref(false)
const error = ref('')

const onVisit = computed(() => !!timer.active?.visit)
const ticketLabel = computed(() => {
  const t = timer.active?.expand?.ticket
  return t ? `#${t.number} ${t.title}` : 'Working…'
})

function fmtElapsed(s: number): string {
  const h = Math.floor(s / 3600)
  const m = Math.floor((s % 3600) / 60)
  const sec = s % 60
  const pad = (n: number) => String(n).padStart(2, '0')
  return h > 0 ? `${h}:${pad(m)}:${pad(sec)}` : `${m}:${pad(sec)}`
}

function beginStop() {
  editMinutes.value = Math.max(1, Math.round(timer.elapsedSeconds / 60 / 5) * 5)
  editNote.value = timer.active?.note || ''
  completeVisit.value = false
  nonBillable.value = false
  error.value = ''
  confirming.value = true
}

async function confirmStop() {
  error.value = ''
  try {
    await timer.stop({
      minutes: editMinutes.value || undefined,
      note: editNote.value.trim(),
      completeVisit: onVisit.value && completeVisit.value,
      nonBillable: nonBillable.value,
    })
    confirming.value = false
  } catch (e: any) {
    error.value = e?.message || 'Failed to stop timer'
  }
}

async function discard() {
  error.value = ''
  try {
    await timer.cancel()
    confirming.value = false
  } catch (e: any) {
    error.value = e?.message || 'Failed to discard timer'
  }
}

function openTicket() {
  const t = timer.active?.ticket
  if (t) router.push(`/staff/tickets/${t}`)
}
</script>

<template>
  <div v-if="timer.active" class="bg-primary text-primary-content px-3 py-2 shadow-sm">
    <div class="mx-auto w-full max-w-7xl flex items-center gap-3">
      <span class="inline-flex h-2.5 w-2.5 rounded-full bg-primary-content animate-pulse shrink-0" aria-hidden="true"></span>
      <button class="font-mono text-lg tabular-nums shrink-0" title="Elapsed — click to stop" @click="beginStop">
        {{ fmtElapsed(timer.elapsedSeconds) }}
      </button>
      <button class="flex-1 min-w-0 truncate text-left text-sm opacity-90 hover:opacity-100" @click="openTicket">
        <span v-if="onVisit" title="on-site">📍 </span>{{ ticketLabel }}
      </button>
      <button v-if="!confirming" class="btn btn-sm shrink-0" :disabled="timer.busy" @click="beginStop">Stop</button>
    </div>

    <div v-if="confirming" class="mx-auto w-full max-w-7xl mt-2 flex flex-wrap items-center gap-2">
      <div v-if="error" class="w-full text-xs bg-error text-error-content rounded px-2 py-1">{{ error }}</div>
      <span class="text-xs opacity-90">Log</span>
      <input v-model.number="editMinutes" type="number" min="1" class="input input-bordered input-sm w-20 text-base-content" :disabled="timer.busy" />
      <span class="text-xs opacity-90">min</span>
      <input v-model="editNote" type="text" placeholder="note" class="input input-bordered input-sm flex-1 min-w-[8rem] text-base-content" :disabled="timer.busy" />
      <label v-if="onVisit" class="flex items-center gap-1 text-xs cursor-pointer">
        <input v-model="completeVisit" type="checkbox" class="checkbox checkbox-xs" :disabled="timer.busy" />
        complete visit
      </label>
      <label class="flex items-center gap-1 text-xs cursor-pointer">
        <input v-model="nonBillable" type="checkbox" class="checkbox checkbox-xs" :disabled="timer.busy" />
        non-billable
      </label>
      <button class="btn btn-sm btn-success" :disabled="timer.busy" @click="confirmStop">
        <span v-if="timer.busy" class="loading loading-spinner loading-xs"></span>
        Log time
      </button>
      <button class="btn btn-sm btn-ghost" :disabled="timer.busy" @click="discard">Discard</button>
      <button class="btn btn-sm btn-ghost" :disabled="timer.busy" @click="confirming = false">Keep running</button>
    </div>
  </div>
</template>
