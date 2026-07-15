<script setup lang="ts">
import { onMounted, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import { pb } from '@/pb'
import { useAuthStore } from '@/stores/auth'
import type { Customer } from '@/types'
import ResponsiveList, { type Column } from '@/components/ResponsiveList.vue'
import ActiveBadge from '@/components/ActiveBadge.vue'
import Pager from '@/components/Pager.vue'

const auth = useAuthStore()
const router = useRouter()

const columns: Column<Customer>[] = [
  { key: 'name', label: 'Name' },
  { key: 'platform_org_id', label: 'Platform Org', mobileLabel: 'Org' },
  { key: 'active', label: 'Active', mobileLabel: 'Status' },
]

const customers = ref<Customer[]>([])
const loading = ref(true)
const error = ref('')
const search = ref('')

const showForm = ref(false)
const newName = ref('')
const saving = ref(false)

const page = ref(1)
const totalPages = ref(1)
const perPage = 30

// Search runs server-side (name / platform org id) so paging stays correct on a
// large roster — same shape as the requester and ticket lists.
function buildFilter(): string {
  const raw = search.value.trim()
  if (!raw) return ''
  const q = raw.replace(/'/g, "\\'")
  return `(name ~ '${q}' || platform_org_id ~ '${q}')`
}

async function load() {
  loading.value = true
  error.value = ''
  try {
    const res = await pb.collection('customers').getList<Customer>(page.value, perPage, { sort: 'name', filter: buildFilter() })
    customers.value = res.items
    totalPages.value = res.totalPages
  } catch (err: any) {
    error.value = err?.message || 'Failed to load customers'
  } finally {
    loading.value = false
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
  if (!newName.value.trim()) return
  saving.value = true
  try {
    await pb.collection('customers').create({ name: newName.value.trim(), active: true })
    newName.value = ''
    showForm.value = false
    await load()
  } catch (err: any) {
    error.value = err?.message || 'Failed to create customer'
  } finally {
    saving.value = false
  }
}

onMounted(load)
</script>

<template>
  <div class="space-y-4">
    <div class="flex flex-col sm:flex-row justify-between items-start sm:items-center gap-2">
      <h1 class="text-2xl font-bold">Customers</h1>
      <button v-if="auth.isAdmin" class="btn btn-primary btn-sm w-full sm:w-auto" @click="showForm = !showForm">
        {{ showForm ? 'Cancel' : 'New Customer' }}
      </button>
    </div>

    <form v-if="showForm" class="flex gap-2" @submit.prevent="create">
      <input v-model="newName" type="text" placeholder="Company name" class="input input-bordered input-sm flex-1 max-w-sm" required :disabled="saving" />
      <button type="submit" class="btn btn-primary btn-sm" :disabled="saving || !newName.trim()">Create</button>
    </form>

    <input v-model="search" type="search" placeholder="Filter by name or platform org…" class="input input-bordered input-sm w-full sm:w-72" />

    <div v-if="loading" class="flex justify-center p-12"><span class="loading loading-spinner loading-lg"></span></div>
    <div v-else-if="error" class="alert alert-error">{{ error }}</div>

    <ResponsiveList
      v-else
      :items="customers"
      :columns="columns"
      @row-click="(c) => router.push(`/staff/customers/${c.id}`)"
    >
      <template #cell-name="{ value }"><span class="font-medium text-sm">{{ value }}</span></template>
      <template #cell-platform_org_id="{ value }"><span class="font-mono text-xs">{{ value || '—' }}</span></template>
      <template #cell-active="{ value }"><ActiveBadge :active="value" /></template>
      <template #empty>
        <span class="text-base-content/60">No customers{{ search ? ' match.' : ' yet.' }}</span>
      </template>
    </ResponsiveList>

    <Pager v-model:page="page" :total-pages="totalPages" />
  </div>
</template>
