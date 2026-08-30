<script setup lang="ts">
// Project detail / create / edit. Handles both create (/staff/projects/new) and
// edit (/staff/projects/:id) in one view — consistent with LocationDetailView.
// The linked tickets, the visits, and the DERIVED rollups — crew (lead ∪
// ticket/visit assignees) and total logged time — are read-only and only
// meaningful once the project exists. Nothing here is a second source of truth:
// crew and time are computed live from the project's tickets, never stored.
//
// Unlike LocationDetailView, view mode is NOT the form greyed out. A location
// record is mostly a form and is opened to be edited; a project is the status
// page for a rollout and is read many times for every time it is changed, so a
// disabled <input> holding the title and a fixed four-row textarea holding the
// scope — the same height empty or full — were furniture in the way of the
// answer. Editing swaps each card to its controls; nothing else changes shape.
//
// Both record lists are bounded and expand in place rather than linking out.
// A "View all →" has nowhere to point: the staff queue carries no project
// filter, and this is the only page that holds a project's tickets.
import { computed, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { pb } from '@/pb'
import { useAuthStore } from '@/stores/auth'
import type { Customer, Location, Project, Staff, Ticket, TimeEntry, Visit } from '@/types'
import { PROJECT_STATUSES } from '@/types'
import SearchSelect from '@/components/SearchSelect.vue'
import TicketBadges from '@/components/TicketBadges.vue'
import { format } from 'date-fns'

const route = useRoute()
const router = useRouter()
const auth = useAuthStore()

const id = computed(() => route.params.id as string | undefined)
const isCreate = computed(() => !id.value)

const PAGE = 15

const project = ref<Project | null>(null)
const tickets = ref<Ticket[]>([])
const ticketTotal = ref(0)
const visits = ref<Visit[]>([])
const entries = ref<TimeEntry[]>([])
const staff = ref<Staff[]>([])
const customers = ref<Customer[]>([])
const locations = ref<Location[]>([])
const loading = ref(true)
const saving = ref(false)
const error = ref('')
// View/edit toggle. Create starts unlocked (nothing to view); edit starts locked.
const editing = ref(false)
const allTickets = ref(false)
const allVisits = ref(false)

// Editable copy of the header fields.
const form = ref({
  customer: '',
  title: '',
  status: 'pending',
  description: '',
  location: '',
  lead: '',
  start_date: '',
  target_date: '',
})

const customerOptions = computed(() => customers.value.map((c) => ({ id: c.id, label: c.name })))
const staffOptions = computed(() => staff.value.map((s) => ({ id: s.id, label: s.name, sublabel: s.email })))
const locationOptions = computed(() =>
  locations.value.map((l) => ({ id: l.id, label: l.name, sublabel: l.code || l.address || undefined })),
)

// Crew: everyone touching the project — the lead, plus the assignee of any
// ticket or visit. Deduped by staff id; names come from the loaded roster.
const staffName = computed(() => new Map(staff.value.map((s) => [s.id, s.name])))
const crew = computed(() => {
  const ids = new Set<string>()
  if (project.value?.lead) ids.add(project.value.lead)
  for (const t of tickets.value) if (t.assignee) ids.add(t.assignee)
  for (const v of visits.value) if (v.assignee) ids.add(v.assignee)
  return [...ids].map((sid) => staffName.value.get(sid) || 'Unknown').sort()
})

function fmt(m: number): string {
  if (!m) return '0m'
  const h = Math.floor(m / 60)
  const min = m % 60
  return h ? `${h}h${min ? ' ' + min + 'm' : ''}` : `${min}m`
}
function fmtDate(s?: string): string {
  return s ? format(new Date(s), 'MMM d, yyyy') : ''
}
function fmtDateTime(s?: string): string {
  return s ? format(new Date(s), 'EEE, MMM d · HH:mm') : ''
}

const totalMinutes = computed(() => entries.value.reduce((sum, e) => sum + (e.minutes || 0), 0))
const totalTime = computed(() => fmt(totalMinutes.value))
const billableMinutes = computed(() =>
  entries.value.reduce((sum, e) => sum + (e.non_billable ? 0 : e.minutes || 0), 0),
)

// Estimated-vs-actual rollup: summed ticket estimates against logged time.
// Derived like crew/total time — nothing stored on the project. It sums EVERY
// ticket on the project, not the page of them currently rendered, so it has
// its own trimmed query rather than folding the visible list.
const totalEstimated = ref(0)
const estimatedTime = computed(() => fmt(totalEstimated.value))
const estPct = computed(() =>
  totalEstimated.value ? Math.round((totalMinutes.value / totalEstimated.value) * 100) : 0,
)
const overEstimate = computed(() => totalEstimated.value > 0 && totalMinutes.value > totalEstimated.value)

const leadName = computed(() => (project.value?.lead ? staffName.value.get(project.value.lead) || '—' : ''))

function applyRecord(p: Project) {
  form.value = {
    customer: p.customer,
    title: p.title,
    status: p.status,
    description: p.description || '',
    location: p.location || '',
    lead: p.lead || '',
    start_date: (p.start_date || '').slice(0, 10),
    target_date: (p.target_date || '').slice(0, 10),
  }
}

async function loadLocations(customerId: string) {
  locations.value = customerId
    ? await pb.collection('locations').getFullList<Location>({ filter: `customer = '${customerId}'`, sort: 'name' })
    : []
}

async function loadTickets() {
  const res = await pb.collection('tickets').getList<Ticket>(1, allTickets.value ? 500 : PAGE, {
    filter: `project = '${id.value}'`,
    sort: '-created',
    expand: 'assignee',
  })
  tickets.value = res.items
  ticketTotal.value = res.totalItems
}

async function loadVisits() {
  // Relation-hop filter: visits whose ticket belongs to this project. Fetched
  // whole because crew needs every assignee, but trimmed to the columns the
  // card and the crew set actually read.
  visits.value = await pb.collection('visits').getFullList<Visit>({
    filter: `ticket.project = '${id.value}'`,
    sort: 'scheduled_at',
    fields: 'id,ticket,assignee,status,scheduled_at,location',
  })
}

async function load() {
  loading.value = true
  error.value = ''
  try {
    staff.value = await pb.collection('staff').getFullList<Staff>({ sort: 'name', filter: 'active = true' })
    if (isCreate.value) {
      customers.value = await pb.collection('customers').getFullList<Customer>({ sort: 'name', filter: 'active = true' })
      editing.value = true
    } else {
      project.value = await pb.collection('projects').getOne<Project>(id.value!, { expand: 'customer,location,lead' })
      applyRecord(project.value)
      editing.value = false
      await Promise.all([
        loadTickets(),
        loadVisits(),
        // Minutes only. Summing needs every row, but a rollout's ledger is the
        // largest thing on this page and none of the rest of a time entry —
        // note, work_date, visit, staff — is read here.
        pb
          .collection('time_entries')
          .getFullList<TimeEntry>({ filter: `ticket.project = '${id.value}'`, fields: 'id,minutes,non_billable' })
          .then((rows) => (entries.value = rows)),
        // Estimates likewise sum across ALL tickets, not the visible page, so
        // they come from their own trimmed query rather than the paged list.
        pb
          .collection('tickets')
          .getFullList<Ticket>({ filter: `project = '${id.value}'`, fields: 'id,estimated_minutes' })
          .then((rows) => (totalEstimated.value = rows.reduce((s, t) => s + (t.estimated_minutes || 0), 0))),
      ])
    }
  } catch (err: any) {
    error.value = err?.message || 'Failed to load project'
  } finally {
    loading.value = false
  }
}

async function save() {
  if (!form.value.title.trim() || !form.value.customer) return
  saving.value = true
  error.value = ''
  const data = {
    customer: form.value.customer,
    title: form.value.title.trim(),
    status: form.value.status,
    description: form.value.description.trim(),
    location: form.value.location,
    lead: form.value.lead,
    start_date: form.value.start_date || '',
    target_date: form.value.target_date || '',
  }
  try {
    if (isCreate.value) {
      const rec = await pb.collection('projects').create<Project>(data)
      router.replace(`/staff/projects/${rec.id}`)
      return
    }
    project.value = await pb
      .collection('projects')
      .update<Project>(id.value!, data, { expand: 'customer,location,lead' })
    editing.value = false
  } catch (err: any) {
    error.value = err?.message || 'Failed to save'
  } finally {
    saving.value = false
  }
}

function startEdit() {
  editing.value = true
}

function cancelEdit() {
  if (isCreate.value) {
    router.push('/staff/projects')
    return
  }
  if (project.value) applyRecord(project.value)
  editing.value = false
}

function expandTickets() {
  allTickets.value = true
  loadTickets()
}

const visibleVisits = computed(() => (allVisits.value ? visits.value : visits.value.slice(0, PAGE)))

const statusClass: Record<string, string> = {
  pending: 'badge-soft-neutral',
  active: 'badge-soft-info',
  completed: 'badge-soft-success',
  canceled: 'badge-soft-neutral opacity-60',
}
const visitBadge: Record<string, string> = {
  requested: 'badge-soft-neutral',
  scheduled: 'badge-soft-info',
  completed: 'badge-soft-success',
  canceled: 'badge-soft-neutral opacity-60',
}

onMounted(load)
// Create flow router.replace()s from /new to /:id, reusing this instance
// (onMounted won't refire) — reload so the freshly created record's expands,
// tickets and rollups populate and the view locks.
watch(() => route.params.id, load)
// Location options follow the selected customer (create mode) or the loaded
// record (edit mode).
watch(() => form.value.customer, (c) => loadLocations(c))
</script>

<template>
  <div class="max-w-6xl mx-auto space-y-4">
    <div class="flex items-center justify-between gap-2 flex-wrap">
      <div class="breadcrumbs text-sm">
        <ul>
          <li><router-link to="/staff/projects">Projects</router-link></li>
          <li>{{ isCreate ? 'New project' : project ? `#${project.number}` : '…' }}</li>
        </ul>
      </div>
      <!-- Field agents get read-only projects: no edit/create affordances. -->
      <div v-if="!loading && !auth.isField" class="flex gap-2">
        <template v-if="editing">
          <button class="btn btn-ghost btn-sm" :disabled="saving" @click="cancelEdit">Cancel</button>
          <button class="btn btn-primary btn-sm" :disabled="saving || !form.title.trim() || !form.customer" @click="save">
            <span v-if="saving" class="loading loading-spinner loading-xs"></span>
            {{ isCreate ? 'Create' : 'Save' }}
          </button>
        </template>
        <button v-else class="btn btn-primary btn-sm" @click="startEdit">Edit</button>
      </div>
    </div>

    <div v-if="error" class="alert alert-error py-2 text-sm">{{ error }}</div>
    <div v-if="loading" class="flex justify-center p-12"><span class="loading loading-spinner loading-lg"></span></div>

    <template v-else>
      <div class="flex flex-col xl:flex-row gap-4 items-start">
        <!-- Main: header + linked tickets + visits -->
        <div class="flex-1 w-full min-w-0 space-y-4">
          <div class="card bg-base-100 shadow-sm">
            <div class="card-body gap-3">
              <!-- Read: the rollout as a page. -->
              <template v-if="!editing && project">
                <div class="flex items-center gap-2 flex-wrap">
                  <span class="badge-soft" :class="statusClass[project.status]">{{ project.status }}</span>
                  <h1 class="text-xl font-bold">{{ project.title }}</h1>
                </div>
                <div class="text-sm text-base-content/60">
                  {{ project.expand?.customer?.name }}
                  <template v-if="project.expand?.location">
                    ·
                    <router-link :to="`/staff/locations/${project.location}`" class="link link-hover">
                      📍 {{ project.expand.location.name }}
                    </router-link>
                  </template>
                </div>
                <p v-if="project.description" class="text-sm whitespace-pre-wrap">{{ project.description }}</p>
                <p v-else class="text-sm text-base-content/40 italic">No scope described.</p>
              </template>

              <!-- Write. -->
              <template v-else>
                <div v-if="isCreate" class="form-control">
                  <label class="label py-1"><span class="label-text text-xs">Customer *</span></label>
                  <SearchSelect v-model="form.customer" :options="customerOptions" size="sm" placeholder="Customer…" :disabled="saving" />
                </div>
                <div class="form-control">
                  <label class="label py-1"><span class="label-text text-xs">Title</span></label>
                  <input v-model="form.title" type="text" maxlength="300" placeholder="e.g. HQ Security Rollout" class="input input-bordered" :disabled="saving" />
                </div>
                <div class="form-control">
                  <label class="label py-1"><span class="label-text text-xs">Description / scope</span></label>
                  <textarea v-model="form.description" rows="4" class="textarea textarea-bordered" :disabled="saving"></textarea>
                </div>
              </template>
            </div>
          </div>

          <!-- Linked tickets (edit mode only — needs a persisted project) -->
          <div v-if="!isCreate" class="card bg-base-100 shadow-sm">
            <div class="card-body">
              <div class="flex items-center justify-between">
                <h2 class="font-semibold">Tickets <span class="text-base-content/50 font-normal">({{ ticketTotal }})</span></h2>
                <router-link v-if="!auth.isField" to="/staff/tickets/new" class="btn btn-ghost btn-xs">＋ New ticket</router-link>
              </div>
              <div class="divide-y divide-base-200">
                <router-link
                  v-for="t in tickets"
                  :key="t.id"
                  :to="`/staff/tickets/${t.id}`"
                  class="flex items-center gap-3 py-2 hover:bg-base-200/50 -mx-2 px-2 rounded"
                >
                  <span class="font-mono text-xs text-base-content/50 w-10">#{{ t.number }}</span>
                  <span v-if="t.type === 'planned'" class="badge-soft badge-soft-primary">planned</span>
                  <span class="flex-1 truncate">{{ t.title }}</span>
                  <span v-if="t.estimated_minutes" class="text-xs text-base-content/50 hidden sm:block whitespace-nowrap" title="Estimated effort">~{{ fmt(t.estimated_minutes) }}</span>
                  <span class="text-xs text-base-content/60 hidden sm:block">{{ t.expand?.assignee?.name || 'Unassigned' }}</span>
                  <TicketBadges :status="t.status" :priority="t.priority" />
                </router-link>
                <p v-if="ticketTotal === 0" class="py-3 text-sm text-base-content/50">
                  No tickets yet. Create tickets and set their Project to this one.
                </p>
              </div>
              <button v-if="ticketTotal > tickets.length" class="btn btn-ghost btn-xs self-start" @click="expandTickets">
                Show all {{ ticketTotal }} →
              </button>
            </div>
          </div>

          <!-- Visits. These were already fetched to build the crew set and then
               thrown away, so a rollout's lead could see who was on it but not
               when anyone was going. -->
          <div v-if="!isCreate && visits.length" class="card bg-base-100 shadow-sm">
            <div class="card-body">
              <h2 class="font-semibold">Visits <span class="text-base-content/50 font-normal">({{ visits.length }})</span></h2>
              <div class="divide-y divide-base-200">
                <router-link
                  v-for="v in visibleVisits"
                  :key="v.id"
                  :to="`/staff/tickets/${v.ticket}`"
                  class="flex items-center gap-3 py-2 hover:bg-base-200/50 -mx-2 px-2 rounded"
                >
                  <span class="badge-soft" :class="visitBadge[v.status]">{{ v.status }}</span>
                  <span class="flex-1 text-sm">
                    <span v-if="v.scheduled_at">{{ fmtDateTime(v.scheduled_at) }}</span>
                    <span v-else class="text-base-content/50">Not yet scheduled</span>
                  </span>
                  <span class="text-xs text-base-content/60 hidden sm:block">
                    {{ v.assignee ? staffName.get(v.assignee) || 'Unknown' : 'Unassigned' }}
                  </span>
                </router-link>
              </div>
              <button v-if="visits.length > visibleVisits.length" class="btn btn-ghost btn-xs self-start" @click="allVisits = true">
                Show all {{ visits.length }} →
              </button>
            </div>
          </div>
        </div>

        <!-- Rail: schedule, assignment, rollups -->
        <div class="w-full xl:w-80 space-y-4">
          <div class="card bg-base-100 shadow-sm">
            <div class="card-body py-4 px-4 space-y-3">
              <!-- Read: a fact list, not four disabled controls. -->
              <dl v-if="!editing && project" class="text-sm space-y-2">
                <div class="flex items-center justify-between gap-2">
                  <dt class="text-base-content/60 text-xs">Status</dt>
                  <dd><span class="badge-soft" :class="statusClass[project.status]">{{ project.status }}</span></dd>
                </div>
                <div class="flex items-center justify-between gap-2">
                  <dt class="text-base-content/60 text-xs">Lead</dt>
                  <dd :class="leadName ? '' : 'text-base-content/40'">{{ leadName || 'Unassigned' }}</dd>
                </div>
                <div class="flex items-center justify-between gap-2">
                  <dt class="text-base-content/60 text-xs">Location</dt>
                  <dd class="text-right">
                    <router-link
                      v-if="project.expand?.location"
                      :to="`/staff/locations/${project.location}`"
                      class="link link-hover"
                    >{{ project.expand.location.name }}</router-link>
                    <span v-else class="text-base-content/40">None</span>
                  </dd>
                </div>
                <div class="flex items-center justify-between gap-2">
                  <dt class="text-base-content/60 text-xs">Window</dt>
                  <dd :class="project.start_date || project.target_date ? '' : 'text-base-content/40'">
                    <template v-if="project.start_date || project.target_date">
                      <span v-if="project.start_date">{{ fmtDate(project.start_date) }} → </span>
                      {{ fmtDate(project.target_date) || 'open' }}
                    </template>
                    <template v-else>Not set</template>
                  </dd>
                </div>
              </dl>

              <!-- Write. -->
              <template v-else>
                <div class="form-control">
                  <label class="label py-1"><span class="label-text text-xs">Status</span></label>
                  <select v-model="form.status" class="select select-bordered select-sm" :disabled="saving">
                    <option v-for="s in PROJECT_STATUSES" :key="s" :value="s">{{ s }}</option>
                  </select>
                </div>
                <div class="form-control">
                  <label class="label py-1"><span class="label-text text-xs">Lead</span></label>
                  <SearchSelect v-model="form.lead" :options="staffOptions" size="sm" empty-label="None" placeholder="Project lead…" :disabled="saving" />
                </div>
                <div class="form-control">
                  <label class="label py-1"><span class="label-text text-xs">Location</span></label>
                  <SearchSelect v-model="form.location" :options="locationOptions" size="sm" empty-label="None" placeholder="Location…" :disabled="saving" />
                </div>
                <div class="flex gap-2">
                  <div class="form-control flex-1">
                    <label class="label py-1"><span class="label-text text-xs">Start</span></label>
                    <input v-model="form.start_date" type="date" class="input input-bordered input-sm" :disabled="saving" />
                  </div>
                  <div class="form-control flex-1">
                    <label class="label py-1"><span class="label-text text-xs">Target</span></label>
                    <input v-model="form.target_date" type="date" class="input input-bordered input-sm" :disabled="saving" />
                  </div>
                </div>
              </template>
            </div>
          </div>

          <!-- Derived rollups (edit mode only) -->
          <div v-if="!isCreate" class="card bg-base-100 shadow-sm">
            <div class="card-body py-4 px-4 space-y-3">
              <div>
                <div class="text-xs text-base-content/60 mb-1">Crew ({{ crew.length }})</div>
                <div v-if="crew.length" class="flex flex-wrap gap-1">
                  <span v-for="name in crew" :key="name" class="badge-soft badge-soft-neutral">{{ name }}</span>
                </div>
                <div v-else class="text-sm text-base-content/50">No one assigned yet.</div>
              </div>
              <div>
                <div class="flex items-center justify-between mb-1">
                  <span class="text-xs text-base-content/60">Time logged</span>
                  <span class="font-semibold">{{ totalTime }}</span>
                </div>
                <template v-if="totalEstimated">
                  <progress
                    class="progress w-full"
                    :class="overEstimate ? 'progress-error' : 'progress-primary'"
                    :value="Math.min(totalMinutes, totalEstimated)"
                    :max="totalEstimated"
                  ></progress>
                  <div class="flex items-center justify-between text-[11px] text-base-content/50 mt-0.5">
                    <span>{{ estPct }}% of {{ estimatedTime }} est</span>
                    <span v-if="overEstimate" class="text-error">+{{ fmt(totalMinutes - totalEstimated) }} over</span>
                  </div>
                </template>
                <div v-else class="text-[11px] text-base-content/40">No estimate set on these tickets.</div>
                <div v-if="totalMinutes && billableMinutes !== totalMinutes" class="text-[11px] text-base-content/50 mt-1">
                  {{ fmt(billableMinutes) }} billable · {{ fmt(totalMinutes - billableMinutes) }} written off
                </div>
              </div>
              <p class="text-[11px] text-base-content/40 leading-snug">
                Crew, estimate, and time are derived from this project's tickets and
                visits — the ticket stays the ledger.
              </p>
            </div>
          </div>
        </div>
      </div>
    </template>
  </div>
</template>
