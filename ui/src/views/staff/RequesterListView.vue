<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import { pb } from '@/pb'
import { useAuthStore } from '@/stores/auth'
import type { Customer, Requester } from '@/types'
import SearchSelect from '@/components/SearchSelect.vue'
import ResponsiveList, { type Column } from '@/components/ResponsiveList.vue'
import ActiveBadge from '@/components/ActiveBadge.vue'
import Avatar from '@/components/Avatar.vue'
import Pager from '@/components/Pager.vue'

const auth = useAuthStore()
const router = useRouter()

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

// One-time credential banner shown after create — never persisted client-side
// beyond this ref. Per-requester edits/resets now live on the detail view.
const issued = ref<{ email: string; password: string } | null>(null)

const customerOptions = computed(() => customers.value.map((c) => ({ id: c.id, label: c.name })))

const page = ref(1)
const totalPages = ref(1)
const perPage = 30

// Search runs server-side now — the roster can be large for a big MSP, so we
// page rather than pulling every requester to filter in the browser.
function buildFilter(): string {
  const raw = search.value.trim()
  if (!raw) return ''
  const q = raw.replace(/'/g, "\\'")
  return `(email ~ '${q}' || name ~ '${q}' || customer.name ~ '${q}')`
}

function generatePassword(): string {
  const bytes = new Uint8Array(12)
  crypto.getRandomValues(bytes)
  return btoa(String.fromCharCode(...bytes)).replace(/[+/=]/g, '').slice(0, 16)
}

async function load() {
  loading.value = true
  error.value = ''
  try {
    const res = await pb.collection('users').getList<Requester>(page.value, perPage, {
      sort: 'email',
      expand: 'customer',
      filter: buildFilter(),
    })
    requesters.value = res.items
    totalPages.value = res.totalPages
  } catch (err: any) {
    error.value = err?.message || 'Failed to load requesters'
  } finally {
    loading.value = false
  }
}

// The customer picker (create form) needs the full active list.
async function loadCustomers() {
  try {
    customers.value = await pb.collection('customers').getFullList<Customer>({ sort: 'name', filter: 'active = true' })
  } catch {
    // Picker degrades to empty; the roster still loads.
  }
}

watch(page, () => load())

let searchTimer: ReturnType<typeof setTimeout> | undefined
watch(search, () => {
  clearTimeout(searchTimer)
  searchTimer = setTimeout(() => {
    page.value = 1
    load()
  }, 300)
})

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

function openDetail(r: Requester) {
  router.push(`/staff/requesters/${r.id}`)
}

onMounted(() => {
  load()
  loadCustomers()
})
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

    <ResponsiveList v-else :items="requesters" :columns="columns" clickable @row-click="openDetail">
      <template #cell-email="{ item }">
        <div class="flex items-center gap-2">
          <Avatar :record="item" :name="item.name || item.email" size="xs" />
          <span class="text-sm">{{ item.email }}</span>
        </div>
      </template>
      <template #card-email="{ item }">
        <div class="flex items-center gap-2 min-w-0">
          <Avatar :record="item" :name="item.name || item.email" size="xs" />
          <div class="text-sm font-bold truncate">{{ item.email }}</div>
        </div>
      </template>
      <template #cell-active="{ value }"><ActiveBadge :active="value" /></template>
      <template #empty>
        <span class="text-base-content/60">No requester accounts match.</span>
      </template>
    </ResponsiveList>

    <Pager v-model:page="page" :total-pages="totalPages" />
  </div>
</template>
