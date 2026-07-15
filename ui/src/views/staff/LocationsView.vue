<script setup lang="ts">
// Locations: a customer's physical places. `code` is the platform Location
// join key that machine intakes resolve against (see docs/protocol.md);
// address + access notes make on-site visits better. In the Directory now, and
// any staff member can create/edit (migration 1813000000) — only delete stays
// admin-only, the one destructive op against a location referenced by
// tickets/projects/visits.
//
// Reads as a roster (read-only rows, edit via a panel above the list) to match
// the other staff list views — the shared ResponsiveList gives the same dense
// desktop table + stacked mobile cards for free.
import { computed, nextTick, onMounted, ref } from 'vue'
import { pb } from '@/pb'
import { useAuthStore } from '@/stores/auth'
import type { Customer, Location } from '@/types'
import SearchSelect from '@/components/SearchSelect.vue'
import ResponsiveList, { type Column } from '@/components/ResponsiveList.vue'

const auth = useAuthStore()

const columns: Column<Location>[] = [
  { key: 'name', label: 'Name' },
  { key: 'expand.customer.name', label: 'Customer' },
  { key: 'code', label: 'Code' },
  { key: 'address', label: 'Address' },
  { key: 'contact', label: 'Contact' },
  { key: 'notes', label: 'Notes' },
]

const locations = ref<Location[]>([])
const customers = ref<Customer[]>([])
const loading = ref(true)
const error = ref('')
const saving = ref(false)

// New-location form.
const nu = ref({ customer: '', name: '', code: '', address: '' })
const creating = ref(false)

// Inline row editing (admin): full record, in a panel above the list.
const editing = ref<Location | null>(null)
const editForm = ref({ name: '', code: '', address: '', contact: '', contact_phone: '', notes: '' })

const customerOptions = computed(() => customers.value.map((c) => ({ id: c.id, label: c.name })))

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

// The edit panel renders above the list, which can be off-screen when the
// triggering row is below the fold — bring it into view.
const editCard = ref<HTMLElement | null>(null)
function startEdit(loc: Location) {
  editing.value = loc
  editForm.value = {
    name: loc.name || '',
    code: loc.code || '',
    address: loc.address || '',
    contact: loc.contact || '',
    contact_phone: loc.contact_phone || '',
    notes: loc.notes || '',
  }
  nextTick(() => editCard.value?.scrollIntoView({ behavior: 'smooth', block: 'nearest' }))
}

async function saveEdit() {
  if (!editing.value) return
  saving.value = true
  error.value = ''
  try {
    await pb.collection('locations').update(editing.value.id, {
      name: editForm.value.name.trim(),
      code: editForm.value.code.trim(),
      address: editForm.value.address.trim(),
      contact: editForm.value.contact.trim(),
      contact_phone: editForm.value.contact_phone.trim(),
      notes: editForm.value.notes.trim(),
    })
    editing.value = null
    await load()
  } catch (err: any) {
    error.value = err?.message || 'Failed to save (code must be unique per customer)'
  } finally {
    saving.value = false
  }
}

async function remove(loc: Location) {
  if (!confirm(`Delete “${loc.name}”? Tickets and projects referencing it keep the label until re-pointed.`)) return
  error.value = ''
  try {
    await pb.collection('locations').delete(loc.id)
    if (editing.value?.id === loc.id) editing.value = null
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

    <!-- Edit panel: lives above the list (an inline table row can't render
         inside the mobile card layout). -->
    <div v-if="editing" ref="editCard" class="card bg-base-100 shadow-sm">
      <div class="card-body p-4 space-y-2">
        <h2 class="card-title text-sm">Edit {{ editing.name }}</h2>
        <form class="flex flex-col sm:flex-row sm:flex-wrap gap-2 items-stretch sm:items-end" @submit.prevent="saveEdit">
          <div class="form-control">
            <label class="label py-1"><span class="label-text text-xs">Name *</span></label>
            <input v-model="editForm.name" type="text" class="input input-bordered input-sm w-full sm:w-48" :disabled="saving" />
          </div>
          <div class="form-control">
            <label class="label py-1"><span class="label-text text-xs">Code</span></label>
            <input v-model="editForm.code" type="text" class="input input-bordered input-sm w-full sm:w-28 font-mono" :disabled="saving" />
          </div>
          <div class="form-control">
            <label class="label py-1"><span class="label-text text-xs">Address</span></label>
            <input v-model="editForm.address" type="text" class="input input-bordered input-sm w-full sm:w-56" :disabled="saving" />
          </div>
          <div class="form-control">
            <label class="label py-1"><span class="label-text text-xs">Contact</span></label>
            <input v-model="editForm.contact" type="text" class="input input-bordered input-sm w-full sm:w-40" :disabled="saving" />
          </div>
          <div class="form-control">
            <label class="label py-1"><span class="label-text text-xs">Phone</span></label>
            <input v-model="editForm.contact_phone" type="text" class="input input-bordered input-sm w-full sm:w-32" :disabled="saving" />
          </div>
          <div class="form-control flex-1 min-w-[12rem]">
            <label class="label py-1"><span class="label-text text-xs">Notes</span></label>
            <input v-model="editForm.notes" type="text" placeholder="Gate code, parking…" class="input input-bordered input-sm w-full" :disabled="saving" />
          </div>
          <div class="flex gap-2">
            <button type="submit" class="btn btn-primary btn-sm" :disabled="saving || !editForm.name.trim()">
              <span v-if="saving" class="loading loading-spinner loading-xs"></span>
              Save
            </button>
            <button type="button" class="btn btn-ghost btn-sm" :disabled="saving" @click="editing = null">Cancel</button>
          </div>
        </form>
      </div>
    </div>

    <div v-if="loading" class="flex justify-center p-12"><span class="loading loading-spinner loading-lg"></span></div>

    <ResponsiveList v-else :items="locations" :columns="columns" :clickable="false">
      <template #cell-name="{ value }"><span class="font-medium text-sm">{{ value }}</span></template>
      <template #cell-code="{ value }"><span class="font-mono text-xs">{{ value || '—' }}</span></template>
      <template #cell-contact="{ item }">
        <span class="text-sm">
          {{ item.contact || '—' }}<span v-if="item.contact_phone" class="text-base-content/50"> · {{ item.contact_phone }}</span>
        </span>
      </template>
      <template #actions="{ item }">
        <button class="btn btn-ghost btn-xs" @click="editing?.id === item.id ? (editing = null) : startEdit(item)">
          {{ editing?.id === item.id ? 'Cancel' : 'Edit' }}
        </button>
        <button v-if="auth.isAdmin" class="btn btn-ghost btn-xs text-error" @click="remove(item)">Delete</button>
      </template>
      <template #empty>
        <span class="text-base-content/60">No locations yet.</span>
      </template>
    </ResponsiveList>
  </div>
</template>
