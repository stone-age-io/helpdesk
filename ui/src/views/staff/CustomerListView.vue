<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { pb } from '@/pb'
import { useAuthStore } from '@/stores/auth'
import type { Customer } from '@/types'

const auth = useAuthStore()

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
    <div v-else-if="customers.length === 0" class="text-center p-12 text-base-content/60">No customers yet.</div>

    <div v-else class="overflow-x-auto bg-base-100 rounded-lg shadow-sm">
      <table class="table table-sm">
        <thead>
          <tr>
            <th>Name</th>
            <th>Platform Org</th>
            <th>Active</th>
          </tr>
        </thead>
        <tbody>
          <tr
            v-for="c in customers"
            :key="c.id"
            class="hover cursor-pointer"
            @click="$router.push(`/staff/customers/${c.id}`)"
          >
            <td class="font-medium">{{ c.name }}</td>
            <td class="font-mono text-xs">{{ c.platform_org_id || '—' }}</td>
            <td>
              <span class="badge badge-sm" :class="c.active ? 'badge-success' : 'badge-ghost'">
                {{ c.active ? 'active' : 'inactive' }}
              </span>
            </td>
          </tr>
        </tbody>
      </table>
    </div>
  </div>
</template>
