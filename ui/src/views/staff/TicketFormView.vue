<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { pb } from '@/pb'
import type { Customer, Requester, Staff } from '@/types'
import { TICKET_PRIORITIES } from '@/types'
import SearchSelect from '@/components/SearchSelect.vue'

const router = useRouter()

const customers = ref<Customer[]>([])
const staff = ref<Staff[]>([])
const requesters = ref<Requester[]>([])
const loading = ref(false)
const error = ref('')

const form = ref({
  customer: '',
  title: '',
  body: '',
  priority: 'normal',
  assignee: '',
  requester: '',
})

async function loadOptions() {
  try {
    customers.value = await pb.collection('customers').getFullList<Customer>({ sort: 'name', filter: 'active = true' })
    staff.value = await pb.collection('staff').getFullList<Staff>({ sort: 'name', filter: 'active = true' })
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

const customerOptions = computed(() => customers.value.map((c) => ({ id: c.id, label: c.name })))
const staffOptions = computed(() => staff.value.map((s) => ({ id: s.id, label: s.name, sublabel: s.email })))
const requesterOptions = computed(() =>
  requesters.value.map((r) => ({ id: r.id, label: r.name || r.email, sublabel: r.name ? r.email : undefined })),
)

async function submit() {
  loading.value = true
  error.value = ''
  try {
    const rec = await pb.collection('tickets').create({
      ...form.value,
      source: 'agent',
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
  <div class="max-w-2xl space-y-4">
    <div class="breadcrumbs text-sm">
      <ul>
        <li><a @click="router.push('/staff/tickets')">Tickets</a></li>
        <li>New</li>
      </ul>
    </div>
    <h1 class="text-2xl font-bold">New Ticket</h1>

    <form class="card bg-base-100 shadow-sm" @submit.prevent="submit">
      <div class="card-body space-y-3">
        <div v-if="error" class="alert alert-error py-2 text-sm">{{ error }}</div>

        <div class="form-control">
          <label class="label"><span class="label-text">Customer *</span></label>
          <SearchSelect
            v-model="form.customer"
            :options="customerOptions"
            placeholder="Type to find a customer…"
            :disabled="loading"
            @update:model-value="loadRequesters"
          />
        </div>

        <div class="form-control">
          <label class="label"><span class="label-text">Title *</span></label>
          <input v-model="form.title" type="text" class="input input-bordered" required :disabled="loading" />
        </div>

        <div class="form-control">
          <label class="label"><span class="label-text">Details</span></label>
          <textarea v-model="form.body" rows="5" class="textarea textarea-bordered" :disabled="loading"></textarea>
        </div>

        <div class="grid grid-cols-1 sm:grid-cols-3 gap-3">
          <div class="form-control">
            <label class="label"><span class="label-text">Priority</span></label>
            <select v-model="form.priority" class="select select-bordered" :disabled="loading">
              <option v-for="p in TICKET_PRIORITIES" :key="p" :value="p">{{ p }}</option>
            </select>
          </div>
          <div class="form-control">
            <label class="label"><span class="label-text">Assignee</span></label>
            <SearchSelect v-model="form.assignee" :options="staffOptions" empty-label="Unassigned" placeholder="Type a name…" :disabled="loading" />
          </div>
          <div class="form-control">
            <label class="label"><span class="label-text">Requester</span></label>
            <SearchSelect v-model="form.requester" :options="requesterOptions" empty-label="None" placeholder="Type a name or email…" :disabled="loading || !form.customer" />
          </div>
        </div>

        <div class="flex justify-end gap-2 pt-2">
          <button type="button" class="btn btn-ghost" :disabled="loading" @click="router.back()">Cancel</button>
          <button type="submit" class="btn btn-primary" :disabled="loading || !form.customer || !form.title.trim()">
            <span v-if="loading" class="loading loading-spinner loading-sm"></span>
            Create
          </button>
        </div>
      </div>
    </form>
  </div>
</template>
