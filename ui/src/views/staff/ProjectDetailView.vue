<script setup lang="ts">
// Project detail / create / edit. Handles both create (/staff/projects/new) and
// edit (/staff/projects/:id) in one view — consistent with LocationDetailView.
// The header fields toggle between a locked "view" and an unlocked "edit" mode
// (any staff may edit); the linked tickets and the DERIVED rollups — crew
// (lead ∪ ticket/visit assignees) and total logged time — are read-only and
// only meaningful once the project exists. Nothing here is a second source of
// truth: crew and time are computed live from the project's tickets, never
// stored.
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

const project = ref<Project | null>(null)
const tickets = ref<Ticket[]>([])
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

// Editable copy of the header fields.
const form = ref({
  customer: '',
  title: '',
  status: 'planned',
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

const totalMinutes = computed(() => entries.value.reduce((sum, e) => sum + (e.minutes || 0), 0))
const totalTime = computed(() => fmt(totalMinutes.value))

// Estimated-vs-actual rollup: summed ticket estimates against logged time.
// Derived like crew/total time — nothing stored on the project.
const totalEstimated = computed(() => tickets.value.reduce((sum, t) => sum + (t.estimated_minutes || 0), 0))
const estimatedTime = computed(() => fmt(totalEstimated.value))
const estPct = computed(() =>
  totalEstimated.value ? Math.round((totalMinutes.value / totalEstimated.value) * 100) : 0,
)
const overEstimate = computed(() => totalEstimated.value > 0 && totalMinutes.value > totalEstimated.value)

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
      tickets.value = await pb.collection('tickets').getFullList<Ticket>({
        filter: `project = '${id.value}'`,
        sort: '-created',
        expand: 'assignee',
      })
      // Relation-hop filters: visits/time whose ticket belongs to this project.
      visits.value = await pb.collection('visits').getFullList<Visit>({ filter: `ticket.project = '${id.value}'` })
      entries.value = await pb.collection('time_entries').getFullList<TimeEntry>({ filter: `ticket.project = '${id.value}'` })
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

const statusClass: Record<string, string> = {
  planned: 'badge-soft-neutral',
  active: 'badge-soft-info',
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
          <li><a @click="router.push('/staff/projects')">Projects</a></li>
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
        <!-- Main: editable header + linked tickets -->
        <div class="flex-1 w-full min-w-0 space-y-4">
          <div class="card bg-base-100 shadow-sm">
            <div class="card-body space-y-3">
              <div v-if="!isCreate && project" class="flex items-center gap-2 flex-wrap">
                <span class="badge-soft" :class="statusClass[project.status]">{{ project.status }}</span>
                <span class="text-base-content/60 text-sm">
                  {{ project.expand?.customer?.name }}
                  <template v-if="project.expand?.location"> · 📍 {{ project.expand.location.name }}</template>
                </span>
              </div>

              <div v-if="isCreate" class="form-control">
                <label class="label py-1"><span class="label-text text-xs">Customer *</span></label>
                <SearchSelect v-model="form.customer" :options="customerOptions" size="sm" placeholder="Customer…" :disabled="saving" />
              </div>
              <div class="form-control">
                <label class="label py-1"><span class="label-text text-xs">Title</span></label>
                <input v-model="form.title" type="text" maxlength="300" placeholder="e.g. HQ Security Rollout" class="input input-bordered" :disabled="!editing || saving" />
              </div>
              <div class="form-control">
                <label class="label py-1"><span class="label-text text-xs">Description / scope</span></label>
                <textarea v-model="form.description" rows="4" class="textarea textarea-bordered" :disabled="!editing || saving"></textarea>
              </div>
            </div>
          </div>

          <!-- Linked tickets (edit mode only — needs a persisted project) -->
          <div v-if="!isCreate" class="card bg-base-100 shadow-sm">
            <div class="card-body">
              <div class="flex items-center justify-between">
                <h2 class="font-semibold">Tickets <span class="text-base-content/50 font-normal">({{ tickets.length }})</span></h2>
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
                  <span v-if="t.type === 'install'" class="badge-soft badge-soft-primary">install</span>
                  <span class="flex-1 truncate">{{ t.title }}</span>
                  <span v-if="t.estimated_minutes" class="text-xs text-base-content/50 hidden sm:block whitespace-nowrap" title="Estimated effort">~{{ fmt(t.estimated_minutes) }}</span>
                  <span class="text-xs text-base-content/60 hidden sm:block">{{ t.expand?.assignee?.name || 'Unassigned' }}</span>
                  <TicketBadges :status="t.status" :priority="t.priority" />
                </router-link>
                <p v-if="tickets.length === 0" class="py-3 text-sm text-base-content/50">
                  No tickets yet. Create tickets and set their Project to this one.
                </p>
              </div>
            </div>
          </div>
        </div>

        <!-- Rail: schedule, assignment, rollups -->
        <div class="w-full xl:w-80 space-y-4">
          <div class="card bg-base-100 shadow-sm">
            <div class="card-body py-4 px-4 space-y-3">
              <div class="form-control">
                <label class="label py-1"><span class="label-text text-xs">Status</span></label>
                <select v-model="form.status" class="select select-bordered select-sm" :disabled="!editing || saving">
                  <option v-for="s in PROJECT_STATUSES" :key="s" :value="s">{{ s }}</option>
                </select>
              </div>
              <div class="form-control">
                <label class="label py-1"><span class="label-text text-xs">Lead</span></label>
                <SearchSelect v-model="form.lead" :options="staffOptions" size="sm" empty-label="None" placeholder="Project lead…" :disabled="!editing || saving" />
              </div>
              <div class="form-control">
                <label class="label py-1"><span class="label-text text-xs">Location</span></label>
                <SearchSelect v-model="form.location" :options="locationOptions" size="sm" empty-label="None" placeholder="Site…" :disabled="!editing || saving" />
              </div>
              <div class="flex gap-2">
                <div class="form-control flex-1">
                  <label class="label py-1"><span class="label-text text-xs">Start</span></label>
                  <input v-model="form.start_date" type="date" class="input input-bordered input-sm" :disabled="!editing || saving" />
                </div>
                <div class="form-control flex-1">
                  <label class="label py-1"><span class="label-text text-xs">Target</span></label>
                  <input v-model="form.target_date" type="date" class="input input-bordered input-sm" :disabled="!editing || saving" />
                </div>
              </div>
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
