<script setup lang="ts">
// Locations roster: a customer's physical places. `code` is the platform
// Location join key that machine intakes resolve against (see docs/protocol.md);
// address + access notes make on-site visits better. In the Directory now — any
// staff can create/edit via the detail view (migration 1813000000); only delete
// stays admin-only. Rows click through to the detail/edit view; the shared
// ResponsiveList gives the dense desktop table + stacked mobile cards.
import { computed, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { pb } from '@/pb'
import type { Location } from '@/types'
import ResponsiveList, { type Column } from '@/components/ResponsiveList.vue'

const router = useRouter()

const columns: Column<Location>[] = [
  { key: 'name', label: 'Name' },
  { key: 'expand.customer.name', label: 'Customer' },
  { key: 'code', label: 'Code' },
  { key: 'address', label: 'Address' },
  { key: 'contact', label: 'Contact' },
  { key: 'notes', label: 'Notes' },
]

const locations = ref<Location[]>([])
const loading = ref(true)
const error = ref('')
const search = ref('')

// Client-side filter — the roster is loaded whole (getFullList, no pager), so a
// browser-side match over the visible fields is simplest and instant.
const filtered = computed(() => {
  const q = search.value.trim().toLowerCase()
  if (!q) return locations.value
  return locations.value.filter((l) =>
    [l.name, l.code, l.address, l.contact, l.expand?.customer?.name]
      .some((v) => (v || '').toLowerCase().includes(q)),
  )
})

async function load() {
  loading.value = true
  error.value = ''
  try {
    locations.value = await pb
      .collection('locations')
      .getFullList<Location>({ sort: 'name', expand: 'customer' })
  } catch (err: any) {
    error.value = err?.message || 'Failed to load locations'
  } finally {
    loading.value = false
  }
}

function openDetail(loc: Location) {
  router.push(`/staff/locations/${loc.id}`)
}

onMounted(load)
</script>

<template>
  <div class="space-y-4">
    <div class="flex flex-col sm:flex-row justify-between items-start sm:items-center gap-2">
      <h1 class="text-2xl font-bold">Locations</h1>
      <router-link to="/staff/locations/new" class="btn btn-primary btn-sm w-full sm:w-auto">New Location</router-link>
    </div>
    <p class="text-sm text-base-content/60">
      A customer's physical sites. The <span class="font-mono">code</span> is the
      platform Location join key machine intakes resolve against; the address and
      access notes travel to the technician on a visit.
    </p>

    <div v-if="error" class="alert alert-error py-2 text-sm">{{ error }}</div>

    <input v-model="search" type="search" placeholder="Filter by name, customer, code, address…" class="input input-bordered input-sm w-full sm:w-72" />

    <div v-if="loading" class="flex justify-center p-12"><span class="loading loading-spinner loading-lg"></span></div>

    <ResponsiveList v-else :items="filtered" :columns="columns" clickable @row-click="openDetail">
      <template #cell-name="{ value }"><span class="font-medium text-sm">{{ value }}</span></template>
      <template #cell-code="{ value }"><span class="font-mono text-xs">{{ value || '—' }}</span></template>
      <template #cell-contact="{ item }">
        <span class="text-sm">
          {{ item.contact || '—' }}<span v-if="item.contact_phone" class="text-base-content/50"> · {{ item.contact_phone }}</span>
        </span>
      </template>
      <template #empty>
        <span class="text-base-content/60">No locations{{ search ? ' match.' : ' yet.' }}</span>
      </template>
    </ResponsiveList>
  </div>
</template>
