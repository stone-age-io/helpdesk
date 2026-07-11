<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { pb } from '@/pb'
import { useAuthStore } from '@/stores/auth'
import type { Customer, Requester } from '@/types'

const auth = useAuthStore()

const requesters = ref<Requester[]>([])
const customers = ref<Customer[]>([])
const loading = ref(true)
const error = ref('')

const showForm = ref(false)
const saving = ref(false)
const form = ref({ email: '', name: '', customer: '' })
const createdPassword = ref('')

function generatePassword(): string {
  const bytes = new Uint8Array(12)
  crypto.getRandomValues(bytes)
  return btoa(String.fromCharCode(...bytes)).replace(/[+/=]/g, '').slice(0, 16)
}

async function load() {
  loading.value = true
  error.value = ''
  try {
    requesters.value = await pb.collection('users').getFullList<Requester>({ sort: 'email', expand: 'customer' })
    customers.value = await pb.collection('customers').getFullList<Customer>({ sort: 'name', filter: 'active = true' })
  } catch (err: any) {
    error.value = err?.message || 'Failed to load requesters'
  } finally {
    loading.value = false
  }
}

async function create() {
  saving.value = true
  error.value = ''
  try {
    const password = generatePassword()
    await pb.collection('users').create({
      email: form.value.email,
      name: form.value.name,
      customer: form.value.customer,
      active: true,
      password,
      passwordConfirm: password,
    })
    createdPassword.value = password
    form.value = { email: '', name: '', customer: '' }
    showForm.value = false
    await load()
  } catch (err: any) {
    error.value = err?.message || 'Failed to create requester'
  } finally {
    saving.value = false
  }
}

async function toggleActive(r: Requester) {
  try {
    await pb.collection('users').update(r.id, { active: !r.active })
    await load()
  } catch (err: any) {
    error.value = err?.message || 'Failed to update'
  }
}

onMounted(load)
</script>

<template>
  <div class="space-y-4">
    <div class="flex flex-col sm:flex-row justify-between items-start sm:items-center gap-2">
      <h1 class="text-2xl font-bold">Requesters</h1>
      <button v-if="auth.isAdmin" class="btn btn-primary btn-sm w-full sm:w-auto" @click="showForm = !showForm">
        {{ showForm ? 'Cancel' : 'New Requester' }}
      </button>
    </div>

    <div v-if="error" class="alert alert-error py-2 text-sm">{{ error }}</div>

    <div v-if="createdPassword" class="alert alert-info py-2 text-sm">
      <span>
        Account created. One-time password: <code class="font-mono font-bold">{{ createdPassword }}</code>
        — share securely, then <button class="link" @click="createdPassword = ''">dismiss</button>.
      </span>
    </div>

    <form v-if="showForm" class="flex flex-wrap gap-2" @submit.prevent="create">
      <input v-model="form.email" type="email" placeholder="email" class="input input-bordered input-sm w-56" required :disabled="saving" />
      <input v-model="form.name" type="text" placeholder="name" class="input input-bordered input-sm w-44" :disabled="saving" />
      <select v-model="form.customer" class="select select-bordered select-sm" required :disabled="saving">
        <option value="" disabled>Customer…</option>
        <option v-for="c in customers" :key="c.id" :value="c.id">{{ c.name }}</option>
      </select>
      <button type="submit" class="btn btn-primary btn-sm" :disabled="saving || !form.email || !form.customer">Create</button>
    </form>

    <div v-if="loading" class="flex justify-center p-12"><span class="loading loading-spinner loading-lg"></span></div>
    <div v-else-if="requesters.length === 0" class="text-center p-12 text-base-content/60">No requester accounts.</div>

    <div v-else class="overflow-x-auto bg-base-100 rounded-lg shadow-sm">
      <table class="table table-sm">
        <thead>
          <tr>
            <th>Email</th>
            <th>Name</th>
            <th>Customer</th>
            <th>Status</th>
            <th v-if="auth.isAdmin"></th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="r in requesters" :key="r.id">
            <td>{{ r.email }}</td>
            <td>{{ r.name || '—' }}</td>
            <td>{{ r.expand?.customer?.name || '—' }}</td>
            <td>
              <span class="badge badge-sm" :class="r.active ? 'badge-success' : 'badge-ghost'">
                {{ r.active ? 'active' : 'inactive' }}
              </span>
            </td>
            <td v-if="auth.isAdmin" class="text-right">
              <button class="btn btn-ghost btn-xs" @click="toggleActive(r)">
                {{ r.active ? 'Deactivate' : 'Activate' }}
              </button>
            </td>
          </tr>
        </tbody>
      </table>
    </div>
  </div>
</template>
