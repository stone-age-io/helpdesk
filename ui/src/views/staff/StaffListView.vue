<script setup lang="ts">
import { onMounted, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import { pb } from '@/pb'
import { useAuthStore } from '@/stores/auth'
import type { Staff } from '@/types'
import ResponsiveList, { type Column } from '@/components/ResponsiveList.vue'
import ActiveBadge from '@/components/ActiveBadge.vue'
import Avatar from '@/components/Avatar.vue'
import Pager from '@/components/Pager.vue'

const auth = useAuthStore()
const router = useRouter()

const columns: Column<Staff>[] = [
  { key: 'name', label: 'Name' },
  { key: 'email', label: 'Email' },
  { key: 'role', label: 'Role' },
  { key: 'active', label: 'Status' },
]

const staff = ref<Staff[]>([])
const loading = ref(true)
const error = ref('')

const page = ref(1)
const totalPages = ref(1)
const perPage = 30

const showForm = ref(false)
const saving = ref(false)
const form = ref({ email: '', name: '', role: 'agent' as 'agent' | 'admin' })

const issued = ref<{ email: string; password: string } | null>(null)

function generatePassword(): string {
  const bytes = new Uint8Array(12)
  crypto.getRandomValues(bytes)
  return btoa(String.fromCharCode(...bytes)).replace(/[+/=]/g, '').slice(0, 16)
}

async function load() {
  loading.value = true
  error.value = ''
  try {
    const res = await pb.collection('staff').getList<Staff>(page.value, perPage, { sort: 'name' })
    staff.value = res.items
    totalPages.value = res.totalPages
  } catch (err: any) {
    error.value = err?.message || 'Failed to load staff'
  } finally {
    loading.value = false
  }
}

watch(page, () => load())

async function create() {
  saving.value = true
  error.value = ''
  try {
    const password = generatePassword()
    await pb.collection('staff').create({
      email: form.value.email,
      name: form.value.name,
      role: form.value.role,
      active: true,
      password,
      passwordConfirm: password,
    })
    issued.value = { email: form.value.email, password }
    form.value = { email: '', name: '', role: 'agent' }
    showForm.value = false
    await load()
  } catch (err: any) {
    error.value = err?.data?.message || err?.message || 'Failed to create staff account'
  } finally {
    saving.value = false
  }
}

function openDetail(s: Staff) {
  router.push(`/staff/staff/${s.id}`)
}

const isSelf = (s: Staff) => s.id === auth.record?.id

onMounted(load)
</script>

<template>
  <div class="space-y-4">
    <div class="flex flex-col sm:flex-row justify-between items-start sm:items-center gap-2">
      <h1 class="text-2xl font-bold">Staff</h1>
      <button class="btn btn-primary btn-sm w-full sm:w-auto" @click="showForm = !showForm">
        {{ showForm ? 'Cancel' : 'New Staff' }}
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

    <form v-if="showForm" class="flex flex-wrap gap-2" @submit.prevent="create">
      <input v-model="form.email" type="email" placeholder="email" class="input input-bordered input-sm w-56" required :disabled="saving" />
      <input v-model="form.name" type="text" placeholder="name" class="input input-bordered input-sm w-44" required :disabled="saving" />
      <select v-model="form.role" class="select select-bordered select-sm" :disabled="saving">
        <option value="agent">agent</option>
        <option value="admin">admin</option>
      </select>
      <button type="submit" class="btn btn-primary btn-sm" :disabled="saving || !form.email || !form.name">Create</button>
    </form>

    <div v-if="loading" class="flex justify-center p-12"><span class="loading loading-spinner loading-lg"></span></div>

    <ResponsiveList v-else :items="staff" :columns="columns" clickable @row-click="openDetail">
      <template #cell-name="{ item }">
        <div class="flex items-center gap-2">
          <Avatar :record="item" :name="item.name || item.email" size="xs" />
          <span class="font-medium text-sm">{{ item.name }}</span>
          <span v-if="isSelf(item)" class="badge-soft badge-soft-neutral">you</span>
        </div>
      </template>
      <template #card-name="{ item }">
        <div class="flex items-center gap-2 min-w-0">
          <Avatar :record="item" :name="item.name || item.email" size="xs" />
          <div class="text-sm font-bold truncate">
            {{ item.name }}<span v-if="isSelf(item)" class="badge-soft badge-soft-neutral ml-2">you</span>
          </div>
        </div>
      </template>
      <template #cell-role="{ value }"><span class="badge-soft badge-soft-neutral">{{ value }}</span></template>
      <template #cell-active="{ value }"><ActiveBadge :active="value" /></template>
      <template #empty>
        <span class="text-base-content/60">No staff accounts.</span>
      </template>
    </ResponsiveList>

    <Pager v-model:page="page" :total-pages="totalPages" />
  </div>
</template>
