<script setup lang="ts">
// Unified "Work" area for a ticket: field work and labor in one place, organized
// by visit. Each visit is a collapsible row (date · status · tech · its time
// subtotal); expanding shows that visit's notes + time entries + inline actions.
// Time not tagged to a visit is the "Desk work" bucket. When a ticket has no
// visits at all it degrades to a plain flat time list — a desk-only ticket
// shouldn't wear visit chrome.
//
// This replaces the old side-by-side TimeEntriesCard + VisitsCard. It keeps the
// data model intact: time_entries stay parented to the ticket (the canonical
// ledger) and merely grouped by their optional `visit` tag; visits stay their
// own records. Full visit management (reschedule / complete / cancel) still
// happens in the shared VisitDetailDrawer, opened from a row.
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { pb } from '@/pb'
import { useAuthStore } from '@/stores/auth'
import { useTimerStore } from '@/stores/timer'
import type { Staff, TimeEntry, Visit, VisitStatus } from '@/types'
import VisitDetailDrawer from '@/components/VisitDetailDrawer.vue'
import SearchSelect from '@/components/SearchSelect.vue'
import MinutesInput from '@/components/MinutesInput.vue'
import BillableTag from '@/components/BillableTag.vue'
import { format } from 'date-fns'

const props = defineProps<{ ticketId: string; staff: Staff[]; estimatedMinutes?: number | null }>()
const auth = useAuthStore()
const timer = useTimerStore()

const entries = ref<TimeEntry[]>([])
const visits = ref<Visit[]>([])
const error = ref('')
const saving = ref(false)

// Which header panel is open. Only one at a time.
const panel = ref<'' | 'time' | 'visit'>('')

// Add-time form.
const logMode = ref<'timer' | 'manual'>('manual')
const logTarget = ref('') // '' = desk work, else a visit id
const logMinutes = ref<number | null>(null)
const logNote = ref('')
const logNonBillable = ref(false)

// Add-visit form (progressive: request now, or schedule now).
const nvLocation = ref('')
const nvNotes = ref('')
const nvScheduleNow = ref(false)
const nvAt = ref('')
const nvAssignee = ref('')
const nvDuration = ref<number | null>(null)

// Expanded visit rows + desk bucket.
const expanded = ref(new Set<string>())
const deskOpen = ref(false)

const openVisitId = ref<string | null>(null)

const staffOptions = computed(() => props.staff.map((s) => ({ id: s.id, label: s.name, sublabel: s.email })))

// --- grouping ---
function byDateDesc(a: TimeEntry, b: TimeEntry) {
  return (b.work_date || '').localeCompare(a.work_date || '')
}
const deskEntries = computed(() => entries.value.filter((e) => !e.visit).sort(byDateDesc))
const entriesByVisit = computed(() => {
  const m = new Map<string, TimeEntry[]>()
  for (const e of entries.value) {
    if (!e.visit) continue
    const arr = m.get(e.visit) || []
    arr.push(e)
    m.set(e.visit, arr)
  }
  for (const arr of m.values()) arr.sort(byDateDesc)
  return m
})

// Needs-scheduling first, then upcoming/recent, then canceled — most
// actionable at top. Within a rank, newest scheduled first.
const RANK: Record<VisitStatus, number> = { requested: 0, scheduled: 1, completed: 2, canceled: 3 }
const orderedVisits = computed(() =>
  [...visits.value].sort((a, b) => {
    if (RANK[a.status] !== RANK[b.status]) return RANK[a.status] - RANK[b.status]
    return (b.scheduled_at || '').localeCompare(a.scheduled_at || '')
  }),
)

const totalMinutes = computed(() => entries.value.reduce((s, e) => s + e.minutes, 0))
const deskMinutes = computed(() => deskEntries.value.reduce((s, e) => s + e.minutes, 0))
const liveVisitCount = computed(() => visits.value.filter((v) => v.status !== 'canceled').length)
// Visits a fresh time entry can be tagged to (canceled ones aren't targets).
const targetVisits = computed(() => orderedVisits.value.filter((v) => v.status !== 'canceled'))

function visitMinutes(id: string): number {
  return (entriesByVisit.value.get(id) || []).reduce((s, e) => s + e.minutes, 0)
}
function fmt(m: number): string {
  const h = Math.floor(m / 60)
  return h > 0 ? `${h}h ${m % 60}m` : `${m}m`
}

const summaryText = computed(() => {
  const est = props.estimatedMinutes || 0
  if (!visits.value.length && !totalMinutes.value) {
    return est ? `0m / ${fmt(est)} est` : 'nothing logged yet'
  }
  const parts: string[] = []
  if (liveVisitCount.value) parts.push(`${liveVisitCount.value} visit${liveVisitCount.value === 1 ? '' : 's'}`)
  parts.push(est ? `${fmt(totalMinutes.value)} / ${fmt(est)} est` : `${fmt(totalMinutes.value)} logged`)
  return parts.join(' · ')
})

const statusClass: Record<VisitStatus, string> = {
  requested: 'badge-soft-warning',
  scheduled: 'badge-soft-info',
  completed: 'badge-soft-success',
  canceled: 'badge-soft-neutral',
}
function rowLabel(v: Visit): string {
  if (v.status === 'requested') return 'needs scheduling'
  if (v.scheduled_at) return format(new Date(v.scheduled_at), 'MMM d, HH:mm')
  return v.status
}
function visitOptionLabel(v: Visit): string {
  const when = v.scheduled_at ? format(new Date(v.scheduled_at), 'MMM d') : 'unscheduled'
  return `On-site — ${when} · ${v.status}`
}

// A timer running on this ticket, and which bucket it belongs to.
const timingHere = computed(() => timer.isTimingTicket(props.ticketId))
const deskTimerActive = computed(() => timingHere.value && !timer.active?.visit)
function visitTimerActive(id: string): boolean {
  return timingHere.value && timer.active?.visit === id
}

async function load() {
  try {
    entries.value = await pb.collection('time_entries').getFullList<TimeEntry>({
      filter: `ticket = '${props.ticketId}'`,
      sort: '-work_date',
      expand: 'staff',
    })
    visits.value = await pb.collection('visits').getFullList<Visit>({
      filter: `ticket = '${props.ticketId}'`,
      sort: 'scheduled_at',
      expand: 'assignee',
    })
  } catch {
    // Optional context card; the ticket view stays usable without it.
  }
}

function toggleVisit(id: string) {
  const s = new Set(expanded.value)
  if (s.has(id)) s.delete(id)
  else s.add(id)
  expanded.value = s
}

function openLogTime(target = '') {
  logTarget.value = target
  logMinutes.value = null
  logNote.value = ''
  logNonBillable.value = false
  error.value = ''
  panel.value = 'time'
}

function openAddVisit() {
  nvLocation.value = ''
  nvNotes.value = ''
  nvScheduleNow.value = false
  nvAt.value = ''
  nvAssignee.value = auth.record?.id || ''
  nvDuration.value = null
  error.value = ''
  panel.value = 'visit'
}

function closePanel() {
  panel.value = ''
}

async function startTimer() {
  error.value = ''
  try {
    await timer.start(props.ticketId, { visit: logTarget.value || undefined })
    closePanel()
  } catch (err: any) {
    error.value = err?.message || 'A timer is already running — stop it first'
    await timer.load()
  }
}

async function logManual() {
  if (!logMinutes.value || logMinutes.value < 1) return
  saving.value = true
  error.value = ''
  try {
    await pb.collection('time_entries').create({
      ticket: props.ticketId,
      staff: auth.record?.id,
      minutes: logMinutes.value,
      work_date: new Date().toISOString(),
      note: logNote.value.trim(),
      visit: logTarget.value || null,
      non_billable: logNonBillable.value,
    })
    closePanel()
    await load()
  } catch (err: any) {
    error.value = err?.message || 'Failed to log time'
  } finally {
    saving.value = false
  }
}

const canAddVisit = computed(() => !nvScheduleNow.value || (!!nvAt.value && !!nvAssignee.value))

async function addVisit() {
  if (!canAddVisit.value) return
  saving.value = true
  error.value = ''
  try {
    const base: Record<string, any> = {
      ticket: props.ticketId,
      location: nvLocation.value.trim(),
      notes: nvNotes.value.trim(),
    }
    if (nvScheduleNow.value) {
      await pb.collection('visits').create({
        ...base,
        status: 'scheduled',
        scheduled_at: new Date(nvAt.value).toISOString(),
        assignee: nvAssignee.value,
        duration_minutes: nvDuration.value || null,
      })
    } else {
      await pb.collection('visits').create({ ...base, status: 'requested' })
    }
    closePanel()
    await load()
  } catch (err: any) {
    error.value = err?.message || 'Failed to add visit'
  } finally {
    saving.value = false
  }
}

async function removeEntry(e: TimeEntry) {
  try {
    await pb.collection('time_entries').delete(e.id)
    await load()
  } catch (err: any) {
    error.value = err?.message || 'Failed to delete entry'
  }
}

// Realtime so time/visits logged elsewhere (the drawer, the work view, another
// agent) reflect here without a manual refresh. Progressive enhancement.
let reloadTimer: ReturnType<typeof setTimeout> | undefined
let unsubTime: (() => void) | null = null
let unsubVisits: (() => void) | null = null
function scheduleReload() {
  clearTimeout(reloadTimer)
  reloadTimer = setTimeout(load, 300)
}
onMounted(async () => {
  await load()
  try {
    unsubTime = await pb.collection('time_entries').subscribe('*', scheduleReload)
    unsubVisits = await pb.collection('visits').subscribe('*', scheduleReload)
  } catch {
    // no realtime; fine.
  }
})
onUnmounted(() => {
  clearTimeout(reloadTimer)
  unsubTime?.()
  unsubVisits?.()
})
</script>

<template>
  <div class="card bg-base-100 shadow-sm">
    <div class="card-body py-4 px-4 space-y-2">
      <!-- Header: title + summary + add actions -->
      <div class="flex items-center gap-2 flex-wrap">
        <h2 class="font-semibold text-sm">Work</h2>
        <span v-if="timingHere" class="inline-flex items-center gap-1 text-xs text-success">
          <span class="inline-flex h-2 w-2 rounded-full bg-success animate-pulse"></span> timing
        </span>
        <span class="text-xs text-base-content/60">{{ summaryText }}</span>
        <div class="ml-auto flex gap-1">
          <button class="btn btn-ghost btn-xs" @click="panel === 'time' ? closePanel() : openLogTime()">
            {{ panel === 'time' ? 'Cancel' : '＋ Log time' }}
          </button>
          <button class="btn btn-ghost btn-xs" @click="panel === 'visit' ? closePanel() : openAddVisit()">
            {{ panel === 'visit' ? 'Cancel' : '＋ Visit' }}
          </button>
        </div>
      </div>

      <div v-if="error" class="text-error text-xs">{{ error }}</div>

      <!-- Add-time panel: timer OR manual, targeted at desk or a visit -->
      <div v-if="panel === 'time'" class="rounded-lg border border-primary/30 bg-primary/5 p-2 space-y-2">
        <div class="flex flex-wrap items-center gap-2">
          <div class="join">
            <button class="btn btn-xs join-item" :class="logMode === 'timer' ? 'btn-primary' : 'btn-ghost'" @click="logMode = 'timer'">▶ Timer</button>
            <button class="btn btn-xs join-item" :class="logMode === 'manual' ? 'btn-primary' : 'btn-ghost'" @click="logMode = 'manual'">✎ Manual</button>
          </div>
          <select v-model="logTarget" class="select select-bordered select-sm flex-1 min-w-[10rem]" :disabled="saving">
            <option value="">Desk work</option>
            <option v-for="v in targetVisits" :key="v.id" :value="v.id">{{ visitOptionLabel(v) }}</option>
          </select>
        </div>

        <div v-if="logMode === 'timer'">
          <button
            v-if="!timingHere"
            class="btn btn-sm btn-primary"
            :disabled="!!timer.active"
            :title="timer.active ? 'Stop your running timer first' : 'Start timing'"
            @click="startTimer"
          >▶ Start timer</button>
          <p v-else class="text-xs text-success">Timing this ticket — stop from the bar above.</p>
        </div>
        <!-- Stacks on phones (number+unit, then a real note textarea, then a
             full-width Log); collapses back to one inline row on sm+. -->
        <div v-else class="flex flex-col gap-2 sm:flex-row sm:items-start sm:gap-1 min-w-0">
          <MinutesInput v-model="logMinutes" size="sm" :disabled="saving" class="shrink-0" />
          <div class="flex flex-1 flex-col gap-1 min-w-0">
            <textarea
              v-model="logNote"
              rows="2"
              placeholder="Note — what you did"
              class="textarea textarea-bordered textarea-sm w-full min-w-0 resize-none"
              :disabled="saving"
            ></textarea>
            <label class="flex items-center gap-2 text-xs cursor-pointer text-base-content/70">
              <input v-model="logNonBillable" type="checkbox" class="checkbox checkbox-xs" :disabled="saving" />
              Non-billable (rework, goodwill — excluded from the customer's total)
            </label>
          </div>
          <button class="btn btn-sm btn-primary shrink-0 w-full sm:w-auto" :disabled="saving || !logMinutes" @click="logManual">Log</button>
        </div>
      </div>

      <!-- Add-visit panel: one progressive form. No schedule = a request the
           dispatcher fills in later; schedule now = pick tech + time here. -->
      <div v-else-if="panel === 'visit'" class="rounded-lg border border-primary/30 bg-primary/5 p-2 space-y-2">
        <input v-model="nvLocation" type="text" placeholder="location (optional)" class="input input-bordered input-sm w-full" :disabled="saving" />
        <textarea v-model="nvNotes" rows="2" placeholder="notes (optional)" class="textarea textarea-bordered textarea-sm w-full" :disabled="saving"></textarea>
        <label class="flex items-center gap-2 text-xs cursor-pointer">
          <input v-model="nvScheduleNow" type="checkbox" class="checkbox checkbox-xs" :disabled="saving" />
          Schedule now (assign a technician and time)
        </label>
        <div v-if="nvScheduleNow" class="space-y-1 pl-1 border-l-2 border-primary/20">
          <input v-model="nvAt" type="datetime-local" class="input input-bordered input-sm w-full min-w-0" :disabled="saving" />
          <input v-model.number="nvDuration" type="number" min="15" step="15" placeholder="duration (min, optional)" class="input input-bordered input-sm w-full" :disabled="saving" />
          <SearchSelect v-model="nvAssignee" :options="staffOptions" size="sm" placeholder="Assign technician…" :disabled="saving" />
        </div>
        <div class="flex justify-end gap-1">
          <button class="btn btn-ghost btn-sm" :disabled="saving" @click="closePanel">Cancel</button>
          <button class="btn btn-primary btn-sm" :disabled="saving || !canAddVisit" @click="addVisit">
            <span v-if="saving" class="loading loading-spinner loading-xs"></span>
            {{ nvScheduleNow ? 'Schedule visit' : 'Request visit' }}
          </button>
        </div>
      </div>

      <!-- Grouped rows: a visit per row (+ a desk bucket) -->
      <div v-if="visits.length" class="divide-y divide-base-200 -mx-1">
        <div v-for="v in orderedVisits" :key="v.id">
          <button
            class="w-full flex items-center gap-2 py-2 px-1 text-sm text-left hover:bg-base-200/50 rounded"
            :aria-expanded="expanded.has(v.id)"
            @click="toggleVisit(v.id)"
          >
            <span class="text-base-content/40 transition-transform shrink-0" :class="{ 'rotate-90': expanded.has(v.id) }">▸</span>
            <span aria-hidden="true">📍</span>
            <span :class="v.status === 'requested' ? 'italic text-base-content/60' : 'font-medium'">{{ rowLabel(v) }}</span>
            <span class="badge-soft shrink-0" :class="statusClass[v.status]">{{ v.status }}</span>
            <span class="flex-1 truncate text-base-content/70">{{ v.expand?.assignee?.name }}</span>
            <span v-if="visitTimerActive(v.id)" class="inline-flex h-2 w-2 rounded-full bg-success animate-pulse shrink-0" title="timing"></span>
            <span class="font-mono text-xs whitespace-nowrap">{{ fmt(visitMinutes(v.id)) }}</span>
          </button>

          <div v-if="expanded.has(v.id)" class="pl-6 pr-1 pb-2 space-y-1">
            <p v-if="v.notes" class="text-xs text-base-content/70 whitespace-pre-wrap">📝 {{ v.notes }}</p>
            <ul v-if="(entriesByVisit.get(v.id) || []).length" class="space-y-1">
              <li v-for="e in entriesByVisit.get(v.id)" :key="e.id" class="flex items-center gap-2 text-sm">
                <span class="text-base-content/60 whitespace-nowrap">{{ format(new Date(e.work_date), 'MMM d') }}</span>
                <span class="flex-1 truncate" :title="e.note">{{ e.note || e.expand?.staff?.name || '' }}</span>
                <BillableTag :entry="e" :editable="e.staff === auth.record?.id || auth.isAdmin" @changed="load" />
                <span class="font-mono whitespace-nowrap">{{ fmt(e.minutes) }}</span>
                <button
                  v-if="e.staff === auth.record?.id || auth.isAdmin"
                  class="btn btn-ghost btn-xs text-error"
                  @click="removeEntry(e)"
                >✕</button>
              </li>
            </ul>
            <p v-else class="text-xs text-base-content/50">No time logged to this visit yet.</p>
            <div class="flex flex-wrap gap-x-3 gap-y-1 pt-0.5">
              <button class="link link-hover text-xs text-primary" @click="openLogTime(v.id)">＋ Log time here</button>
              <button class="link link-hover text-xs" @click="openVisitId = v.id">Details / manage</button>
            </div>
          </div>
        </div>

        <!-- Desk work bucket -->
        <div v-if="deskEntries.length || deskTimerActive">
          <button
            class="w-full flex items-center gap-2 py-2 px-1 text-sm text-left hover:bg-base-200/50 rounded"
            :aria-expanded="deskOpen"
            @click="deskOpen = !deskOpen"
          >
            <span class="text-base-content/40 transition-transform shrink-0" :class="{ 'rotate-90': deskOpen }">▸</span>
            <span aria-hidden="true">🖥</span>
            <span class="font-medium">Desk work</span>
            <span class="flex-1 text-base-content/50">no visit</span>
            <span v-if="deskTimerActive" class="inline-flex h-2 w-2 rounded-full bg-success animate-pulse shrink-0" title="timing"></span>
            <span class="font-mono text-xs whitespace-nowrap">{{ fmt(deskMinutes) }}</span>
          </button>
          <ul v-if="deskOpen" class="pl-6 pr-1 pb-2 space-y-1">
            <li v-for="e in deskEntries" :key="e.id" class="flex items-center gap-2 text-sm">
              <span class="text-base-content/60 whitespace-nowrap">{{ format(new Date(e.work_date), 'MMM d') }}</span>
              <span class="flex-1 truncate" :title="e.note">{{ e.note || e.expand?.staff?.name || '' }}</span>
              <BillableTag :entry="e" :editable="e.staff === auth.record?.id || auth.isAdmin" @changed="load" />
              <span class="font-mono whitespace-nowrap">{{ fmt(e.minutes) }}</span>
              <button
                v-if="e.staff === auth.record?.id || auth.isAdmin"
                class="btn btn-ghost btn-xs text-error"
                @click="removeEntry(e)"
              >✕</button>
            </li>
          </ul>
        </div>
      </div>

      <!-- No visits: a plain time list, no bucket chrome -->
      <template v-else>
        <ul v-if="deskEntries.length" class="space-y-1">
          <li v-for="e in deskEntries" :key="e.id" class="flex items-center gap-2 text-sm">
            <span class="text-base-content/60 whitespace-nowrap">{{ format(new Date(e.work_date), 'MMM d') }}</span>
            <span class="flex-1 truncate" :title="e.note">{{ e.note || e.expand?.staff?.name || '' }}</span>
            <span class="font-mono whitespace-nowrap">{{ fmt(e.minutes) }}</span>
            <button
              v-if="e.staff === auth.record?.id || auth.isAdmin"
              class="btn btn-ghost btn-xs text-error"
              @click="removeEntry(e)"
            >✕</button>
          </li>
        </ul>
        <p v-else class="text-xs text-base-content/50">No time logged yet. Use ＋ Log time to start the clock or enter minutes.</p>
      </template>
    </div>
  </div>

  <VisitDetailDrawer :visit-id="openVisitId" :staff="staff" @close="openVisitId = null" @changed="load" />
</template>
