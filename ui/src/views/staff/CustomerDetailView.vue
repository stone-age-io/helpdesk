<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { pb } from '@/pb'
import { useAuthStore } from '@/stores/auth'
import type { Customer, Requester, Ticket } from '@/types'
import TicketBadges from '@/components/TicketBadges.vue'

const route = useRoute()
const router = useRouter()
const auth = useAuthStore()
const id = route.params.id as string

const customer = ref<Customer | null>(null)
const tickets = ref<Ticket[]>([])
const requesters = ref<Requester[]>([])
const loading = ref(true)
const error = ref('')
const saving = ref(false)
// View/edit toggle: the record opens locked; admins unlock with Edit. Non-admins
// never see Edit, so the view stays permanently read-only for them.
const editing = ref(false)

const form = ref({ name: '', active: true, platform_org_id: '', notes: '', show_time_to_requester: false })
const webhookToken = ref('')

function applyRecord(c: Customer) {
  form.value = {
    name: c.name,
    active: c.active,
    platform_org_id: c.platform_org_id || '',
    notes: c.notes || '',
    show_time_to_requester: c.show_time_to_requester || false,
  }
}

function startEdit() {
  editing.value = true
}

function cancelEdit() {
  if (customer.value) applyRecord(customer.value)
  editing.value = false
}

async function load() {
  loading.value = true
  try {
    customer.value = await pb.collection('customers').getOne<Customer>(id)
    applyRecord(customer.value)
    editing.value = false
    tickets.value = (
      await pb.collection('tickets').getList<Ticket>(1, 10, {
        filter: `customer = '${id}'`,
        sort: '-created',
      })
    ).items
    requesters.value = await pb.collection('users').getFullList<Requester>({
      filter: `customer = '${id}'`,
      sort: 'name',
    })
  } catch (err: any) {
    error.value = err?.message || 'Failed to load customer'
  } finally {
    loading.value = false
  }
}

async function save() {
  saving.value = true
  error.value = ''
  try {
    customer.value = await pb.collection('customers').update<Customer>(id, form.value)
    editing.value = false
  } catch (err: any) {
    error.value = err?.message || 'Failed to save'
  } finally {
    saving.value = false
  }
}

async function revealToken(rotate = false) {
  error.value = ''
  if (rotate && !confirm('Rotate the webhook token? Anything using the current URL stops working immediately.')) return
  try {
    const res = await pb.send(`/api/helpdesk/customers/${id}/webhook-token${rotate ? '?rotate=1' : ''}`, { method: 'POST' })
    webhookToken.value = res.token
  } catch (err: any) {
    error.value = err?.message || 'Failed to fetch webhook token'
  }
}

const webhookUrl = () => `${location.origin}/api/helpdesk/inbound/${webhookToken.value}`
const copied = ref(false)
async function copyWebhookUrl() {
  try {
    await navigator.clipboard.writeText(webhookUrl())
    copied.value = true
    setTimeout(() => (copied.value = false), 1500)
  } catch {
    error.value = 'Clipboard unavailable — copy manually.'
  }
}

onMounted(load)
</script>

<template>
  <div v-if="loading" class="flex justify-center p-12"><span class="loading loading-spinner loading-lg"></span></div>

  <div v-else-if="customer" class="space-y-4">
    <div class="breadcrumbs text-sm">
      <ul>
        <li><a @click="router.push('/staff/customers')">Customers</a></li>
        <li>{{ customer.name }}</li>
      </ul>
    </div>
    <div class="flex items-center justify-between gap-2 flex-wrap">
      <h1 class="text-2xl font-bold">{{ customer.name }}</h1>
      <div v-if="auth.isAdmin" class="flex gap-2">
        <template v-if="editing">
          <button class="btn btn-ghost btn-sm" :disabled="saving" @click="cancelEdit">Cancel</button>
          <button class="btn btn-primary btn-sm" :disabled="saving" @click="save">
            <span v-if="saving" class="loading loading-spinner loading-xs"></span>
            Save
          </button>
        </template>
        <button v-else class="btn btn-primary btn-sm" @click="startEdit">Edit</button>
      </div>
    </div>

    <div v-if="error" class="alert alert-error py-2 text-sm">{{ error }}</div>

    <div class="grid grid-cols-1 lg:grid-cols-2 gap-4 items-start">
      <div class="card bg-base-100 shadow-sm">
        <div class="card-body space-y-3">
          <h2 class="card-title text-base">Details</h2>
          <div class="form-control">
            <label class="label py-1"><span class="label-text">Name</span></label>
            <input v-model="form.name" type="text" class="input input-bordered input-sm" :disabled="!editing || saving" />
          </div>
          <div class="form-control">
            <label class="label py-1">
              <span class="label-text">Platform Org ID</span>
            </label>
            <input v-model="form.platform_org_id" type="text" class="input input-bordered input-sm font-mono" placeholder="15-char platform organization id" :disabled="!editing || saving" />
            <label class="label py-1">
              <span class="label-text-alt text-base-content/60">NATS events on helpdesk.{org}.> create tickets for this customer</span>
            </label>
          </div>
          <div class="form-control">
            <label class="label py-1"><span class="label-text">Notes</span></label>
            <textarea v-model="form.notes" rows="3" class="textarea textarea-bordered textarea-sm" :disabled="!editing || saving"></textarea>
          </div>
          <div class="form-control">
            <label class="label cursor-pointer justify-start gap-3 py-1">
              <input v-model="form.active" type="checkbox" class="toggle toggle-success toggle-sm" :disabled="!editing || saving" />
              <span class="label-text">Active</span>
            </label>
          </div>
          <div class="form-control">
            <label class="label cursor-pointer justify-start gap-3 py-1">
              <input v-model="form.show_time_to_requester" type="checkbox" class="toggle toggle-sm" :disabled="!editing || saving" />
              <span class="label-text">Show logged time to requesters</span>
            </label>
            <label class="label py-0">
              <span class="label-text-alt text-base-content/60">Portal shows the total hours on each ticket — the aggregate only, never entries or names.</span>
            </label>
          </div>
          <div v-if="auth.isAdmin && editing" class="pt-1">
            <button class="btn btn-ghost btn-sm" @click="revealToken()">Webhook token</button>
          </div>
          <div v-if="webhookToken" class="alert py-2">
            <div class="text-xs w-full">
              <div class="font-semibold mb-1">Inbound webhook</div>
              <code class="break-all">POST {{ webhookUrl() }}</code>
              <div class="flex gap-2 mt-2">
                <button class="btn btn-xs" @click="copyWebhookUrl">{{ copied ? 'Copied ✓' : 'Copy URL' }}</button>
                <button class="btn btn-xs btn-ghost text-error" @click="revealToken(true)">Rotate token</button>
              </div>
            </div>
          </div>
        </div>
      </div>

      <div class="space-y-4">
        <div class="card bg-base-100 shadow-sm">
          <div class="card-body">
            <div class="flex items-center justify-between">
              <h2 class="card-title text-base">Recent Tickets</h2>
              <router-link :to="`/staff/tickets?customer=${id}`" class="link link-hover text-sm">View all →</router-link>
            </div>
            <div class="divide-y divide-base-200">
              <router-link
                v-for="t in tickets"
                :key="t.id"
                :to="`/staff/tickets/${t.id}`"
                class="flex items-center gap-3 py-2 hover:bg-base-200/50 -mx-2 px-2 rounded"
              >
                <span class="font-mono text-xs text-base-content/50 w-10">#{{ t.number }}</span>
                <span class="flex-1 truncate">{{ t.title }}</span>
                <TicketBadges :status="t.status" :priority="t.priority" />
              </router-link>
              <p v-if="tickets.length === 0" class="py-3 text-sm text-base-content/50">No tickets.</p>
            </div>
          </div>
        </div>

        <div class="card bg-base-100 shadow-sm">
          <div class="card-body">
            <h2 class="card-title text-base">
              Requesters
              <span v-if="requesters.length" class="text-sm font-normal text-base-content/50">· {{ requesters.length }}</span>
            </h2>
            <div class="divide-y divide-base-200">
              <router-link
                v-for="r in requesters"
                :key="r.id"
                :to="`/staff/tickets?search=${encodeURIComponent(r.email)}`"
                class="flex items-center gap-3 py-2 hover:bg-base-200/50 -mx-2 px-2 rounded"
                title="View this requester's tickets"
              >
                <span class="flex-1 truncate">{{ r.name || r.email }}</span>
                <span class="hidden sm:inline text-xs text-base-content/50 truncate">{{ r.email }}</span>
                <span class="badge-soft shrink-0" :class="r.active ? 'badge-soft-success' : 'badge-soft-neutral'">{{ r.active ? 'active' : 'inactive' }}</span>
              </router-link>
              <p v-if="requesters.length === 0" class="py-3 text-sm text-base-content/50">No portal accounts.</p>
            </div>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>
