<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { pb } from '@/pb'
import { useAuthStore } from '@/stores/auth'
import type { Customer, Requester } from '@/types'
import SearchSelect from '@/components/SearchSelect.vue'

const auth = useAuthStore()

const requesters = ref<Requester[]>([])
const customers = ref<Customer[]>([])
const loading = ref(true)
const error = ref('')
const search = ref('')

const showForm = ref(false)
const saving = ref(false)
const form = ref({ email: '', name: '', customer: '' })

// One-time credential banner: shown after create or password reset, never
// persisted anywhere client-side beyond this ref.
const issued = ref<{ email: string; password: string } | null>(null)

// Inline row editing (admin): name / email / customer.
const editing = ref<Requester | null>(null)
const editForm = ref({ name: '', email: '', customer: '' })

const customerOptions = computed(() => customers.value.map((c) => ({ id: c.id, label: c.name })))

const filtered = computed(() => {
  const q = search.value.trim().toLowerCase()
  if (!q) return requesters.value
  return requesters.value.filter(
    (r) =>
      r.email.toLowerCase().includes(q) ||
      r.name?.toLowerCase().includes(q) ||
      r.expand?.customer?.name?.toLowerCase().includes(q),
  )
})

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
    issued.value = { email: form.value.email, password }
    form.value = { email: '', name: '', customer: '' }
    showForm.value = false
    await load()
  } catch (err: any) {
    error.value = err?.data?.message || err?.message || 'Failed to create requester'
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

async function resetPassword(r: Requester) {
  if (!confirm(`Reset the password for ${r.email}? Their current password stops working immediately.`)) return
  error.value = ''
  try {
    const password = generatePassword()
    await pb.collection('users').update(r.id, { password, passwordConfirm: password })
    issued.value = { email: r.email, password }
  } catch (err: any) {
    error.value = err?.data?.message || err?.message || 'Failed to reset password'
  }
}

function startEdit(r: Requester) {
  editing.value = r
  editForm.value = { name: r.name || '', email: r.email, customer: r.customer }
}

async function saveEdit() {
  if (!editing.value) return
  saving.value = true
  error.value = ''
  try {
    await pb.collection('users').update(editing.value.id, {
      name: editForm.value.name,
      email: editForm.value.email,
      customer: editForm.value.customer,
    })
    editing.value = null
    await load()
  } catch (err: any) {
    error.value = err?.data?.message || err?.message || 'Failed to save'
  } finally {
    saving.value = false
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

    <div v-if="issued" class="alert alert-info py-2 text-sm">
      <span>
        Credentials for <b>{{ issued.email }}</b> — one-time password:
        <code class="font-mono font-bold">{{ issued.password }}</code>
        — share securely, then <button class="link" @click="issued = null">dismiss</button>.
      </span>
    </div>

    <form v-if="showForm" class="flex flex-wrap gap-2 items-start" @submit.prevent="create">
      <input v-model="form.email" type="email" placeholder="email" class="input input-bordered input-sm w-56" required :disabled="saving" />
      <input v-model="form.name" type="text" placeholder="name" class="input input-bordered input-sm w-44" :disabled="saving" />
      <div class="w-56">
        <SearchSelect v-model="form.customer" :options="customerOptions" size="sm" placeholder="Customer…" :disabled="saving" />
      </div>
      <button type="submit" class="btn btn-primary btn-sm" :disabled="saving || !form.email || !form.customer">Create</button>
    </form>

    <input v-model="search" type="search" placeholder="Filter by email, name, customer…" class="input input-bordered input-sm w-full sm:w-72" />

    <div v-if="loading" class="flex justify-center p-12"><span class="loading loading-spinner loading-lg"></span></div>
    <div v-else-if="filtered.length === 0" class="text-center p-12 text-base-content/60">No requester accounts match.</div>

    <div v-else class="overflow-x-auto bg-base-100 rounded-lg shadow-sm">
      <table class="table table-sm">
        <thead>
          <tr>
            <th>Email</th>
            <th>Name</th>
            <th>Customer</th>
            <th>Status</th>
            <th v-if="auth.isAdmin" class="text-right">Actions</th>
          </tr>
        </thead>
        <tbody>
          <template v-for="r in filtered" :key="r.id">
            <tr>
              <td>{{ r.email }}</td>
              <td>{{ r.name || '—' }}</td>
              <td>{{ r.expand?.customer?.name || '—' }}</td>
              <td>
                <span class="badge badge-sm" :class="r.active ? 'badge-success' : 'badge-ghost'">
                  {{ r.active ? 'active' : 'inactive' }}
                </span>
              </td>
              <td v-if="auth.isAdmin" class="text-right whitespace-nowrap">
                <button class="btn btn-ghost btn-xs" @click="editing?.id === r.id ? (editing = null) : startEdit(r)">
                  {{ editing?.id === r.id ? 'Cancel' : 'Edit' }}
                </button>
                <button class="btn btn-ghost btn-xs" @click="resetPassword(r)">Reset password</button>
                <button class="btn btn-ghost btn-xs" @click="toggleActive(r)">
                  {{ r.active ? 'Deactivate' : 'Activate' }}
                </button>
              </td>
            </tr>
            <tr v-if="editing?.id === r.id" class="bg-base-200/50">
              <td :colspan="auth.isAdmin ? 5 : 4">
                <form class="flex flex-wrap gap-2 items-start py-1" @submit.prevent="saveEdit">
                  <input v-model="editForm.email" type="email" placeholder="email" class="input input-bordered input-sm w-56" required :disabled="saving" />
                  <input v-model="editForm.name" type="text" placeholder="name" class="input input-bordered input-sm w-44" :disabled="saving" />
                  <div class="w-56">
                    <SearchSelect v-model="editForm.customer" :options="customerOptions" size="sm" placeholder="Customer…" :disabled="saving" />
                  </div>
                  <button type="submit" class="btn btn-primary btn-sm" :disabled="saving || !editForm.email || !editForm.customer">Save</button>
                </form>
              </td>
            </tr>
          </template>
        </tbody>
      </table>
    </div>
  </div>
</template>
