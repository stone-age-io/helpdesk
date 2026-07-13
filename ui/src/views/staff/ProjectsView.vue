<script setup lang="ts">
// Projects: the durable container grouping 1..N tickets (installs + reactive
// work) at a customer location over a target window. This is the list +
// quick-create; the detail view owns the rest (linked tickets, derived crew
// and time). A project is created minimally here, then enriched on its page.
import { computed, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { pb } from '@/pb'
import type { Customer, Project, ProjectStatus } from '@/types'
import { PROJECT_STATUSES } from '@/types'
import SearchSelect from '@/components/SearchSelect.vue'
import { format } from 'date-fns'

const router = useRouter()

const projects = ref<Project[]>([])
const customers = ref<Customer[]>([])
const loading = ref(true)
const error = ref('')

// Filters.
const statusFilter = ref<'' | ProjectStatus>('')
const customerFilter = ref('')

// New-project form.
const nu = ref({ customer: '', title: '', target_date: '' })
const creating = ref(false)

const customerOptions = computed(() => customers.value.map((c) => ({ id: c.id, label: c.name })))

const filtered = computed(() =>
  projects.value.filter(
    (p) =>
      (!statusFilter.value || p.status === statusFilter.value) &&
      (!customerFilter.value || p.customer === customerFilter.value),
  ),
)

function fmtDate(s?: string): string {
  return s ? format(new Date(s), 'MMM d, yyyy') : '—'
}

const statusClass: Record<ProjectStatus, string> = {
  planned: 'badge-ghost',
  active: 'badge-info',
  completed: 'badge-success',
  canceled: 'badge-ghost opacity-60',
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

async function create() {
  const title = nu.value.title.trim()
  if (!nu.value.customer || !title) return
  creating.value = true
  error.value = ''
  try {
    const rec = await pb.collection('projects').create({
      customer: nu.value.customer,
      title,
      target_date: nu.value.target_date || '',
      status: 'planned',
    })
    router.push(`/staff/projects/${rec.id}`)
  } catch (err: any) {
    error.value = err?.message || 'Failed to create project'
    creating.value = false
  }
}

onMounted(load)
</script>

<template>
  <div class="space-y-4">
    <h1 class="text-2xl font-bold">Projects</h1>
    <p class="text-sm text-base-content/60">
      Installations and multi-visit field work. A project groups its tickets at a
      customer location over a target window.
    </p>

    <div v-if="error" class="alert alert-error py-2 text-sm">{{ error }}</div>

    <!-- New project -->
    <form class="flex flex-col sm:flex-row flex-wrap gap-2 sm:items-end" @submit.prevent="create">
      <div class="form-control">
        <label class="label py-1"><span class="label-text text-xs">Customer *</span></label>
        <div class="w-full sm:w-56">
          <SearchSelect v-model="nu.customer" :options="customerOptions" size="sm" placeholder="Customer…" :disabled="creating" />
        </div>
      </div>
      <div class="form-control">
        <label class="label py-1"><span class="label-text text-xs">Title *</span></label>
        <input v-model="nu.title" type="text" placeholder="e.g. HQ Security Rollout" class="input input-bordered input-sm w-full sm:w-64" :disabled="creating" />
      </div>
      <div class="form-control">
        <label class="label py-1"><span class="label-text text-xs">Target date</span></label>
        <input v-model="nu.target_date" type="date" class="input input-bordered input-sm" :disabled="creating" />
      </div>
      <button type="submit" class="btn btn-primary btn-sm" :disabled="creating || !nu.customer || !nu.title.trim()">
        <span v-if="creating" class="loading loading-spinner loading-xs"></span>
        Create
      </button>
    </form>

    <!-- Filters -->
    <div class="flex flex-wrap gap-2 items-end">
      <div class="form-control">
        <label class="label py-1"><span class="label-text text-xs">Status</span></label>
        <select v-model="statusFilter" class="select select-bordered select-sm">
          <option value="">All statuses</option>
          <option v-for="s in PROJECT_STATUSES" :key="s" :value="s">{{ s }}</option>
        </select>
      </div>
      <div class="form-control">
        <label class="label py-1"><span class="label-text text-xs">Customer</span></label>
        <div class="w-56"><SearchSelect v-model="customerFilter" :options="customerOptions" size="sm" empty-label="All customers" placeholder="Any customer…" /></div>
      </div>
    </div>

    <div v-if="loading" class="flex justify-center p-12"><span class="loading loading-spinner loading-lg"></span></div>

    <div v-else class="overflow-x-auto bg-base-100 rounded-lg shadow-sm">
      <table class="table table-sm">
        <thead>
          <tr>
            <th class="w-16">#</th>
            <th>Title</th>
            <th>Customer</th>
            <th>Location</th>
            <th>Status</th>
            <th>Target</th>
            <th>Lead</th>
          </tr>
        </thead>
        <tbody>
          <tr
            v-for="p in filtered"
            :key="p.id"
            class="hover cursor-pointer"
            @click="router.push(`/staff/projects/${p.id}`)"
          >
            <td class="font-mono text-base-content/60">{{ p.number }}</td>
            <td class="font-medium">{{ p.title }}</td>
            <td class="text-base-content/70">{{ p.expand?.customer?.name || '—' }}</td>
            <td class="text-base-content/70">{{ p.expand?.location?.name || '—' }}</td>
            <td><span class="badge badge-sm" :class="statusClass[p.status]">{{ p.status }}</span></td>
            <td class="text-base-content/70 whitespace-nowrap">{{ fmtDate(p.target_date) }}</td>
            <td class="text-base-content/70">{{ p.expand?.lead?.name || '—' }}</td>
          </tr>
          <tr v-if="filtered.length === 0">
            <td colspan="7" class="text-base-content/50">No projects{{ statusFilter || customerFilter ? ' match the filters' : ' yet' }}.</td>
          </tr>
        </tbody>
      </table>
    </div>
  </div>
</template>
