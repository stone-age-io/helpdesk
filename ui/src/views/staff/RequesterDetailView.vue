<script setup lang="ts">
// Requester detail / edit view (mirrors CustomerDetailView). The single
// management surface for a portal account: identity fields, active toggle,
// password reset, and this requester's recent tickets. Field editing and the
// password reset are admin-only (roles decision: agents get write access to
// Locations, not the requester roster); non-admins see a read-only view.
import { computed, onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { pb } from '@/pb'
import { useAuthStore } from '@/stores/auth'
import type { Customer, Requester, Ticket } from '@/types'
import SearchSelect from '@/components/SearchSelect.vue'
import TicketBadges from '@/components/TicketBadges.vue'
import Avatar from '@/components/Avatar.vue'

const route = useRoute()
const router = useRouter()
const auth = useAuthStore()
const id = route.params.id as string
const canEdit = computed(() => auth.isAdmin)

const requester = ref<Requester | null>(null)
const tickets = ref<Ticket[]>([])
const customers = ref<Customer[]>([])
const loading = ref(true)
const error = ref('')
const saving = ref(false)
// View/edit toggle: opens locked; admins unlock with Edit (non-admins never see
// the button, so their view stays read-only).
const editing = ref(false)

const form = ref({ name: '', email: '', phone: '', customer: '', active: true })
// One-time credential banner after a password reset — never persisted.
const issued = ref<{ email: string; password: string } | null>(null)

const customerOptions = computed(() => customers.value.map((c) => ({ id: c.id, label: c.name })))

function generatePassword(): string {
  const bytes = new Uint8Array(12)
  crypto.getRandomValues(bytes)
  return btoa(String.fromCharCode(...bytes)).replace(/[+/=]/g, '').slice(0, 16)
}

function applyRecord(r: Requester) {
  form.value = {
    name: r.name || '',
    email: r.email,
    phone: r.phone || '',
    customer: r.customer,
    active: r.active,
  }
}

function startEdit() {
  editing.value = true
}

function cancelEdit() {
  if (requester.value) applyRecord(requester.value)
  editing.value = false
}

async function load() {
  loading.value = true
  error.value = ''
  try {
    requester.value = await pb.collection('users').getOne<Requester>(id, { expand: 'customer' })
    applyRecord(requester.value)
    editing.value = false
    tickets.value = (
      await pb.collection('tickets').getList<Ticket>(1, 10, {
        filter: `requester = '${id}'`,
        sort: '-created',
      })
    ).items
    customers.value = await pb.collection('customers').getFullList<Customer>({ sort: 'name' })
  } catch (err: any) {
    error.value = err?.message || 'Failed to load requester'
  } finally {
    loading.value = false
  }
}

async function save() {
  saving.value = true
  error.value = ''
  try {
    requester.value = await pb.collection('users').update<Requester>(id, {
      name: form.value.name,
      email: form.value.email,
      phone: form.value.phone,
      customer: form.value.customer,
      active: form.value.active,
    }, { expand: 'customer' })
    editing.value = false
  } catch (err: any) {
    error.value = err?.data?.message || err?.message || 'Failed to save'
  } finally {
    saving.value = false
  }
}

async function resetPassword() {
  if (!requester.value) return
  if (!confirm(`Reset the password for ${requester.value.email}? Their current password stops working immediately.`)) return
  error.value = ''
  try {
    const password = generatePassword()
    await pb.collection('users').update(id, { password, passwordConfirm: password })
    issued.value = { email: requester.value.email, password }
  } catch (err: any) {
    error.value = err?.data?.message || err?.message || 'Failed to reset password'
  }
}

onMounted(load)
</script>

<template>
  <div v-if="loading" class="flex justify-center p-12"><span class="loading loading-spinner loading-lg"></span></div>

  <div v-else-if="requester" class="space-y-4">
    <div class="breadcrumbs text-sm">
      <ul>
        <li><a @click="router.push('/staff/requesters')">Requesters</a></li>
        <li>{{ requester.email }}</li>
      </ul>
    </div>
    <div class="flex items-center justify-between gap-2 flex-wrap">
      <div class="flex items-center gap-3 min-w-0">
        <Avatar :record="requester" :name="requester.name || requester.email" size="md" />
        <h1 class="text-2xl font-bold truncate">{{ requester.name || requester.email }}</h1>
      </div>
      <div v-if="canEdit" class="flex gap-2">
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

    <div v-if="issued" class="alert alert-info py-2 text-sm">
      <span>
        Credentials for <b>{{ issued.email }}</b> — one-time password:
        <code class="font-mono font-bold">{{ issued.password }}</code>
        — share securely, then <button class="link" @click="issued = null">dismiss</button>.
      </span>
    </div>

    <div class="grid grid-cols-1 lg:grid-cols-2 gap-4 items-start">
      <div class="card bg-base-100 shadow-sm">
        <div class="card-body space-y-3">
          <h2 class="card-title text-base">Details</h2>
          <div class="form-control">
            <label class="label py-1"><span class="label-text">Name</span></label>
            <input v-model="form.name" type="text" class="input input-bordered input-sm" :disabled="!editing || saving" />
          </div>
          <div class="form-control">
            <label class="label py-1"><span class="label-text">Email</span></label>
            <input v-model="form.email" type="email" class="input input-bordered input-sm" :disabled="!editing || saving" />
          </div>
          <div class="form-control">
            <label class="label py-1"><span class="label-text">Phone</span></label>
            <input v-model="form.phone" type="tel" class="input input-bordered input-sm" placeholder="+1 555-555-0100" :disabled="!editing || saving" />
          </div>
          <div class="form-control">
            <label class="label py-1"><span class="label-text">Customer</span></label>
            <SearchSelect v-model="form.customer" :options="customerOptions" size="sm" placeholder="Customer…" :disabled="!editing || saving" />
          </div>
          <div class="form-control">
            <label class="label cursor-pointer justify-start gap-3 py-1">
              <input v-model="form.active" type="checkbox" class="toggle toggle-success toggle-sm" :disabled="!editing || saving" />
              <span class="label-text">Active</span>
            </label>
          </div>
          <div v-if="canEdit && editing" class="pt-1">
            <button class="btn btn-ghost btn-sm" @click="resetPassword">Reset password</button>
          </div>
        </div>
      </div>

      <div class="card bg-base-100 shadow-sm">
        <div class="card-body">
          <div class="flex items-center justify-between">
            <h2 class="card-title text-base">Recent Tickets</h2>
            <router-link :to="`/staff/tickets?search=${encodeURIComponent(requester.email)}`" class="link link-hover text-sm">View all →</router-link>
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
    </div>
  </div>
</template>
