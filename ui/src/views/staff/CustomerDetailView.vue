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

const form = ref({ name: '', active: true, platform_org_id: '', notes: '' })
const webhookToken = ref('')

async function load() {
  loading.value = true
  try {
    customer.value = await pb.collection('customers').getOne<Customer>(id)
    form.value = {
      name: customer.value.name,
      active: customer.value.active,
      platform_org_id: customer.value.platform_org_id || '',
      notes: customer.value.notes || '',
    }
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
  } catch (err: any) {
    error.value = err?.message || 'Failed to save'
  } finally {
    saving.value = false
  }
}

async function revealToken() {
  error.value = ''
  try {
    const res = await pb.send(`/api/helpdesk/customers/${id}/webhook-token`, { method: 'POST' })
    webhookToken.value = res.token
  } catch (err: any) {
    error.value = err?.message || 'Failed to fetch webhook token'
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
    <h1 class="text-2xl font-bold">{{ customer.name }}</h1>

    <div v-if="error" class="alert alert-error py-2 text-sm">{{ error }}</div>

    <div class="grid grid-cols-1 lg:grid-cols-2 gap-4 items-start">
      <div class="card bg-base-100 shadow-sm">
        <div class="card-body space-y-3">
          <h2 class="card-title text-base">Details</h2>
          <div class="form-control">
            <label class="label py-1"><span class="label-text">Name</span></label>
            <input v-model="form.name" type="text" class="input input-bordered input-sm" :disabled="!auth.isAdmin || saving" />
          </div>
          <div class="form-control">
            <label class="label py-1">
              <span class="label-text">Platform Org ID</span>
            </label>
            <input v-model="form.platform_org_id" type="text" class="input input-bordered input-sm font-mono" placeholder="15-char platform organization id" :disabled="!auth.isAdmin || saving" />
            <label class="label py-1">
              <span class="label-text-alt text-base-content/60">NATS events on helpdesk.{org}.> create tickets for this customer</span>
            </label>
          </div>
          <div class="form-control">
            <label class="label py-1"><span class="label-text">Notes</span></label>
            <textarea v-model="form.notes" rows="3" class="textarea textarea-bordered textarea-sm" :disabled="!auth.isAdmin || saving"></textarea>
          </div>
          <div class="form-control">
            <label class="label cursor-pointer justify-start gap-3 py-1">
              <input v-model="form.active" type="checkbox" class="toggle toggle-success toggle-sm" :disabled="!auth.isAdmin || saving" />
              <span class="label-text">Active</span>
            </label>
          </div>
          <div v-if="auth.isAdmin" class="flex justify-between items-center pt-1">
            <button class="btn btn-ghost btn-sm" @click="revealToken">Webhook token</button>
            <button class="btn btn-primary btn-sm" :disabled="saving" @click="save">
              <span v-if="saving" class="loading loading-spinner loading-xs"></span>
              Save
            </button>
          </div>
          <div v-if="webhookToken" class="alert py-2">
            <div class="text-xs">
              <div class="font-semibold mb-1">Inbound webhook</div>
              <code class="break-all">POST /api/helpdesk/inbound/{{ webhookToken }}</code>
            </div>
          </div>
        </div>
      </div>

      <div class="space-y-4">
        <div class="card bg-base-100 shadow-sm">
          <div class="card-body">
            <h2 class="card-title text-base">Recent Tickets</h2>
            <ul class="space-y-1">
              <li v-for="t in tickets" :key="t.id">
                <router-link :to="`/staff/tickets/${t.id}`" class="flex items-center gap-2 text-sm link-hover">
                  <span class="font-mono">#{{ t.number }}</span>
                  <span class="flex-1 truncate">{{ t.title }}</span>
                  <TicketBadges :status="t.status" />
                </router-link>
              </li>
            </ul>
            <p v-if="tickets.length === 0" class="text-sm text-base-content/50">No tickets.</p>
          </div>
        </div>

        <div class="card bg-base-100 shadow-sm">
          <div class="card-body">
            <h2 class="card-title text-base">Requesters</h2>
            <ul class="space-y-1">
              <li v-for="r in requesters" :key="r.id" class="text-sm flex items-center gap-2">
                <span class="flex-1">{{ r.name || r.email }}</span>
                <span class="text-base-content/50">{{ r.email }}</span>
                <span class="badge badge-xs" :class="r.active ? 'badge-success' : 'badge-ghost'">{{ r.active ? 'active' : 'inactive' }}</span>
              </li>
            </ul>
            <p v-if="requesters.length === 0" class="text-sm text-base-content/50">No portal accounts.</p>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>
