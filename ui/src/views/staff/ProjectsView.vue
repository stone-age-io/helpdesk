<script setup lang="ts">
// Projects: the durable container grouping 1..N tickets (installs + reactive
// work) at a customer location over a target window. This is the list; creation
// and everything else (linked tickets, derived crew and time) lives on the
// detail/form view, reached via "New Project" — consistent with the other
// objects (customers, locations).
import { computed, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { pb } from '@/pb'
import { useAuthStore } from '@/stores/auth'
import type { Customer, Project, ProjectStatus } from '@/types'
import { PROJECT_STATUSES } from '@/types'
import SearchSelect from '@/components/SearchSelect.vue'
import ResponsiveList, { type Column } from '@/components/ResponsiveList.vue'
import { format } from 'date-fns'

const router = useRouter()
const auth = useAuthStore()

const columns: Column<Project>[] = [
  { key: 'title', label: 'Title' },
  { key: 'expand.customer.name', label: 'Customer' },
  { key: 'expand.location.name', label: 'Location' },
  { key: 'status', label: 'Status' },
  { key: 'target_date', label: 'Target', format: (v) => fmtDate(v) },
  { key: 'expand.lead.name', label: 'Lead' },
]

const projects = ref<Project[]>([])
const customers = ref<Customer[]>([])
const loading = ref(true)
const error = ref('')

// Filters.
const statusFilter = ref<'' | ProjectStatus>('')
const customerFilter = ref('')
const search = ref('')

const customerOptions = computed(() => customers.value.map((c) => ({ id: c.id, label: c.name })))

const filtered = computed(() => {
  const q = search.value.trim().toLowerCase()
  return projects.value.filter(
    (p) =>
      (!statusFilter.value || p.status === statusFilter.value) &&
      (!customerFilter.value || p.customer === customerFilter.value) &&
      (!q ||
        p.title.toLowerCase().includes(q) ||
        String(p.number).includes(q) ||
        (p.expand?.customer?.name || '').toLowerCase().includes(q) ||
        (p.expand?.location?.name || '').toLowerCase().includes(q)),
  )
})

function fmtDate(s?: string): string {
  return s ? format(new Date(s), 'MMM d, yyyy') : '—'
}

const statusClass: Record<ProjectStatus, string> = {
  planned: 'badge-soft-neutral',
  active: 'badge-soft-info',
  completed: 'badge-soft-success',
  canceled: 'badge-soft-neutral opacity-60',
}

async function load() {
  loading.value = true
  error.value = ''
  try {
    customers.value = await pb.collection('customers').getFullList<Customer>({ sort: 'name' })
    projects.value = await pb
      .collection('projects')
      .getFullList<Project>({ sort: '-created', expand: 'customer,location,lead' })
  } catch (err: any) {
    error.value = err?.message || 'Failed to load projects'
  } finally {
    loading.value = false
  }
}

onMounted(load)
</script>

<template>
  <div class="space-y-4">
    <div class="flex flex-col sm:flex-row justify-between items-start sm:items-center gap-2">
      <h1 class="text-2xl font-bold">Projects</h1>
      <router-link v-if="!auth.isField" to="/staff/projects/new" class="btn btn-primary btn-sm w-full sm:w-auto">New Project</router-link>
    </div>
    <p class="text-sm text-base-content/60">
      Installations and multi-visit field work. A project groups its tickets at a
      customer location over a target window.
    </p>

    <div v-if="error" class="alert alert-error py-2 text-sm">{{ error }}</div>

    <!-- Filters -->
    <div class="flex flex-col sm:flex-row sm:flex-wrap gap-2 sm:items-end">
      <input v-model="search" type="search" placeholder="Search #, title, customer, location…" class="input input-bordered input-sm w-full sm:w-64" />
      <select v-model="statusFilter" class="select select-bordered select-sm w-full sm:w-auto">
        <option value="">All statuses</option>
        <option v-for="s in PROJECT_STATUSES" :key="s" :value="s">{{ s }}</option>
      </select>
      <div class="w-full sm:w-56"><SearchSelect v-model="customerFilter" :options="customerOptions" size="sm" empty-label="All customers" placeholder="Any customer…" /></div>
    </div>

    <div v-if="loading" class="flex justify-center p-12"><span class="loading loading-spinner loading-lg"></span></div>

    <ResponsiveList
      v-else
      :items="filtered"
      :columns="columns"
      @row-click="(p: Project) => router.push(`/staff/projects/${p.id}`)"
    >
      <template #cell-title="{ item }">
        <span class="font-mono text-xs text-base-content/50 mr-1.5">#{{ item.number }}</span>
        <span class="font-medium text-sm">{{ item.title }}</span>
      </template>
      <template #card-title="{ item }">
        <div class="text-sm font-bold text-primary truncate">
          <span class="font-mono text-xs opacity-60 mr-1">#{{ item.number }}</span>{{ item.title }}
        </div>
      </template>
      <template #cell-status="{ item }">
        <span class="badge-soft" :class="statusClass[item.status]">{{ item.status }}</span>
      </template>
      <template #empty>
        <span class="text-base-content/60">No projects{{ statusFilter || customerFilter || search ? ' match the filters' : ' yet' }}.</span>
      </template>
    </ResponsiveList>
  </div>
</template>
