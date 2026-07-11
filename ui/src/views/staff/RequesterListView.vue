<script setup lang="ts">
import { computed, nextTick, onMounted, ref } from 'vue'
import { pb } from '@/pb'
import { useAuthStore } from '@/stores/auth'
import type { Customer, Requester } from '@/types'
import SearchSelect from '@/components/SearchSelect.vue'
import ResponsiveList, { type Column } from '@/components/ResponsiveList.vue'
import ActiveBadge from '@/components/ActiveBadge.vue'

const auth = useAuthStore()

const columns: Column<Requester>[] = [
  { key: 'email', label: 'Email' },
  { key: 'name', label: 'Name' },
  { key: 'expand.customer.name', label: 'Customer' },
  { key: 'active', label: 'Status' },
]

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

// The edit panel renders above the list, which can be off-screen when the
// triggering row is below the fold — bring it into view.
const editCard = ref<HTMLElement | null>(null)
function startEdit(r: Requester) {
  editing.value = r
  editForm.value = { name: r.name || '', email: r.email, customer: r.customer }
  nextTick(() => editCard.value?.scrollIntoView({ behavior: 'smooth', block: 'nearest' }))
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

    <!-- Edit panel: lives above the list (an inline table row can't render
         inside the mobile card layout). -->
    <div v-if="editing" ref="editCard" class="card bg-base-100 shadow-sm">
      <div class="card-body p-4 space-y-2">
        <h2 class="card-title text-sm">Edit {{ editing.email }}</h2>
        <form class="flex flex-col sm:flex-row sm:flex-wrap gap-2 items-stretch sm:items-start" @submit.prevent="saveEdit">
          <input v-model="editForm.email" type="email" placeholder="email" class="input input-bordered input-sm w-full sm:w-56" required :disabled="saving" />
          <input v-model="editForm.name" type="text" placeholder="name" class="input input-bordered input-sm w-full sm:w-44" :disabled="saving" />
          <div class="w-full sm:w-56">
            <SearchSelect v-model="editForm.customer" :options="customerOptions" size="sm" placeholder="Customer…" :disabled="saving" />
          </div>
          <div class="flex gap-2">
            <button type="submit" class="btn btn-primary btn-sm" :disabled="saving || !editForm.email || !editForm.customer">Save</button>
            <button type="button" class="btn btn-ghost btn-sm" :disabled="saving" @click="editing = null">Cancel</button>
          </div>
        </form>
      </div>
    </div>

    <div v-if="loading" class="flex justify-center p-12"><span class="loading loading-spinner loading-lg"></span></div>

    <ResponsiveList v-else :items="filtered" :columns="columns" :clickable="false">
      <template #cell-email="{ value }"><span class="text-sm">{{ value }}</span></template>
      <template #card-email="{ value }"><div class="text-sm font-bold truncate">{{ value }}</div></template>
      <template #cell-active="{ value }"><ActiveBadge :active="value" /></template>
      <template #actions="{ item }">
        <router-link class="btn btn-ghost btn-xs" :to="`/staff/tickets?search=${encodeURIComponent(item.email)}`">Tickets</router-link>
        <template v-if="auth.isAdmin">
          <button class="btn btn-ghost btn-xs" @click="editing?.id === item.id ? (editing = null) : startEdit(item)">
            {{ editing?.id === item.id ? 'Cancel' : 'Edit' }}
          </button>
          <button class="btn btn-ghost btn-xs" @click="resetPassword(item)">Reset password</button>
          <button class="btn btn-ghost btn-xs" @click="toggleActive(item)">
            {{ item.active ? 'Deactivate' : 'Activate' }}
          </button>
        </template>
      </template>
      <template #empty>
        <span class="text-base-content/60">No requester accounts match.</span>
      </template>
    </ResponsiveList>
  </div>
</template>
