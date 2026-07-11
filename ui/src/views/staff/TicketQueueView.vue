<script setup lang="ts">
import { onMounted, ref, watch } from 'vue'
import { pb } from '@/pb'
import type { Customer, Staff, Ticket, TicketStatus, TicketPriority } from '@/types'
import { TICKET_PRIORITIES, TICKET_STATUSES } from '@/types'
import TicketBadges from '@/components/TicketBadges.vue'
import { formatDistanceToNow } from 'date-fns'

const tickets = ref<Ticket[]>([])
const customers = ref<Customer[]>([])
const staff = ref<Staff[]>([])
const loading = ref(false)
const error = ref('')

const page = ref(1)
const totalPages = ref(1)
const perPage = 30

// Filters. Status defaults to "active" (everything not resolved/closed).
const status = ref<'active' | TicketStatus | ''>('active')
const priority = ref<TicketPriority | ''>('')
const customer = ref('')
const assignee = ref('')
const search = ref('')

function buildFilter(): string {
  const parts: string[] = []
  if (status.value === 'active') parts.push(`status != 'resolved' && status != 'closed'`)
  else if (status.value) parts.push(`status = '${status.value}'`)
  if (priority.value) parts.push(`priority = '${priority.value}'`)
  if (customer.value) parts.push(`customer = '${customer.value}'`)
  if (assignee.value === 'unassigned') parts.push(`assignee = ''`)
  else if (assignee.value) parts.push(`assignee = '${assignee.value}'`)
  if (search.value.trim()) {
    const q = search.value.trim().replace(/'/g, "\\'")
    parts.push(`(title ~ '${q}' || body ~ '${q}')`)
  }
  return parts.join(' && ')
}

async function load() {
  loading.value = true
  error.value = ''
  try {
    const result = await pb.collection('tickets').getList<Ticket>(page.value, perPage, {
      filter: buildFilter(),
      sort: '-created',
      expand: 'customer,assignee',
    })
    tickets.value = result.items
    totalPages.value = result.totalPages
  } catch (err: any) {
    error.value = err?.message || 'Failed to load tickets'
  } finally {
    loading.value = false
  }
}

async function loadFilterOptions() {
  try {
    customers.value = await pb.collection('customers').getFullList<Customer>({ sort: 'name' })
    staff.value = await pb.collection('staff').getFullList<Staff>({ sort: 'name', filter: 'active = true' })
  } catch {
    // Filter dropdowns degrade gracefully; the queue itself still loads.
  }
}

watch([status, priority, customer, assignee], () => {
  page.value = 1
  load()
})

let searchTimer: ReturnType<typeof setTimeout> | undefined
watch(search, () => {
  clearTimeout(searchTimer)
  searchTimer = setTimeout(() => {
    page.value = 1
    load()
  }, 300)
})

watch(page, load)

onMounted(() => {
  load()
  loadFilterOptions()
})
</script>

<template>
  <div class="space-y-4">
    <div class="flex flex-col sm:flex-row justify-between items-start sm:items-center gap-2">
      <h1 class="text-2xl font-bold">Tickets</h1>
      <router-link to="/staff/tickets/new" class="btn btn-primary btn-sm w-full sm:w-auto">New Ticket</router-link>
    </div>

    <div class="flex flex-wrap gap-2">
      <input v-model="search" type="search" placeholder="Search title or body…" class="input input-bordered input-sm w-full sm:w-64" />
      <select v-model="status" class="select select-bordered select-sm">
        <option value="active">Active</option>
        <option value="">All statuses</option>
        <option v-for="s in TICKET_STATUSES" :key="s" :value="s">{{ s.replace('_', ' ') }}</option>
      </select>
      <select v-model="priority" class="select select-bordered select-sm">
        <option value="">All priorities</option>
        <option v-for="p in TICKET_PRIORITIES" :key="p" :value="p">{{ p }}</option>
      </select>
      <select v-model="customer" class="select select-bordered select-sm">
        <option value="">All customers</option>
        <option v-for="c in customers" :key="c.id" :value="c.id">{{ c.name }}</option>
      </select>
      <select v-model="assignee" class="select select-bordered select-sm">
        <option value="">Anyone</option>
        <option value="unassigned">Unassigned</option>
        <option v-for="s in staff" :key="s.id" :value="s.id">{{ s.name }}</option>
      </select>
    </div>

    <div v-if="loading" class="flex justify-center p-12"><span class="loading loading-spinner loading-lg"></span></div>
    <div v-else-if="error" class="alert alert-error">{{ error }}</div>
    <div v-else-if="tickets.length === 0" class="text-center p-12 text-base-content/60">No tickets match.</div>

    <div v-else class="overflow-x-auto bg-base-100 rounded-lg shadow-sm">
      <table class="table table-sm">
        <thead>
          <tr>
            <th>#</th>
            <th>Title</th>
            <th>Customer</th>
            <th>Status</th>
            <th>Priority</th>
            <th>Assignee</th>
            <th>Age</th>
          </tr>
        </thead>
        <tbody>
          <tr
            v-for="t in tickets"
            :key="t.id"
            class="hover cursor-pointer"
            @click="$router.push(`/staff/tickets/${t.id}`)"
          >
            <td class="font-mono">{{ t.number }}</td>
            <td class="max-w-md truncate font-medium">{{ t.title }}</td>
            <td>{{ t.expand?.customer?.name || '—' }}</td>
            <td><TicketBadges :status="t.status" /></td>
            <td><TicketBadges :priority="t.priority" /></td>
            <td>{{ t.expand?.assignee?.name || '—' }}</td>
            <td class="whitespace-nowrap text-base-content/60">{{ formatDistanceToNow(new Date(t.created)) }}</td>
          </tr>
        </tbody>
      </table>
    </div>

    <div v-if="totalPages > 1" class="flex justify-center gap-2">
      <button class="btn btn-sm" :disabled="page <= 1" @click="page--">«</button>
      <span class="btn btn-sm btn-ghost no-animation">{{ page }} / {{ totalPages }}</span>
      <button class="btn btn-sm" :disabled="page >= totalPages" @click="page++">»</button>
    </div>
  </div>
</template>
