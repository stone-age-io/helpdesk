import { defineStore } from 'pinia'
import { computed, ref } from 'vue'
import { pb } from '@/pb'
import { useAuthStore } from '@/stores/auth'
import type { TimeEntry, TimeSession } from '@/types'

// The running-timer store: the ergonomic front-end to the time_entries ledger.
// Holds the agent's ONE open time_sessions row (if any), ticks a 1s clock so
// the timer bar shows live elapsed, and resolves the timer into a normal
// TimeEntry on stop via the server route. Nothing is stored client-side beyond
// the open session — refreshing the page just re-loads it.
export const useTimerStore = defineStore('timer', () => {
  const active = ref<TimeSession | null>(null)
  const busy = ref(false)
  const nowMs = ref(Date.now())
  let ticker: ReturnType<typeof setInterval> | undefined

  const elapsedSeconds = computed(() => {
    if (!active.value) return 0
    const started = new Date(active.value.started_at).getTime()
    return Math.max(0, Math.floor((nowMs.value - started) / 1000))
  })

  function startTicker() {
    stopTicker()
    nowMs.value = Date.now()
    ticker = setInterval(() => (nowMs.value = Date.now()), 1000)
  }
  function stopTicker() {
    if (ticker) clearInterval(ticker)
    ticker = undefined
  }

  const isTimingTicket = (ticketId: string) => active.value?.ticket === ticketId
  const isTimingVisit = (visitId: string) => active.value?.visit === visitId

  // load re-syncs from the server (own open session). Called on shell mount and
  // safe to call anytime — it's how a second device / a stale tab catches up.
  async function load() {
    const auth = useAuthStore()
    if (!auth.isStaff || !auth.record?.id) {
      active.value = null
      stopTicker()
      return
    }
    try {
      active.value = await pb
        .collection('time_sessions')
        .getFirstListItem<TimeSession>(`staff = "${auth.record.id}"`, { expand: 'ticket,visit' })
      startTicker()
    } catch {
      // 404 = no timer running; anything else = treat as none (progressive).
      active.value = null
      stopTicker()
    }
  }

  // start opens a timer on a ticket (optionally an on-site visit). Throws if a
  // timer is already running server-side (unique index) — callers should catch
  // and call load() to re-sync, then offer to switch.
  async function start(ticketId: string, opts: { visit?: string; note?: string } = {}) {
    const auth = useAuthStore()
    if (!auth.record?.id) return
    busy.value = true
    try {
      active.value = await pb.collection('time_sessions').create<TimeSession>(
        { staff: auth.record.id, ticket: ticketId, visit: opts.visit || null, note: opts.note || '' },
        { expand: 'ticket,visit' },
      )
      startTicker()
    } finally {
      busy.value = false
    }
  }

  // stop resolves the timer into a TimeEntry (server-side, atomic) and clears
  // local state. minutes omitted → the server rounds the elapsed time.
  async function stop(
    opts: { minutes?: number; note?: string; completeVisit?: boolean } = {},
  ): Promise<TimeEntry | null> {
    if (!active.value) return null
    busy.value = true
    const id = active.value.id
    const body: Record<string, unknown> = {}
    if (opts.minutes && opts.minutes > 0) body.minutes = opts.minutes
    if (opts.note !== undefined) body.note = opts.note
    if (opts.completeVisit) body.complete_visit = true
    try {
      const entry = await pb.send(`/api/helpdesk/timers/${id}/stop`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(body),
      })
      active.value = null
      stopTicker()
      return entry as TimeEntry
    } finally {
      busy.value = false
    }
  }

  // cancel discards the timer without logging anything.
  async function cancel() {
    if (!active.value) return
    const id = active.value.id
    active.value = null
    stopTicker()
    try {
      await pb.collection('time_sessions').delete(id)
    } catch {
      // Already gone (stopped elsewhere) — local state is already cleared.
    }
  }

  return { active, busy, elapsedSeconds, isTimingTicket, isTimingVisit, load, start, stop, cancel }
})
