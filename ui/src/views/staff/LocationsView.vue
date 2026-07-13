<script setup lang="ts">
// Admin-managed locations: a customer's physical places. `code` is the
// platform Location join key that machine intakes resolve against (see
// docs/protocol.md); address + access notes make on-site visits better.
// Staff can quick-create a location inline from the ticket form; this roster
// is where admins curate codes, addresses, and contacts. Reached only by
// admins (route meta.adminOnly + collection update/delete rules).
import { computed, onMounted, ref } from 'vue'
import { pb } from '@/pb'
import type { Customer, Location } from '@/types'
import SearchSelect from '@/components/SearchSelect.vue'

const locations = ref<Location[]>([])
const customers = ref<Customer[]>([])
const loading = ref(true)
const error = ref('')
const savingId = ref('')

// New-location form.
const nu = ref({ customer: '', name: '', code: '', address: '' })
const creating = ref(false)

const customerOptions = computed(() => customers.value.map((c) => ({ id: c.id, label: c.name })))

function customerName(loc: Location): string {
  return loc.expand?.customer?.name || '—'
}

async function load() {
  loading.value = true
  error.value = ''
  try {
    customers.value = await pb.collection('customers').getFullList<Customer>({ sort: 'name' })
    locations.value = await pb
      .collection('locations')
      .getFullList<Location>({ sort: 'name', expand: 'customer' })
  } catch (err: any) {
    error.value = err?.message || 'Failed to load locations'
  } finally {
    loading.value = false
  }
}

async function create() {
  const name = nu.value.name.trim()
  if (!nu.value.customer || !name) return
  creating.value = true
  error.value = ''
  try {
    await pb.collection('locations').create({
      customer: nu.value.customer,
      name,
      code: nu.value.code.trim(),
      address: nu.value.address.trim(),
    })
    nu.value = { customer: '', name: '', code: '', address: '' }
    await load()
  } catch (err: any) {
    error.value = err?.message || 'Failed to create location (code must be unique per customer)'
  } finally {
    creating.value = false
  }
}

async function save(loc: Location) {
  savingId.value = loc.id
  error.value = ''
  try {
    await pb.collection('locations').update(loc.id, {
      name: loc.name.trim(),
      code: (loc.code || '').trim(),
      address: (loc.address || '').trim(),
      contact: (loc.contact || '').trim(),
      contact_phone: (loc.contact_phone || '').trim(),
      notes: (loc.notes || '').trim(),
    })
    await load()
  } catch (err: any) {
    error.value = err?.message || 'Failed to save (code must be unique per customer)'
  } finally {
    savingId.value = ''
  }
}

async function remove(loc: Location) {
  if (!confirm(`Delete “${loc.name}”? Tickets and projects referencing it keep the label until re-pointed.`)) return
  error.value = ''
  try {
    await pb.collection('locations').delete(loc.id)
    await load()
  } catch (err: any) {
    error.value = err?.message || 'Failed to delete'
  }
}

onMounted(load)
</script>

<template>
  <div class="space-y-4">
    <h1 class="text-2xl font-bold">Locations</h1>
    <p class="text-sm text-base-content/60">
      A customer's physical sites. The <span class="font-mono">code</span> is the
      platform Location join key machine intakes resolve against; the address and
      access notes travel to the technician on a visit.
    </p>

    <div v-if="error" class="alert alert-error py-2 text-sm">{{ error }}</div>

    <!-- New location -->
    <form class="flex flex-col sm:flex-row flex-wrap gap-2 sm:items-end" @submit.prevent="create">
      <div class="form-control">
        <label class="label py-1"><span class="label-text text-xs">Customer *</span></label>
        <div class="w-full sm:w-56">
          <SearchSelect v-model="nu.customer" :options="customerOptions" size="sm" placeholder="Customer…" :disabled="creating" />
        </div>
      </div>
      <div class="form-control">
        <label class="label py-1"><span class="label-text text-xs">Name *</span></label>
        <input v-model="nu.name" type="text" placeholder="e.g. HQ – Bldg C" class="input input-bordered input-sm w-full sm:w-48" :disabled="creating" />
      </div>
      <div class="form-control">
        <label class="label py-1"><span class="label-text text-xs">Code</span></label>
        <input v-model="nu.code" type="text" placeholder="BLDG-C" class="input input-bordered input-sm w-full sm:w-28 font-mono" :disabled="creating" />
      </div>
      <div class="form-control">
        <label class="label py-1"><span class="label-text text-xs">Address</span></label>
        <input v-model="nu.address" type="text" placeholder="123 Main St" class="input input-bordered input-sm w-full sm:w-56" :disabled="creating" />
      </div>
      <button type="submit" class="btn btn-primary btn-sm" :disabled="creating || !nu.customer || !nu.name.trim()">Add</button>
    </form>

    <div v-if="loading" class="flex justify-center p-12"><span class="loading loading-spinner loading-lg"></span></div>

    <div v-else class="overflow-x-auto bg-base-100 rounded-lg shadow-sm">
      <table class="table table-sm">
        <thead>
          <tr>
            <th>Customer</th>
            <th>Name</th>
            <th>Code</th>
            <th>Address</th>
            <th>Contact</th>
            <th>Phone</th>
            <th>Notes</th>
            <th></th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="loc in locations" :key="loc.id">
            <td class="whitespace-nowrap text-base-content/70">{{ customerName(loc) }}</td>
            <td><input v-model="loc.name" type="text" class="input input-bordered input-xs w-40" /></td>
            <td><input v-model="loc.code" type="text" class="input input-bordered input-xs w-24 font-mono" /></td>
            <td><input v-model="loc.address" type="text" class="input input-bordered input-xs w-48" /></td>
            <td><input v-model="loc.contact" type="text" class="input input-bordered input-xs w-32" /></td>
            <td><input v-model="loc.contact_phone" type="text" class="input input-bordered input-xs w-28" /></td>
            <td><input v-model="loc.notes" type="text" class="input input-bordered input-xs w-48" placeholder="Gate code, parking…" /></td>
            <td class="text-right whitespace-nowrap">
              <button class="btn btn-ghost btn-xs" :disabled="savingId === loc.id" @click="save(loc)">
                <span v-if="savingId === loc.id" class="loading loading-spinner loading-xs"></span>
                Save
              </button>
              <button class="btn btn-ghost btn-xs text-error" @click="remove(loc)">Delete</button>
            </td>
          </tr>
          <tr v-if="locations.length === 0">
            <td colspan="8" class="text-base-content/50">No locations yet.</td>
          </tr>
        </tbody>
      </table>
    </div>
  </div>
</template>
