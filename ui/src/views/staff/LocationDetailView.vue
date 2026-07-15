<script setup lang="ts">
// Location detail / edit view. Handles both create (/staff/locations/new) and
// edit (/staff/locations/:id). Any staff can create/edit (migration
// 1813000000); delete stays admin. The LocationPicker sets lat/lng, which also
// power a maps "Navigate" deep link — coordinates preferred, the free-text
// address as fallback so a site with neither still degrades gracefully.
import { computed, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { pb } from '@/pb'
import { useAuthStore } from '@/stores/auth'
import type { Customer, Location, Ticket } from '@/types'
import SearchSelect from '@/components/SearchSelect.vue'
import LocationPicker from '@/components/LocationPicker.vue'
import TicketBadges from '@/components/TicketBadges.vue'

const route = useRoute()
const router = useRouter()
const auth = useAuthStore()

const id = computed(() => route.params.id as string | undefined)
const isEdit = computed(() => !!id.value)

const loading = ref(true)
const saving = ref(false)
const error = ref('')
const customer = ref<Customer | null>(null)
const customers = ref<Customer[]>([])
const tickets = ref<Ticket[]>([])

const form = ref({
  customer: '',
  code: '',
  name: '',
  address: '',
  contact: '',
  contact_phone: '',
  notes: '',
  lat: 0,
  lng: 0,
})

const customerOptions = computed(() => customers.value.map((c) => ({ id: c.id, label: c.name })))

// Prefer coordinates; fall back to the free-text address. Empty when neither
// is set, which hides the Navigate control.
const navigateUrl = computed(() => {
  if (form.value.lat || form.value.lng) {
    return `https://www.google.com/maps/search/?api=1&query=${form.value.lat},${form.value.lng}`
  }
  if (form.value.address.trim()) {
    return `https://www.google.com/maps/search/?api=1&query=${encodeURIComponent(form.value.address.trim())}`
  }
  return ''
})

async function load() {
  loading.value = true
  error.value = ''
  try {
    customers.value = await pb.collection('customers').getFullList<Customer>({ sort: 'name' })
    if (isEdit.value) {
      const loc = await pb.collection('locations').getOne<Location>(id.value!, { expand: 'customer' })
      form.value = {
        customer: loc.customer,
        code: loc.code || '',
        name: loc.name || '',
        address: loc.address || '',
        contact: loc.contact || '',
        contact_phone: loc.contact_phone || '',
        notes: loc.notes || '',
        lat: loc.lat || 0,
        lng: loc.lng || 0,
      }
      customer.value = (loc.expand?.customer as Customer) || null
      tickets.value = (
        await pb.collection('tickets').getList<Ticket>(1, 10, {
          filter: `location = '${id.value}'`,
          sort: '-created',
        })
      ).items
    }
  } catch (err: any) {
    error.value = err?.message || 'Failed to load location'
  } finally {
    loading.value = false
  }
}

async function save() {
  if (!form.value.customer || !form.value.name.trim()) return
  saving.value = true
  error.value = ''
  const data = {
    customer: form.value.customer,
    code: form.value.code.trim(),
    name: form.value.name.trim(),
    address: form.value.address.trim(),
    contact: form.value.contact.trim(),
    contact_phone: form.value.contact_phone.trim(),
    notes: form.value.notes.trim(),
    lat: form.value.lat || 0,
    lng: form.value.lng || 0,
  }
  try {
    if (isEdit.value) {
      await pb.collection('locations').update(id.value!, data)
    } else {
      const created = await pb.collection('locations').create<Location>(data)
      router.replace(`/staff/locations/${created.id}`)
      return
    }
  } catch (err: any) {
    error.value = err?.data?.message || err?.message || 'Failed to save (code must be unique per customer)'
  } finally {
    saving.value = false
  }
}

async function remove() {
  if (!isEdit.value) return
  if (!confirm(`Delete “${form.value.name}”? Tickets and projects referencing it keep the label until re-pointed.`)) return
  error.value = ''
  try {
    await pb.collection('locations').delete(id.value!)
    router.push('/staff/locations')
  } catch (err: any) {
    error.value = err?.data?.message || err?.message || 'Failed to delete'
  }
}

onMounted(load)
// The create flow router.replace()s from /new to /:id, which reuses this
// component instance (onMounted won't refire) — reload so the freshly created
// record's customer expand and tickets populate.
watch(() => route.params.id, load)
</script>

<template>
  <div v-if="loading" class="flex justify-center p-12"><span class="loading loading-spinner loading-lg"></span></div>

  <div v-else class="space-y-4">
    <div class="breadcrumbs text-sm">
      <ul>
        <li><a @click="router.push('/staff/locations')">Locations</a></li>
        <li>{{ isEdit ? form.name || 'Location' : 'New location' }}</li>
      </ul>
    </div>

    <div class="flex items-center justify-between gap-2 flex-wrap">
      <h1 class="text-2xl font-bold">{{ isEdit ? form.name || 'Location' : 'New location' }}</h1>
      <a
        v-if="navigateUrl"
        :href="navigateUrl"
        target="_blank"
        rel="noopener"
        class="btn btn-ghost btn-sm gap-1"
      >📍 Navigate</a>
    </div>

    <div v-if="error" class="alert alert-error py-2 text-sm">{{ error }}</div>

    <div class="grid grid-cols-1 lg:grid-cols-2 gap-4 items-start">
      <!-- Details -->
      <div class="card bg-base-100 shadow-sm">
        <div class="card-body space-y-3">
          <h2 class="card-title text-base">Details</h2>
          <div class="form-control">
            <label class="label py-1"><span class="label-text">Customer *</span></label>
            <SearchSelect
              v-if="!isEdit"
              v-model="form.customer"
              :options="customerOptions"
              size="sm"
              placeholder="Customer…"
              :disabled="saving"
            />
            <input v-else type="text" class="input input-bordered input-sm" :value="customer?.name || '—'" disabled />
          </div>
          <div class="flex gap-2">
            <div class="form-control flex-1">
              <label class="label py-1"><span class="label-text">Name *</span></label>
              <input v-model="form.name" type="text" placeholder="HQ – Bldg C" class="input input-bordered input-sm" :disabled="saving" />
            </div>
            <div class="form-control w-32">
              <label class="label py-1"><span class="label-text">Code</span></label>
              <input v-model="form.code" type="text" placeholder="BLDG-C" class="input input-bordered input-sm font-mono" :disabled="saving" />
            </div>
          </div>
          <div class="form-control">
            <label class="label py-1"><span class="label-text">Address</span></label>
            <input v-model="form.address" type="text" placeholder="123 Main St, City" class="input input-bordered input-sm" :disabled="saving" />
          </div>
          <div class="flex gap-2">
            <div class="form-control flex-1">
              <label class="label py-1"><span class="label-text">Contact</span></label>
              <input v-model="form.contact" type="text" class="input input-bordered input-sm" :disabled="saving" />
            </div>
            <div class="form-control flex-1">
              <label class="label py-1"><span class="label-text">Phone</span></label>
              <input v-model="form.contact_phone" type="tel" class="input input-bordered input-sm" :disabled="saving" />
            </div>
          </div>
          <div class="form-control">
            <label class="label py-1"><span class="label-text">Access notes</span></label>
            <textarea v-model="form.notes" rows="2" placeholder="Gate code, parking, dock hours…" class="textarea textarea-bordered textarea-sm" :disabled="saving"></textarea>
          </div>
          <div class="flex justify-between items-center pt-1">
            <button v-if="isEdit && auth.isAdmin" class="btn btn-ghost btn-sm text-error" @click="remove">Delete</button>
            <span v-else></span>
            <button class="btn btn-primary btn-sm" :disabled="saving || !form.customer || !form.name.trim()" @click="save">
              <span v-if="saving" class="loading loading-spinner loading-xs"></span>
              {{ isEdit ? 'Save' : 'Create' }}
            </button>
          </div>
        </div>
      </div>

      <!-- Map + coordinates -->
      <div class="space-y-4">
        <div class="card bg-base-100 shadow-sm">
          <div class="card-body space-y-2">
            <h2 class="card-title text-base">Location on map</h2>
            <LocationPicker v-model:lat="form.lat" v-model:lng="form.lng" />
            <div class="flex gap-2">
              <input v-model.number="form.lat" type="number" step="any" placeholder="Latitude" class="input input-bordered input-sm font-mono flex-1" :disabled="saving" />
              <input v-model.number="form.lng" type="number" step="any" placeholder="Longitude" class="input input-bordered input-sm font-mono flex-1" :disabled="saving" />
            </div>
          </div>
        </div>

        <!-- Tickets at this site -->
        <div v-if="isEdit" class="card bg-base-100 shadow-sm">
          <div class="card-body">
            <h2 class="card-title text-base">Recent tickets here</h2>
            <div class="divide-y divide-base-200">
              <router-link
                v-for="t in tickets"
                :key="t.id"
                :to="`/staff/tickets/${t.id}`"
                class="flex items-center gap-3 py-2 hover:bg-base-200/50 -mx-2 px-2 rounded"
              >
                <span class="font-mono text-xs text-base-content/50 w-10">#{{ t.number }}</span>
                <span class="flex-1 truncate">{{ t.title }}</span>
                <TicketBadges :status="t.status" :priority="t.priority" />
              </router-link>
              <p v-if="tickets.length === 0" class="py-3 text-sm text-base-content/50">No tickets at this location yet.</p>
            </div>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>
