<script setup lang="ts">
// Project detail: the header fields (editable), the linked tickets that make
// up the work, and the DERIVED rollups — crew (lead ∪ ticket/visit assignees)
// and total logged time. Nothing here is a second source of truth: crew and
// time are computed live from the project's tickets, never stored.
import { computed, onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { pb } from '@/pb'
import type { Location, Project, Staff, Ticket, TimeEntry, Visit } from '@/types'
import { PROJECT_STATUSES } from '@/types'
import SearchSelect from '@/components/SearchSelect.vue'
import TicketBadges from '@/components/TicketBadges.vue'
import { format } from 'date-fns'

const route = useRoute()
const router = useRouter()
const id = route.params.id as string

const project = ref<Project | null>(null)
const tickets = ref<Ticket[]>([])
const visits = ref<Visit[]>([])
const entries = ref<TimeEntry[]>([])
const staff = ref<Staff[]>([])
const locations = ref<Location[]>([])
const loading = ref(true)
const saving = ref(false)
const error = ref('')

// Editable copy of the header fields.
const form = ref({
  title: '',
  status: 'planned',
  description: '',
  location: '',
  lead: '',
  start_date: '',
  target_date: '',
})

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

const totalMinutes = computed(() => entries.value.reduce((sum, e) => sum + (e.minutes || 0), 0))
const totalTime = computed(() => {
  const m = totalMinutes.value
  if (!m) return '0m'
  const h = Math.floor(m / 60)
  const min = m % 60
  return h ? `${h}h${min ? ' ' + min + 'm' : ''}` : `${min}m`
})

function fmtDate(s?: string): string {
  return s ? format(new Date(s), 'MMM d, yyyy') : '—'
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
    project.value = await pb.collection('projects').getOne<Project>(id, { expand: 'customer,location,lead' })
    form.value = {
      title: project.value.title,
      status: project.value.status,
      description: project.value.description || '',
      location: project.value.location || '',
      lead: project.value.lead || '',
      start_date: (project.value.start_date || '').slice(0, 10),
      target_date: (project.value.target_date || '').slice(0, 10),
    }
    staff.value = await pb.collection('staff').getFullList<Staff>({ sort: 'name', filter: 'active = true' })
    await loadLocations(project.value.customer)
    tickets.value = await pb.collection('tickets').getFullList<Ticket>({
      filter: `project = '${id}'`,
      sort: '-created',
      expand: 'assignee',
    })
    // Relation-hop filters: visits/time whose ticket belongs to this project.
    visits.value = await pb.collection('visits').getFullList<Visit>({ filter: `ticket.project = '${id}'` })
    entries.value = await pb.collection('time_entries').getFullList<TimeEntry>({ filter: `ticket.project = '${id}'` })
  } catch (err: any) {
    error.value = err?.message || 'Failed to load project'
  } finally {
    loading.value = false
  }
}

async function save() {
  if (!form.value.title.trim()) return
  saving.value = true
  error.value = ''
  try {
    project.value = await pb.collection('projects').update<Project>(
      id,
      {
        title: form.value.title.trim(),
        status: form.value.status,
        description: form.value.description.trim(),
        location: form.value.location,
        lead: form.value.lead,
        start_date: form.value.start_date || '',
        target_date: form.value.target_date || '',
      },
      { expand: 'customer,location,lead' },
    )
  } catch (err: any) {
    error.value = err?.message || 'Failed to save'
  } finally {
    saving.value = false
  }
}

const statusClass: Record<string, string> = {
  planned: 'badge-soft-neutral',
  active: 'badge-soft-info',
  completed: 'badge-soft-success',
  canceled: 'badge-soft-neutral opacity-60',
}

onMounted(load)
</script>

<template>
  <div class="max-w-6xl mx-auto space-y-4">
    <div class="breadcrumbs text-sm">
      <ul>
        <li><a @click="router.push('/staff/projects')">Projects</a></li>
        <li>{{ project ? `#${project.number}` : '…' }}</li>
      </ul>
    </div>

    <div v-if="error" class="alert alert-error py-2 text-sm">{{ error }}</div>
    <div v-if="loading" class="flex justify-center p-12"><span class="loading loading-spinner loading-lg"></span></div>

    <template v-else-if="project">
      <div class="flex flex-col xl:flex-row gap-4 items-start">
        <!-- Main: editable header + linked tickets -->
        <div class="flex-1 w-full min-w-0 space-y-4">
          <div class="card bg-base-100 shadow-sm">
            <div class="card-body space-y-3">
              <div class="flex items-center gap-2 flex-wrap">
                <span class="badge-soft" :class="statusClass[project.status]">{{ project.status }}</span>
                <span class="text-base-content/60 text-sm">
                  {{ project.expand?.customer?.name }}
                  <template v-if="project.expand?.location"> · 📍 {{ project.expand.location.name }}</template>
                </span>
              </div>

              <div class="form-control">
                <label class="label py-1"><span class="label-text text-xs">Title</span></label>
                <input v-model="form.title" type="text" maxlength="300" class="input input-bordered" :disabled="saving" />
              </div>
              <div class="form-control">
                <label class="label py-1"><span class="label-text text-xs">Description / scope</span></label>
                <textarea v-model="form.description" rows="4" class="textarea textarea-bordered" :disabled="saving"></textarea>
              </div>
              <div class="flex justify-end">
                <button class="btn btn-primary btn-sm" :disabled="saving || !form.title.trim()" @click="save">
                  <span v-if="saving" class="loading loading-spinner loading-xs"></span>
                  Save
                </button>
              </div>
            </div>
          </div>

          <!-- Linked tickets -->
          <div class="card bg-base-100 shadow-sm">
            <div class="card-body">
              <div class="flex items-center justify-between">
                <h2 class="font-semibold">Tickets <span class="text-base-content/50 font-normal">({{ tickets.length }})</span></h2>
                <router-link to="/staff/tickets/new" class="btn btn-ghost btn-xs">＋ New ticket</router-link>
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
                <SearchSelect v-model="form.location" :options="locationOptions" size="sm" empty-label="None" placeholder="Site…" :disabled="saving" />
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
            </div>
          </div>

          <!-- Derived rollups -->
          <div class="card bg-base-100 shadow-sm">
            <div class="card-body py-4 px-4 space-y-3">
              <div>
                <div class="text-xs text-base-content/60 mb-1">Crew ({{ crew.length }})</div>
                <div v-if="crew.length" class="flex flex-wrap gap-1">
                  <span v-for="name in crew" :key="name" class="badge-soft badge-soft-neutral">{{ name }}</span>
                </div>
                <div v-else class="text-sm text-base-content/50">No one assigned yet.</div>
              </div>
              <div class="flex items-center justify-between">
                <span class="text-xs text-base-content/60">Total time logged</span>
                <span class="font-semibold">{{ totalTime }}</span>
              </div>
              <p class="text-[11px] text-base-content/40 leading-snug">
                Crew and time are derived from this project's tickets and visits — the
                ticket stays the ledger.
              </p>
            </div>
          </div>
        </div>
      </div>
    </template>
  </div>
</template>
