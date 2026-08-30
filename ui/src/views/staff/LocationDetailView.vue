<script setup lang="ts">
// Location detail / edit view. Handles both create (/staff/locations/new) and
// edit (/staff/locations/:id). Any staff can create/edit (migration
// 1813000000); delete stays admin. The LocationPicker sets lat/lng, which also
// power a maps "Navigate" deep link — coordinates preferred, the free-text
// address as fallback so a location with neither still degrades gracefully.
//
// LAYOUT: the record first (Details beside the map — two cards of about equal
// height), then Metadata full width because its height is whatever the type's
// schema says, then the two read-only cards as their own pair. Everything below
// the map used to be stacked in the right-hand column, which meant one column
// carried four cards against the other's one and the page ran as a single tall
// stack with dead space beside it.
import { computed, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { pb } from '@/pb'
import { useAuthStore } from '@/stores/auth'
import type { Customer, Location, RecordType, Thing, Ticket } from '@/types'
import SearchSelect from '@/components/SearchSelect.vue'
import LocationPicker from '@/components/LocationPicker.vue'
import MetadataEditor from '@/components/MetadataEditor.vue'
import TicketBadges from '@/components/TicketBadges.vue'
import QrLabelModal from '@/components/QrLabelModal.vue'

const route = useRoute()
const router = useRouter()
const auth = useAuthStore()

const id = computed(() => route.params.id as string | undefined)
const isEdit = computed(() => !!id.value)
// View/edit toggle: create opens unlocked, an existing record opens locked.
const editing = ref(false)
const labelOpen = ref(false)

const loading = ref(true)
const saving = ref(false)
const error = ref('')
const record = ref<Location | null>(null)
const customer = ref<Customer | null>(null)
const customers = ref<Customer[]>([])
const tickets = ref<Ticket[]>([])
// Total behind the page of ten, so the card can offer "View all" only when
// there is actually more to see.
const ticketTotal = ref(0)
// The gear installed here. ThingDetailView has answered "every ticket for this
// thing" since the relation existed; this is the other direction, and it is the
// one a tech standing on site asks first — scan the door, see what is on it.
const things = ref<Thing[]>([])
const types = ref<RecordType[]>([])
// Same-customer locations, for the parent picker and the ancestor path.
const siblings = ref<Location[]>([])

// The JSON tab holds a text buffer parsed only on blur, so a save fired while it
// is focused would silently drop the edit. commit() flushes it, or refuses.
const metadataEditor = ref<InstanceType<typeof MetadataEditor> | null>(null)

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
  type: '',
  parent: '',
  metadata: null as Record<string, any> | null,
})

const customerOptions = computed(() => customers.value.map((c) => ({ id: c.id, label: c.name })))
const typeOptions = computed(() =>
  types.value.map((t) => ({ id: t.id, label: t.name, sublabel: t.code || undefined })),
)

// The parent picker offers same-customer locations, minus this record and anything
// beneath it. Excluding only self (which is all the platform console does) still
// admits A→B→A; walking down from here and dropping the whole subtree is the
// same few lines and actually closes it. PocketBase has no server-side cycle
// detection, so this picker is the only guard there is.
const parentOptions = computed(() => {
  const excluded = new Set<string>()
  if (id.value) {
    excluded.add(id.value)
    // Breadth-first down the tree. The visited set is not optional: without it a
    // cycle already in the data would spin here forever.
    let frontier = [id.value]
    while (frontier.length) {
      const next: string[] = []
      for (const parentId of frontier) {
        for (const l of siblings.value) {
          if (l.parent === parentId && !excluded.has(l.id)) {
            excluded.add(l.id)
            next.push(l.id)
          }
        }
      }
      frontier = next
    }
  }
  return siblings.value
    .filter((l) => !excluded.has(l.id))
    .map((l) => ({ id: l.id, label: l.name, sublabel: l.code || undefined }))
})

// Read-only breadcrumb of the containing locations, nearest last. Capped and
// visited-guarded for the same reason as above — a cycle must degrade to a short
// path, never to a hung tab.
const ancestorPath = computed(() => {
  if (!form.value.parent) return ''
  const byId = new Map(siblings.value.map((l) => [l.id, l]))
  const names: string[] = []
  const seen = new Set<string>(id.value ? [id.value] : [])
  let cursor: string | undefined = form.value.parent
  while (cursor && !seen.has(cursor) && names.length < 8) {
    seen.add(cursor)
    const node = byId.get(cursor)
    if (!node) break
    names.unshift(node.name)
    cursor = node.parent
  }
  return names.length ? `${names.join(' → ')} → ${form.value.name || 'this location'}` : ''
})

// The schema driving the metadata form comes from the selected type; read from
// the loaded list so switching the picker re-types the form before any save.
const activeSchema = computed(() => {
  if (!form.value.type) return null
  return types.value.find((t) => t.id === form.value.type)?.metadata_schema || null
})

// A locked record with no metadata hides the card entirely rather than spending
// one on "No metadata recorded" — most locations never grow a field, and this
// page is read to find out what is installed here.
const metadataEmpty = computed(() => {
  const m = form.value.metadata
  return !m || Object.keys(m).length === 0
})

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

function applyRecord(loc: Location) {
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
    type: loc.type || '',
    parent: loc.parent || '',
    metadata: (loc.metadata as Record<string, any> | null) ?? null,
  }
}

// Types and candidate parents are both customer-scoped.
async function loadScoped(customerId: string) {
  types.value = []
  siblings.value = []
  if (!customerId) return
  try {
    types.value = await pb.collection('location_types').getFullList<RecordType>({
      filter: `customer = '${customerId}'`,
      sort: 'name',
    })
    siblings.value = await pb.collection('locations').getFullList<Location>({
      filter: `customer = '${customerId}'`,
      sort: 'name',
    })
  } catch {
    // Both are optional; the pickers just stay empty.
  }
}

// Create path only: the customer bounds both the type and the parent picker.
function onCustomerChange(value: string) {
  form.value.type = ''
  form.value.parent = ''
  loadScoped(value)
}

function startEdit() {
  editing.value = true
}

function cancelEdit() {
  if (!isEdit.value) {
    router.push('/staff/locations')
    return
  }
  if (record.value) applyRecord(record.value)
  editing.value = false
}

async function load() {
  loading.value = true
  error.value = ''
  try {
    customers.value = await pb.collection('customers').getFullList<Customer>({ sort: 'name' })
    if (isEdit.value) {
      const loc = await pb.collection('locations').getOne<Location>(id.value!, { expand: 'customer' })
      record.value = loc
      applyRecord(loc)
      editing.value = false
      customer.value = (loc.expand?.customer as Customer) || null
      await loadScoped(loc.customer)
      const ticketPage = await pb.collection('tickets').getList<Ticket>(1, 10, {
        filter: `location = '${id.value}'`,
        sort: '-created',
      })
      tickets.value = ticketPage.items
      ticketTotal.value = ticketPage.totalItems
      // Retired gear included deliberately, same reasoning as the scanner: you
      // cannot file against it, but "what used to be on this door" is a question
      // a tech asks on site. The row carries its own badge.
      things.value = await pb.collection('things').getFullList<Thing>({
        filter: `location = '${id.value}'`,
        sort: 'retired,name',
        expand: 'type',
      })
    } else {
      editing.value = true
    }
  } catch (err: any) {
    error.value = err?.message || 'Failed to load location'
  } finally {
    loading.value = false
  }
}

async function save() {
  if (!form.value.customer || !form.value.name.trim()) return
  // Flush a JSON tab left mid-edit; refuses the save if it doesn't parse.
  if (metadataEditor.value && !metadataEditor.value.commit()) return
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
    type: form.value.type,
    parent: form.value.parent,
    metadata: form.value.metadata,
  }
  try {
    if (isEdit.value) {
      record.value = await pb.collection('locations').update<Location>(id.value!, data, { expand: 'customer' })
      customer.value = (record.value.expand?.customer as Customer) || customer.value
      editing.value = false
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
        <li><router-link to="/staff/locations">Locations</router-link></li>
        <li>{{ isEdit ? form.name || 'Location' : 'New location' }}</li>
      </ul>
    </div>

    <div class="flex items-center justify-between gap-2 flex-wrap">
      <h1 class="text-2xl font-bold">{{ isEdit ? form.name || 'Location' : 'New location' }}</h1>
      <div class="flex gap-2 items-center">
        <a
          v-if="navigateUrl"
          :href="navigateUrl"
          target="_blank"
          rel="noopener"
          class="btn btn-ghost btn-sm gap-1"
        >📍 Navigate</a>
        <template v-if="editing">
          <button class="btn btn-ghost btn-sm" :disabled="!editing || saving" @click="cancelEdit">Cancel</button>
          <button class="btn btn-primary btn-sm" :disabled="saving || !form.customer || !form.name.trim()" @click="save">
            <span v-if="saving" class="loading loading-spinner loading-xs"></span>
            {{ isEdit ? 'Save' : 'Create' }}
          </button>
        </template>
        <template v-else>
          <!-- No code, no label: the QR payload IS the code (ADR 0002). -->
          <button v-if="isEdit && form.code" class="btn btn-ghost btn-sm" @click="labelOpen = true">Label</button>
          <button class="btn btn-primary btn-sm" @click="startEdit">Edit</button>
        </template>
      </div>
    </div>

    <QrLabelModal
      v-if="labelOpen"
      :code="form.code"
      :name="form.name"
      kind="location"
      :customer-name="customer?.name"
      @close="labelOpen = false"
    />

    <div v-if="error" class="alert alert-error py-2 text-sm">{{ error }}</div>

    <div class="grid grid-cols-1 lg:grid-cols-2 gap-4 items-start">
      <!-- Left column: the record's own fields. Metadata sits under Details
           rather than in a full-width row of its own — it is type-defined data
           about the record, next of kin to the Type field a few lines above it,
           and a schema with three properties stretched across the page was a
           short card wearing a long one's clothes. Whichever column ends first
           simply ends; that is ordinary in a two-column grid, and nothing like
           the four-cards-against-one this page used to run. -->
      <div class="space-y-4">
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
                :disabled="!editing || saving"
                @update:model-value="onCustomerChange"
              />
              <input v-else type="text" class="input input-bordered input-sm" :value="customer?.name || '—'" disabled />
            </div>
            <div class="flex gap-2">
              <div class="form-control flex-1">
                <label class="label py-1"><span class="label-text">Name *</span></label>
                <input v-model="form.name" type="text" placeholder="HQ – Bldg C" class="input input-bordered input-sm" :disabled="!editing || saving" />
              </div>
              <div class="form-control w-32">
                <label class="label py-1"><span class="label-text">Code</span></label>
                <input v-model="form.code" type="text" placeholder="BLDG-C" class="input input-bordered input-sm font-mono" :disabled="!editing || saving" />
              </div>
            </div>
            <div class="flex gap-2">
              <div class="form-control flex-1 min-w-0">
                <label class="label py-1">
                  <span class="label-text">Type</span>
                  <router-link v-if="editing" to="/staff/location-types" class="label-text-alt link link-hover">manage →</router-link>
                </label>
                <SearchSelect
                  v-model="form.type"
                  :options="typeOptions"
                  size="sm"
                  empty-label="None"
                  placeholder="Pick a type…"
                  :disabled="!editing || saving || !form.customer"
                />
              </div>
              <div class="form-control flex-1 min-w-0">
                <label class="label py-1"><span class="label-text">Parent</span></label>
                <SearchSelect
                  v-model="form.parent"
                  :options="parentOptions"
                  size="sm"
                  empty-label="None"
                  placeholder="Contained by…"
                  :disabled="!editing || saving || !form.customer"
                />
              </div>
            </div>
            <p v-if="ancestorPath" class="text-xs text-base-content/50 -mt-1">{{ ancestorPath }}</p>
            <div class="flex gap-2">
              <div class="form-control flex-1">
                <label class="label py-1"><span class="label-text">Contact</span></label>
                <input v-model="form.contact" type="text" class="input input-bordered input-sm" :disabled="!editing || saving" />
              </div>
              <div class="form-control flex-1">
                <label class="label py-1"><span class="label-text">Phone</span></label>
                <input v-model="form.contact_phone" type="tel" class="input input-bordered input-sm" :disabled="!editing || saving" />
              </div>
            </div>
            <div class="form-control">
              <label class="label py-1"><span class="label-text">Access notes</span></label>
              <textarea v-model="form.notes" rows="2" placeholder="Gate code, parking, dock hours…" class="textarea textarea-bordered textarea-sm" :disabled="!editing || saving"></textarea>
            </div>
            <div v-if="isEdit && auth.isAdmin && editing" class="pt-1">
              <button class="btn btn-ghost btn-sm text-error" @click="remove">Delete</button>
            </div>
          </div>
        </div>

        <!-- Metadata. Hidden entirely in read mode when there is nothing in it:
             a locked record used to spend a whole card saying "No metadata
             recorded" — true, and not worth the room on a page whose job is to
             answer what is installed here. Editing always shows it, because that
             is when you would be adding the first field. -->
        <div v-if="editing || !metadataEmpty" class="card bg-base-100 shadow-sm">
          <div class="card-body">
            <h2 class="card-title text-base">Metadata</h2>
            <p class="text-xs text-base-content/60 mb-2">
              <template v-if="activeSchema">Fields defined by this location's type.</template>
              <template v-else>
                Free-form fields. Give the type a metadata schema to get typed inputs instead.
              </template>
            </p>
            <MetadataEditor
              ref="metadataEditor"
              v-model="form.metadata"
              :schema="activeSchema"
              :disabled="!editing || saving"
            />
          </div>
        </div>
      </div>

      <!-- Map + coordinates. Half the width, beside the record's own fields:
           identity, contact and type-defined data on the left, where the place
           physically is on the right. -->
      <div class="card bg-base-100 shadow-sm">
        <div class="card-body space-y-2">
          <h2 class="card-title text-base">Location &amp; map</h2>
          <LocationPicker v-model:lat="form.lat" v-model:lng="form.lng" v-model:address="form.address" :disabled="!editing || saving" />
          <div class="flex gap-2">
            <input v-model.number="form.lat" type="number" step="any" placeholder="Latitude" class="input input-bordered input-sm font-mono flex-1" :disabled="!editing || saving" />
            <input v-model.number="form.lng" type="number" step="any" placeholder="Longitude" class="input input-bordered input-sm font-mono flex-1" :disabled="!editing || saving" />
          </div>
        </div>
      </div>
    </div>

    <!-- What is here, and what has happened here. These two used to be stacked
         under the map in the right-hand column, which left that column carrying
         four cards against the left's one — the record's own fields ran out
         after a screen and the rest of the page was a single tall stack with a
         column of dead space beside it. They are a pair (what is installed /
         what went wrong), they are read rather than edited, and they belong
         below the record rather than beside half of it. -->
    <div v-if="isEdit" class="grid grid-cols-1 lg:grid-cols-2 gap-4 items-start">
      <!-- Things installed here -->
      <div class="card bg-base-100 shadow-sm">
        <div class="card-body">
          <div class="flex items-center justify-between gap-2">
            <h2 class="card-title text-base">Things here</h2>
            <!-- The list is complete (getFullList), so this is a count and not
                 a "see more" link — a location holds a handful of things, not a
                 roster's worth. -->
            <span v-if="things.length" class="text-xs text-base-content/50">{{ things.length }}</span>
          </div>
          <div class="divide-y divide-base-200">
            <router-link
              v-for="t in things"
              :key="t.id"
              :to="`/staff/things/${t.id}`"
              class="flex items-center gap-3 py-2 hover:bg-base-200/50 -mx-2 px-2 rounded"
            >
              <span class="font-mono text-xs text-base-content/50 w-20 truncate">{{ t.code || '—' }}</span>
              <span class="flex-1 truncate">{{ t.name }}</span>
              <span v-if="t.expand?.type?.name" class="badge badge-sm badge-soft hidden sm:inline-flex">
                {{ t.expand.type.name }}
              </span>
              <span v-if="t.retired" class="badge badge-sm badge-soft">Retired</span>
            </router-link>
            <p v-if="things.length === 0" class="py-3 text-sm text-base-content/50">
              No things are filed at this location yet.
            </p>
          </div>
        </div>
      </div>

      <!-- Tickets at this location -->
      <div class="card bg-base-100 shadow-sm">
        <div class="card-body">
          <div class="flex items-center justify-between gap-2">
            <h2 class="card-title text-base">Recent tickets here</h2>
            <!-- Unlike Things above, this list IS a page of a longer one, so it
                 gets the "View all →" the thing detail already uses. The queue's
                 location picker is not gated on the customer filter (only its
                 thing picker is), so the id alone lands with a live control. -->
            <router-link
              v-if="ticketTotal > tickets.length"
              :to="`/staff/tickets?location=${id}`"
              class="text-xs link link-hover"
            >View all {{ ticketTotal }} →</router-link>
          </div>
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
</template>
