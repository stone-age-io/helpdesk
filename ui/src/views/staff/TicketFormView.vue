<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { pb } from '@/pb'
import type { Customer, Location, Requester, Staff, TicketCategory } from '@/types'
import { TICKET_PRIORITIES, TICKET_TYPES } from '@/types'
import SearchSelect from '@/components/SearchSelect.vue'
import FileInput from '@/components/FileInput.vue'

const router = useRouter()

const customers = ref<Customer[]>([])
const staff = ref<Staff[]>([])
const requesters = ref<Requester[]>([])
const categories = ref<TicketCategory[]>([])
const locations = ref<Location[]>([])
const files = ref<File[]>([])
const loading = ref(false)
const error = ref('')

const form = ref({
  customer: '',
  title: '',
  body: '',
  priority: 'normal',
  type: 'issue',
  assignee: '',
  requester: '',
  category: '',
  asset: '',
  location: '',
  location_note: '',
})

async function loadOptions() {
  try {
    customers.value = await pb.collection('customers').getFullList<Customer>({ sort: 'name', filter: 'active = true' })
    staff.value = await pb.collection('staff').getFullList<Staff>({ sort: 'name', filter: 'active = true' })
    categories.value = await pb.collection('ticket_categories').getFullList<TicketCategory>({ sort: 'sort_order,name', filter: 'active = true' })
  } catch (err: any) {
    error.value = err?.message || 'Failed to load form options'
  }
}

async function loadRequesters() {
  form.value.requester = ''
  requesters.value = []
  if (!form.value.customer) return
  try {
    requesters.value = await pb.collection('users').getFullList<Requester>({
      filter: `customer = '${form.value.customer}'`,
      sort: 'name',
    })
  } catch {
    // Requester linking is optional.
  }
}

async function loadLocations() {
  form.value.location = ''
  locations.value = []
  if (!form.value.customer) return
  try {
    locations.value = await pb.collection('locations').getFullList<Location>({
      filter: `customer = '${form.value.customer}'`,
      sort: 'name',
    })
  } catch {
    // Location linking is optional.
  }
}

// The customer bounds both the requester and location pickers.
function onCustomerChange() {
  loadRequesters()
  loadLocations()
}

async function createLocation(label: string) {
  if (!form.value.customer || !label.trim()) return
  try {
    const rec = await pb.collection('locations').create({ customer: form.value.customer, name: label.trim() })
    await loadLocations()
    form.value.location = rec.id
  } catch (err: any) {
    error.value = err?.message || 'Failed to create location'
  }
}

const customerOptions = computed(() => customers.value.map((c) => ({ id: c.id, label: c.name })))
const staffOptions = computed(() => staff.value.map((s) => ({ id: s.id, label: s.name, sublabel: s.email })))
const categoryOptions = computed(() => categories.value.map((c) => ({ id: c.id, label: c.name })))
const requesterOptions = computed(() =>
  requesters.value.map((r) => ({ id: r.id, label: r.name || r.email, sublabel: r.name ? r.email : undefined })),
)
const locationOptions = computed(() =>
  locations.value.map((l) => ({ id: l.id, label: l.name, sublabel: l.code || l.address || undefined })),
)

async function submit() {
  loading.value = true
  error.value = ''
  try {
    const rec = await pb.collection('tickets').create({
      ...form.value,
      source: 'agent',
      attachments: files.value,
    })
    router.push(`/staff/tickets/${rec.id}`)
  } catch (err: any) {
    error.value = err?.message || 'Failed to create ticket'
  } finally {
    loading.value = false
  }
}

onMounted(loadOptions)
</script>

<template>
  <div class="max-w-6xl mx-auto space-y-4">
    <div class="breadcrumbs text-sm">
      <ul>
        <li><a @click="router.push('/staff/tickets')">Tickets</a></li>
        <li>New</li>
      </ul>
    </div>
    <h1 class="text-2xl font-bold">New Ticket</h1>

    <div v-if="error" class="alert alert-error py-2 text-sm">{{ error }}</div>

    <form @submit.prevent="submit" class="space-y-4">
      <!-- Two columns mirroring the ticket detail view: the "what" on the
           left, the "how we're filing it" metadata on the right. -->
      <div class="flex flex-col xl:flex-row gap-4 items-start">
        <!-- Main: the problem -->
        <div class="flex-1 w-full min-w-0 card bg-base-100 shadow-sm">
          <div class="card-body space-y-3">
            <div class="form-control">
              <label class="label"><span class="label-text">Customer *</span></label>
              <SearchSelect
                v-model="form.customer"
                :options="customerOptions"
                placeholder="Type to find a customer…"
                :disabled="loading"
                @update:model-value="onCustomerChange"
              />
            </div>

            <div class="form-control">
              <label class="label"><span class="label-text">Title *</span></label>
              <input v-model="form.title" type="text" class="input input-bordered" required :disabled="loading" />
            </div>

            <div class="form-control">
              <label class="label"><span class="label-text">Details</span></label>
              <textarea v-model="form.body" rows="8" class="textarea textarea-bordered" :disabled="loading"></textarea>
            </div>

            <div class="form-control">
              <label class="label"><span class="label-text">Attachments</span></label>
              <FileInput v-model:files="files" :disabled="loading" />
            </div>
          </div>
        </div>

        <!-- Rail: classification + assignment -->
        <div class="w-full xl:w-80 card bg-base-100 shadow-sm">
          <div class="card-body space-y-3 py-4 px-4">
            <div class="form-control">
              <label class="label py-1"><span class="label-text text-xs">Priority</span></label>
              <select v-model="form.priority" class="select select-bordered select-sm" :disabled="loading">
                <option v-for="p in TICKET_PRIORITIES" :key="p" :value="p">{{ p }}</option>
              </select>
            </div>
            <div class="form-control">
              <label class="label py-1"><span class="label-text text-xs">Assignee</span></label>
              <SearchSelect v-model="form.assignee" :options="staffOptions" size="sm" empty-label="Unassigned" placeholder="Type a name…" :disabled="loading" />
            </div>
            <div class="form-control">
              <label class="label py-1"><span class="label-text text-xs">Requester</span></label>
              <SearchSelect v-model="form.requester" :options="requesterOptions" size="sm" empty-label="None" placeholder="Type a name or email…" :disabled="loading || !form.customer" />
            </div>

            <div class="divider my-0"></div>

            <div class="form-control">
              <label class="label py-1"><span class="label-text text-xs">Category</span></label>
              <SearchSelect v-model="form.category" :options="categoryOptions" size="sm" empty-label="None" placeholder="Classify…" :disabled="loading" />
            </div>
            <div class="form-control">
              <label class="label py-1"><span class="label-text text-xs">Type</span></label>
              <select v-model="form.type" class="select select-bordered select-sm" :disabled="loading">
                <option v-for="t in TICKET_TYPES" :key="t" :value="t">{{ t }}</option>
              </select>
            </div>
            <div class="form-control">
              <label class="label py-1"><span class="label-text text-xs">Asset</span></label>
              <input v-model="form.asset" type="text" maxlength="200" class="input input-bordered input-sm" placeholder="Device / system" :disabled="loading" />
            </div>
            <div class="form-control">
              <label class="label py-1"><span class="label-text text-xs">Location</span></label>
              <SearchSelect v-model="form.location" :options="locationOptions" size="sm" empty-label="None" placeholder="Pick a site…" create-label="New location" :disabled="loading || !form.customer" @create="createLocation" />
            </div>
            <div class="form-control">
              <label class="label py-1"><span class="label-text text-xs">Location note</span></label>
              <input v-model="form.location_note" type="text" maxlength="200" class="input input-bordered input-sm" placeholder="Access hints / where" :disabled="loading" />
            </div>
          </div>
        </div>
      </div>

      <div class="flex justify-end gap-2">
        <button type="button" class="btn btn-ghost" :disabled="loading" @click="router.back()">Cancel</button>
        <button type="submit" class="btn btn-primary" :disabled="loading || !form.customer || !form.title.trim()">
          <span v-if="loading" class="loading loading-spinner loading-sm"></span>
          Create
        </button>
      </div>
    </form>
  </div>
</template>
