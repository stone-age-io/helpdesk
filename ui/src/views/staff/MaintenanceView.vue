<script setup lang="ts">
// Maintenance plans roster: the recurrence layer. A plan says "this gets
// serviced every N days" and its only output is an ordinary ticket, so nothing
// here duplicates the queue — rows click through to a detail view that owns the
// schedule and lists what it has generated.
//
// Any staff member creates and edits (delete stays admin, gated in the detail
// view), the same split migration 1813000000 gave locations: the person who
// learns a site needs quarterly service is usually the tech standing in it.
//
// Not in the Directory section of the sidebar — that one is lookups. A plan is
// work, so it sits beside Projects.
import { computed, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { pb } from '@/pb'
import type { Customer, MaintenancePlan } from '@/types'
import ResponsiveList, { type Column } from '@/components/ResponsiveList.vue'
import SearchSelect from '@/components/SearchSelect.vue'
import { formatDay, isPastDue } from '@/due'

const router = useRouter()

const plans = ref<MaintenancePlan[]>([])
const customers = ref<Customer[]>([])
const loading = ref(true)
const error = ref('')

const search = ref('')
const customerFilter = ref('')
const showPaused = ref(false)

const columns: Column<MaintenancePlan>[] = [
  { key: 'title', label: 'Plan' },
  { key: 'expand.customer.name', label: 'Customer' },
  { key: 'target', label: 'Target' },
  { key: 'interval_days', label: 'Every' },
  { key: 'next_due', label: 'Next due' },
  { key: 'expand.assignee.name', label: 'Assignee' },
  { key: 'paused', label: 'Status' },
]

const customerOptions = computed(() => customers.value.map((c) => ({ id: c.id, label: c.name })))

// Loaded whole (no pager) and filtered in the browser, same as the other
// rosters — plans number in the tens even for a large desk.
const filtered = computed(() => {
  const q = search.value.trim().toLowerCase()
  return plans.value.filter((p) => {
    if (!showPaused.value && p.paused) return false
    if (customerFilter.value && p.customer !== customerFilter.value) return false
    if (!q) return true
    return [p.title, p.expand?.customer?.name, p.expand?.thing?.name, p.expand?.location?.name]
      .some((v) => (v || '').toLowerCase().includes(q))
  })
})

// What the plan services: the device if it names one, else the site. Both are
// optional — a plan can be a plain recurring visit to a customer.
function targetLabel(p: MaintenancePlan): string {
  return p.expand?.thing?.name || p.expand?.location?.name || '—'
}

function isOverdue(p: MaintenancePlan): boolean {
  if (p.paused) return false
  return isPastDue(p.next_due)
}

async function load() {
  loading.value = true
  error.value = ''
  try {
    plans.value = await pb.collection('maintenance_plans').getFullList<MaintenancePlan>({
      sort: 'next_due',
      expand: 'customer,thing,location,assignee',
    })
  } catch (err: any) {
    error.value = err?.message || 'Failed to load maintenance plans'
  } finally {
    loading.value = false
  }
}

async function loadCustomers() {
  try {
    customers.value = await pb.collection('customers').getFullList<Customer>({
      sort: 'name',
      filter: 'active = true',
    })
  } catch {
    // The filter degrades to "all customers"; the list itself still loads.
  }
}

onMounted(() => {
  load()
  loadCustomers()
})
</script>

<template>
  <div class="space-y-4">
    <div class="flex flex-col sm:flex-row justify-between items-start sm:items-center gap-2">
      <h1 class="text-2xl font-bold">Maintenance</h1>
      <router-link to="/staff/maintenance/new" class="btn btn-primary btn-sm w-full sm:w-auto">New Plan</router-link>
    </div>
    <p class="text-sm text-base-content/60">
      Recurring preventive work. Each plan opens a planned ticket when it comes
      due — nightly, or on demand with
      <span class="font-mono text-xs">helpdesk maintenance-run</span>.
    </p>

    <div v-if="error" class="alert alert-error py-2 text-sm">{{ error }}</div>

    <div class="flex flex-col sm:flex-row sm:flex-wrap gap-2">
      <input
        v-model="search"
        type="text"
        placeholder="Filter by plan, customer, device, site…"
        class="input input-bordered input-sm w-full sm:flex-1 sm:min-w-52"
      />
      <div class="w-full sm:w-52">
        <SearchSelect v-model="customerFilter" :options="customerOptions" size="sm" empty-label="All customers" placeholder="Customer…" />
      </div>
      <label class="label cursor-pointer gap-2 justify-start">
        <input v-model="showPaused" type="checkbox" class="toggle toggle-sm" />
        <span class="label-text text-sm">Show paused</span>
      </label>
    </div>

    <div v-if="loading" class="flex justify-center p-12"><span class="loading loading-spinner loading-lg"></span></div>

    <ResponsiveList v-else :items="filtered" :columns="columns" clickable @row-click="(p) => router.push(`/staff/maintenance/${p.id}`)">
      <template #cell-title="{ value }"><span class="font-medium text-sm">{{ value }}</span></template>
      <template #cell-target="{ item }"><span class="text-sm">{{ targetLabel(item) }}</span></template>
      <!-- Plain days, matching what the form takes. A friendlier "3 mo" for 90
           would be a lie (three months is 89–92 days) and would disagree with
           the number you typed one click away. -->
      <template #cell-interval_days="{ value }"><span class="text-sm tabular-nums">{{ value }} days</span></template>
      <!-- An empty next_due is not "no date", it is PARKED: either the plan was
           never scheduled, or it is completion-anchored and waiting on its open
           ticket. Saying so is the whole reason that state is visible here —
           otherwise a plan legitimately waiting looks broken. -->
      <template #cell-next_due="{ item }">
        <span v-if="item.next_due" class="text-sm tabular-nums" :class="isOverdue(item) ? 'text-error font-medium' : ''">
          {{ formatDay(item.next_due, 'MMM d, yyyy') }}
        </span>
        <span v-else-if="item.anchor === 'completion'" class="text-sm text-base-content/50">
          Awaiting completion
        </span>
        <span v-else class="text-sm text-base-content/50">Not scheduled</span>
      </template>
      <!-- Same markup as ActiveBadge (badge-soft + a colour modifier, no raw
           `badge` class) so the soft-badge palette applies in both themes; the
           wording differs because `paused` is stored as the exception. -->
      <template #cell-paused="{ value }">
        <span class="badge-soft" :class="value ? 'badge-soft-neutral' : 'badge-soft-success'">
          <span class="badge-dot"></span>{{ value ? 'paused' : 'active' }}
        </span>
      </template>
      <template #empty>
        <span class="text-base-content/60">No maintenance plans{{ search || customerFilter ? ' match.' : ' yet.' }}</span>
      </template>
    </ResponsiveList>
  </div>
</template>
