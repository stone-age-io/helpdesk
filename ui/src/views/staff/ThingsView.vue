<script setup lang="ts">
// Things roster: the devices and equipment tickets are filed against. A subset
// mirror of the platform's `things` — `code` is the join key machine intakes
// resolve against (docs/protocol.md) — and deliberately a SUPERSET of it, since
// MSP work covers printers, door strikes and customer switches that were never
// onboarded to the control plane. Those rows simply have no code.
//
// Any staff can create/edit via the detail view; only delete stays admin-only,
// matching locations after migration 1813000000.
//
// Search / customer / type come from the shared RosterFilters (see that
// component for why the type filter keys on the type NAME rather than its id);
// the in-service toggle is this roster's own and rides its slot.
//
// SCALING: loaded whole via getFullList like the other Directory rosters, which
// pages at 500/request under the hood. Things are an order or two more numerous
// than locations, so this is the roster most likely to outgrow the pattern. When
// it does, the move is TicketQueueView's server-side getList + buildFilter +
// Pager — not a bigger client-side filter.
import { computed, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { pb } from '@/pb'
import type { Customer, Location, RecordType, Thing } from '@/types'
import ResponsiveList, { type Column } from '@/components/ResponsiveList.vue'
import RosterFilters from '@/components/RosterFilters.vue'

const router = useRouter()

const columns: Column<Thing>[] = [
  { key: 'name', label: 'Name' },
  { key: 'expand.customer.name', label: 'Customer' },
  { key: 'code', label: 'Code' },
  { key: 'expand.type.name', label: 'Type' },
  { key: 'expand.location.name', label: 'Location' },
  { key: 'retired', label: 'Status' },
]

const things = ref<Thing[]>([])
const customers = ref<Customer[]>([])
const loading = ref(true)
const error = ref('')

const search = ref('')
const customerFilter = ref('')
const typeFilter = ref('')
// A mirror accumulates decommissioned gear; in-service is the useful default.
const statusFilter = ref<'in-service' | 'all' | 'retired'>('in-service')

const filtered = computed(() => {
  const q = search.value.trim().toLowerCase()
  return things.value.filter((t) => {
    if (customerFilter.value && t.customer !== customerFilter.value) return false
    if (typeFilter.value && (t.expand?.type as RecordType | undefined)?.name !== typeFilter.value) return false
    if (statusFilter.value === 'in-service' && t.retired) return false
    if (statusFilter.value === 'retired' && !t.retired) return false
    if (!q) return true
    return [
      t.name,
      t.code,
      t.notes,
      (t.expand?.type as RecordType | undefined)?.name,
      (t.expand?.location as Location | undefined)?.name,
      (t.expand?.customer as Customer | undefined)?.name,
    ].some((v) => (v || '').toLowerCase().includes(q))
  })
})

async function load() {
  loading.value = true
  error.value = ''
  try {
    things.value = await pb.collection('things').getFullList<Thing>({
      sort: 'name',
      expand: 'customer,type,location',
    })
  } catch (err: any) {
    error.value = err?.message || 'Failed to load things'
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

function openDetail(thing: Thing) {
  router.push(`/staff/things/${thing.id}`)
}

onMounted(() => {
  load()
  loadCustomers()
})
</script>

<template>
  <div class="space-y-4">
    <div class="flex flex-col sm:flex-row justify-between items-start sm:items-center gap-2">
      <h1 class="text-2xl font-bold">Things</h1>
      <router-link to="/staff/things/new" class="btn btn-primary btn-sm w-full sm:w-auto">New Thing</router-link>
    </div>
    <p class="text-sm text-base-content/60">
      The devices and equipment tickets are filed against. A curated mirror of the
      platform's things, joined by <span class="font-mono">customer + code</span> — plus
      the gear the platform never onboarded, which simply has no code. Naming a thing on
      a ticket is what makes "everything that ever happened to this device" answerable.
    </p>

    <div v-if="error" class="alert alert-error py-2 text-sm">{{ error }}</div>

    <RosterFilters
      v-model:search="search"
      v-model:customer="customerFilter"
      v-model:type="typeFilter"
      :items="things"
      :customers="customers"
      search-placeholder="Filter by name, code, type, location…"
    >
      <select v-model="statusFilter" class="select select-bordered select-sm w-full sm:w-40">
        <option value="in-service">In service</option>
        <option value="all">All</option>
        <option value="retired">Retired</option>
      </select>
    </RosterFilters>

    <div v-if="loading" class="flex justify-center p-12"><span class="loading loading-spinner loading-lg"></span></div>

    <ResponsiveList v-else :items="filtered" :columns="columns" clickable @row-click="openDetail">
      <template #cell-name="{ value }"><span class="font-medium text-sm">{{ value }}</span></template>
      <template #cell-code="{ value }"><span class="font-mono text-xs">{{ value || '—' }}</span></template>
      <template #cell-retired="{ item }">
        <span v-if="item.retired" class="badge badge-sm badge-soft">Retired</span>
        <span v-else class="badge badge-sm badge-soft-success">In service</span>
      </template>
      <template #empty>
        <span class="text-base-content/60">No things{{ search || customerFilter || typeFilter ? ' match.' : ' yet.' }}</span>
      </template>
    </ResponsiveList>
  </div>
</template>
