<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { pb } from '@/pb'
import { useAuthStore } from '@/stores/auth'
import type { Customer } from '@/types'
import ResponsiveList, { type Column } from '@/components/ResponsiveList.vue'
import ActiveBadge from '@/components/ActiveBadge.vue'

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

const showForm = ref(false)
const newName = ref('')
const saving = ref(false)

async function load() {
  loading.value = true
  error.value = ''
  try {
    customers.value = await pb.collection('customers').getFullList<Customer>({ sort: 'name' })
  } catch (err: any) {
    error.value = err?.message || 'Failed to load customers'
  } finally {
    loading.value = false
  }
}

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
        <span class="text-base-content/60">No customers yet.</span>
      </template>
    </ResponsiveList>
  </div>
</template>
