<script setup lang="ts">
// One maintenance plan: the schedule, the triage it stamps onto every ticket it
// opens, and the tickets it has opened so far. Serves both /new and /:id, with
// the create-opens-unlocked / existing-opens-locked toggle the other detail
// views use.
//
// Any staff member edits; only an admin deletes (migration 1829000000, the same
// split locations got in 1813000000).
import { computed, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { pb } from '@/pb'
import { useAuthStore } from '@/stores/auth'
import type { Customer, Location, MaintenancePlan, Project, Staff, Thing, Ticket, TicketCategory } from '@/types'
import { TICKET_PRIORITIES } from '@/types'
import SearchSelect from '@/components/SearchSelect.vue'
import MinutesInput from '@/components/MinutesInput.vue'
import TicketBadges from '@/components/TicketBadges.vue'
import { formatDay } from '@/due'

const route = useRoute()
const router = useRouter()
const auth = useAuthStore()

const id = computed(() => route.params.id as string | undefined)
const isEdit = computed(() => !!id.value)
// View/edit toggle: create opens unlocked, an existing record opens locked.
const editing = ref(false)

const record = ref<MaintenancePlan | null>(null)
const tickets = ref<Ticket[]>([])
const customers = ref<Customer[]>([])
const staff = ref<Staff[]>([])
const categories = ref<TicketCategory[]>([])
const locations = ref<Location[]>([])
const things = ref<Thing[]>([])
const projects = ref<Project[]>([])

const loading = ref(true)
const saving = ref(false)
const error = ref('')

const form = ref({
  customer: '',
  title: '',
  body: '',
  thing: '',
  location: '',
  project: '',
  category: '',
  assignee: '',
  priority: 'normal',
  estimated_minutes: null as number | null,
  interval_days: 90,
  anchor: 'schedule',
  lead_time_days: 0,
  // A calendar date: '' is the empty value, no timezone round-trip. Matching
  // ProjectDetailView's start_date/target_date.
  next_due: '',
  paused: false,
})

const customerOptions = computed(() => customers.value.map((c) => ({ id: c.id, label: c.name })))
const staffOptions = computed(() => staff.value.map((s) => ({ id: s.id, label: s.name, sublabel: s.email })))
const categoryOptions = computed(() => categories.value.map((c) => ({ id: c.id, label: c.name })))
const locationOptions = computed(() => locations.value.map((l) => ({ id: l.id, label: l.name, sublabel: l.code || undefined })))
const thingOptions = computed(() => things.value.map((t) => ({ id: t.id, label: t.name, sublabel: t.code || undefined })))
const projectOptions = computed(() => projects.value.map((p) => ({ id: p.id, label: `#${p.number} ${p.title}` })))

// Parked: an empty next_due is invisible to the generator's query. For a
// completion-anchored plan that is the normal waiting state, not a fault — say
// which, or a plan doing exactly the right thing looks broken.
const parked = computed(() => isEdit.value && !form.value.next_due)
const openTicket = computed(() =>
  tickets.value.find((t) => t.status !== 'resolved' && t.status !== 'closed'),
)

function applyRecord(p: MaintenancePlan) {
  form.value = {
    customer: p.customer || '',
    title: p.title || '',
    body: p.body || '',
    thing: p.thing || '',
    location: p.location || '',
    project: p.project || '',
    category: p.category || '',
    assignee: p.assignee || '',
    priority: p.priority || 'normal',
    estimated_minutes: p.estimated_minutes ?? null,
    interval_days: p.interval_days ?? 90,
    anchor: p.anchor || 'schedule',
    lead_time_days: p.lead_time_days ?? 0,
    next_due: (p.next_due || '').slice(0, 10),
    paused: !!p.paused,
  }
}

// The pickers are customer-scoped, so switching customer invalidates them.
async function loadScoped(customerId: string) {
  if (!customerId) {
    locations.value = []
    things.value = []
    projects.value = []
    return
  }
  try {
    const [locs, thgs, projs] = await Promise.all([
      pb.collection('locations').getFullList<Location>({ filter: `customer = '${customerId}'`, sort: 'name' }),
      pb.collection('things').getFullList<Thing>({ filter: `customer = '${customerId}' && retired = false`, sort: 'name' }),
      pb.collection('projects').getFullList<Project>({ filter: `customer = '${customerId}'`, sort: '-number' }),
    ])
    locations.value = locs
    things.value = thgs
    projects.value = projs
  } catch {
    // Optional pickers; the form still saves without them.
  }
}

function onCustomerChange(value: string) {
  form.value.thing = ''
  form.value.location = ''
  form.value.project = ''
  loadScoped(value)
}

async function load() {
  loading.value = true
  error.value = ''
  try {
    const [custs, stf, cats] = await Promise.all([
      pb.collection('customers').getFullList<Customer>({ sort: 'name', filter: 'active = true' }),
      pb.collection('staff').getFullList<Staff>({ sort: 'name', filter: 'active = true' }),
      pb.collection('ticket_categories').getFullList<TicketCategory>({ sort: 'sort_order,name', filter: 'active = true' }),
    ])
    customers.value = custs
    staff.value = stf
    categories.value = cats

    if (isEdit.value) {
      const p = await pb.collection('maintenance_plans').getOne<MaintenancePlan>(id.value!, {
        expand: 'customer,thing,location,assignee',
      })
      record.value = p
      applyRecord(p)
      editing.value = false
      await loadScoped(p.customer)
      tickets.value = (
        await pb.collection('tickets').getList<Ticket>(1, 20, {
          filter: `maintenance_plan = '${id.value}'`,
          sort: '-created',
        })
      ).items
    } else {
      editing.value = true
    }
  } catch (err: any) {
    error.value = err?.message || 'Failed to load plan'
  } finally {
    loading.value = false
  }
}

async function save() {
  if (!form.value.customer || !form.value.title.trim() || !form.value.interval_days) return
  saving.value = true
  error.value = ''
  const data = {
    customer: form.value.customer,
    title: form.value.title.trim(),
    body: form.value.body.trim(),
    thing: form.value.thing,
    location: form.value.location,
    project: form.value.project,
    category: form.value.category,
    assignee: form.value.assignee,
    priority: form.value.priority,
    estimated_minutes: form.value.estimated_minutes,
    interval_days: form.value.interval_days,
    anchor: form.value.anchor,
    lead_time_days: form.value.lead_time_days || 0,
    next_due: form.value.next_due || '',
    paused: form.value.paused,
  }
  try {
    if (isEdit.value) {
      record.value = await pb.collection('maintenance_plans').update<MaintenancePlan>(id.value!, data, {
        expand: 'customer,thing,location,assignee',
      })
      applyRecord(record.value)
      editing.value = false
    } else {
      const created = await pb.collection('maintenance_plans').create<MaintenancePlan>(data)
      router.replace(`/staff/maintenance/${created.id}`)
      return
    }
  } catch (err: any) {
    error.value = err?.data?.message || err?.message || 'Failed to save'
  } finally {
    saving.value = false
  }
}

function cancel() {
  if (!isEdit.value) {
    router.push('/staff/maintenance')
    return
  }
  if (record.value) applyRecord(record.value)
  editing.value = false
}

async function remove() {
  if (!isEdit.value) return
  if (!confirm(`Delete “${form.value.title}”? Tickets it already generated are kept.`)) return
  error.value = ''
  try {
    await pb.collection('maintenance_plans').delete(id.value!)
    router.push('/staff/maintenance')
  } catch (err: any) {
    error.value = err?.data?.message || err?.message || 'Failed to delete'
  }
}

onMounted(load)
// The create flow router.replace()s from /new to /:id, which reuses this
// component instance (onMounted won't refire) — reload so the freshly created
// record's expands and generated-ticket list populate.
watch(() => route.params.id, load)
</script>

<template>
  <div v-if="loading" class="flex justify-center p-12"><span class="loading loading-spinner loading-lg"></span></div>

  <div v-else class="space-y-4">
    <div class="flex flex-col sm:flex-row justify-between items-start sm:items-center gap-2">
      <div class="min-w-0">
        <h1 class="text-2xl font-bold truncate">{{ isEdit ? form.title || 'Plan' : 'New Maintenance Plan' }}</h1>
        <p v-if="isEdit" class="text-sm text-base-content/60">
          {{ record?.expand?.customer?.name }}
        </p>
      </div>
      <div class="flex gap-2 w-full sm:w-auto">
        <template v-if="editing">
          <button class="btn btn-ghost btn-sm" :disabled="saving" @click="cancel">Cancel</button>
          <button class="btn btn-primary btn-sm" :disabled="saving" @click="save">
            <span v-if="saving" class="loading loading-spinner loading-xs"></span>
            Save
          </button>
        </template>
        <template v-else>
          <router-link to="/staff/maintenance" class="btn btn-ghost btn-sm">Back</router-link>
          <button class="btn btn-primary btn-sm" @click="editing = true">Edit</button>
        </template>
      </div>
    </div>

    <div v-if="error" class="alert alert-error py-2 text-sm">{{ error }}</div>

    <!-- The parked state, said out loud. A completion-anchored plan with an
         open ticket is working exactly as designed; without this it reads as a
         plan that forgot to have a next date. -->
    <div v-if="isEdit && !editing && parked" class="alert py-2 text-sm">
      <span v-if="form.anchor === 'completion'">
        Waiting on
        <router-link v-if="openTicket" :to="`/staff/tickets/${openTicket.id}`" class="link">
          #{{ openTicket.number }}
        </router-link>
        <span v-else>its open ticket</span>
        — the next date is set when that work is resolved.
      </span>
      <span v-else>Not scheduled. Set a next due date to start generating.</span>
    </div>

    <div class="grid grid-cols-1 lg:grid-cols-2 gap-4 items-start">
      <!-- What the work is -->
      <div class="card bg-base-100 shadow-sm">
        <div class="card-body p-4 space-y-3">
          <h2 class="font-semibold text-sm">Work</h2>
          <div class="form-control">
            <label class="label py-1"><span class="label-text text-xs">Customer *</span></label>
            <SearchSelect
              v-model="form.customer"
              :options="customerOptions"
              size="sm"
              placeholder="Type a customer…"
              :disabled="!editing || saving"
              @update:model-value="onCustomerChange"
            />
          </div>
          <div class="form-control">
            <label class="label py-1"><span class="label-text text-xs">Title *</span></label>
            <input v-model="form.title" type="text" maxlength="300" class="input input-bordered input-sm" placeholder="Quarterly door controller service" :disabled="!editing || saving" />
          </div>
          <div class="form-control">
            <label class="label py-1"><span class="label-text text-xs">Details</span></label>
            <textarea v-model="form.body" rows="4" class="textarea textarea-bordered textarea-sm" placeholder="What the technician should do on site." :disabled="!editing || saving"></textarea>
          </div>
          <div class="form-control">
            <label class="label py-1"><span class="label-text text-xs">Thing</span></label>
            <SearchSelect v-model="form.thing" :options="thingOptions" size="sm" empty-label="None" placeholder="Thing…" :disabled="!editing || saving || !form.customer" />
          </div>
          <div class="form-control">
            <label class="label py-1"><span class="label-text text-xs">Location</span></label>
            <SearchSelect v-model="form.location" :options="locationOptions" size="sm" empty-label="None" placeholder="Location…" :disabled="!editing || saving || !form.customer" />
          </div>
        </div>
      </div>

      <!-- The schedule + the triage stamped onto every generated ticket -->
      <div class="card bg-base-100 shadow-sm">
        <div class="card-body p-4 space-y-3">
          <h2 class="font-semibold text-sm">Schedule</h2>
          <div class="flex gap-2">
            <div class="form-control flex-1">
              <label class="label py-1"><span class="label-text text-xs">Every (days) *</span></label>
              <input v-model.number="form.interval_days" type="number" min="1" class="input input-bordered input-sm" :disabled="!editing || saving" />
            </div>
            <div class="form-control flex-1">
              <label class="label py-1"><span class="label-text text-xs">Next due</span></label>
              <input v-model="form.next_due" type="date" class="input input-bordered input-sm" :disabled="!editing || saving" />
            </div>
          </div>
          <div class="form-control">
            <label class="label py-1"><span class="label-text text-xs">Repeat from</span></label>
            <select v-model="form.anchor" class="select select-bordered select-sm" :disabled="!editing || saving">
              <option value="schedule">The calendar — every N days regardless</option>
              <option value="completion">Last completion — N days after the work is done</option>
            </select>
            <span class="label-text-alt text-base-content/50 pt-1">
              {{
                form.anchor === 'completion'
                  ? 'The plan pauses while its ticket is open, then restarts from the day it was resolved.'
                  : 'Quarterly stays quarterly however late the visit ran.'
              }}
            </span>
          </div>
          <div class="form-control">
            <label class="label py-1"><span class="label-text text-xs">Open the ticket this many days early</span></label>
            <input v-model.number="form.lead_time_days" type="number" min="0" class="input input-bordered input-sm" :disabled="!editing || saving" />
          </div>

          <div class="divider my-0"></div>

          <div class="form-control">
            <label class="label py-1"><span class="label-text text-xs">Assignee</span></label>
            <SearchSelect v-model="form.assignee" :options="staffOptions" size="sm" empty-label="Unassigned" placeholder="Type a name…" :disabled="!editing || saving" />
          </div>
          <div class="form-control">
            <label class="label py-1"><span class="label-text text-xs">Priority</span></label>
            <select v-model="form.priority" class="select select-bordered select-sm" :disabled="!editing || saving">
              <option v-for="p in TICKET_PRIORITIES" :key="p" :value="p">{{ p }}</option>
            </select>
          </div>
          <div class="form-control">
            <label class="label py-1"><span class="label-text text-xs">Category</span></label>
            <SearchSelect v-model="form.category" :options="categoryOptions" size="sm" empty-label="None" placeholder="Classify…" :disabled="!editing || saving" />
          </div>
          <div class="form-control">
            <label class="label py-1"><span class="label-text text-xs">Project</span></label>
            <SearchSelect v-model="form.project" :options="projectOptions" size="sm" empty-label="None" placeholder="Group into…" :disabled="!editing || saving || !form.customer" />
          </div>
          <div class="form-control">
            <label class="label py-1"><span class="label-text text-xs">Estimated effort</span></label>
            <MinutesInput v-model="form.estimated_minutes" size="sm" placeholder="estimate" :disabled="!editing || saving" />
          </div>

          <label class="label cursor-pointer justify-start gap-2 py-1">
            <input v-model="form.paused" type="checkbox" class="toggle toggle-sm" :disabled="!editing || saving" />
            <span class="label-text text-xs">Paused — stop generating tickets</span>
          </label>

          <div v-if="isEdit && auth.isAdmin && editing" class="pt-2">
            <button class="btn btn-error btn-outline btn-sm" @click="remove">Delete plan</button>
          </div>
        </div>
      </div>
    </div>

    <!-- What this plan has actually produced. The audit for "is this thing
         working", and the reason tickets carry a maintenance_plan relation. -->
    <div v-if="isEdit" class="card bg-base-100 shadow-sm">
      <div class="card-body p-4 space-y-2">
        <h2 class="font-semibold text-sm">Generated tickets</h2>
        <div v-if="tickets.length === 0" class="text-sm text-base-content/60">
          Nothing yet — the first ticket appears when the plan comes due.
        </div>
        <ul v-else class="divide-y divide-base-200">
          <li v-for="t in tickets" :key="t.id" class="py-2 flex items-center justify-between gap-2">
            <router-link :to="`/staff/tickets/${t.id}`" class="link link-hover text-sm truncate">
              #{{ t.number }} — {{ t.title }}
            </router-link>
            <div class="flex items-center gap-2 shrink-0">
              <span v-if="t.due_at" class="text-xs text-base-content/50 tabular-nums">
                due {{ formatDay(t.due_at) }}
              </span>
              <TicketBadges :status="t.status" />
            </div>
          </li>
        </ul>
      </div>
    </div>
  </div>
</template>
