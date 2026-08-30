<script setup lang="ts">
// Thing detail / edit view. Handles both create (/staff/things/new) and edit
// (/staff/things/:id), mirroring LocationDetailView's shape: view/edit lock,
// customer pinned after creation, admin-only delete.
//
// The "Tickets for this thing" card is the point of the whole collection —
// promoting the device from free text to a relation exists so this question has
// an answer. The metadata card is typed when the selected type carries a
// metadata_schema and free-form otherwise.
import { computed, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { pb } from '@/pb'
import { useAuthStore } from '@/stores/auth'
import type { Customer, Location, RecordType, Thing, Ticket } from '@/types'
import SearchSelect from '@/components/SearchSelect.vue'
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
const record = ref<Thing | null>(null)
const customer = ref<Customer | null>(null)
const customers = ref<Customer[]>([])
const types = ref<RecordType[]>([])
const locations = ref<Location[]>([])
const tickets = ref<Ticket[]>([])
const ticketTotal = ref(0)
const openTotal = ref(0)

// The JSON tab holds a text buffer that is only parsed on blur, so a save fired
// while it is focused would silently drop the edit. commit() flushes it (or
// refuses), which is why the editor is reached through a ref.
const metadataEditor = ref<InstanceType<typeof MetadataEditor> | null>(null)

const form = ref({
  customer: '',
  code: '',
  name: '',
  type: '',
  location: '',
  notes: '',
  retired: false,
  metadata: null as Record<string, any> | null,
})

const customerOptions = computed(() => customers.value.map((c) => ({ id: c.id, label: c.name })))
const typeOptions = computed(() =>
  types.value.map((t) => ({ id: t.id, label: t.name, sublabel: t.code || undefined })),
)
const locationOptions = computed(() =>
  locations.value.map((l) => ({ id: l.id, label: l.name, sublabel: l.code || l.address || undefined })),
)

// The schema driving the metadata form comes from the selected type. Read from
// the loaded list rather than the record's expand so switching the type picker
// re-types the form immediately, before any save.
const activeSchema = computed(() => {
  if (!form.value.type) return null
  return types.value.find((t) => t.id === form.value.type)?.metadata_schema || null
})

function applyRecord(thing: Thing) {
  form.value = {
    customer: thing.customer,
    code: thing.code || '',
    name: thing.name || '',
    type: thing.type || '',
    location: thing.location || '',
    notes: thing.notes || '',
    retired: !!thing.retired,
    metadata: (thing.metadata as Record<string, any> | null) ?? null,
  }
}

function startEdit() {
  editing.value = true
}

function cancelEdit() {
  if (!isEdit.value) {
    router.push('/staff/things')
    return
  }
  if (record.value) applyRecord(record.value)
  editing.value = false
}

// Types and locations are both customer-scoped, so they reload whenever the
// customer changes on the create path.
async function loadScoped(customerId: string) {
  types.value = []
  locations.value = []
  if (!customerId) return
  try {
    types.value = await pb.collection('thing_types').getFullList<RecordType>({
      filter: `customer = '${customerId}'`,
      sort: 'name',
    })
    locations.value = await pb.collection('locations').getFullList<Location>({
      filter: `customer = '${customerId}'`,
      sort: 'name',
    })
  } catch {
    // Both are optional on a thing; the pickers just stay empty.
  }
}

function onCustomerChange(value: string) {
  form.value.type = ''
  form.value.location = ''
  loadScoped(value)
}

async function loadTickets() {
  if (!id.value) return
  try {
    const page = await pb.collection('tickets').getList<Ticket>(1, 10, {
      filter: `thing = '${id.value}'`,
      sort: '-created',
    })
    tickets.value = page.items
    ticketTotal.value = page.totalItems
    // Second count only — "is this device a repeat offender, and is it broken
    // right now" is the question the card exists to answer.
    openTotal.value = (
      await pb.collection('tickets').getList<Ticket>(1, 1, {
        filter: `thing = '${id.value}' && status != 'resolved' && status != 'closed'`,
      })
    ).totalItems
  } catch {
    // The card degrades to empty; the record itself still renders.
  }
}

async function load() {
  loading.value = true
  error.value = ''
  try {
    customers.value = await pb.collection('customers').getFullList<Customer>({ sort: 'name' })
    if (isEdit.value) {
      const thing = await pb.collection('things').getOne<Thing>(id.value!, { expand: 'customer' })
      record.value = thing
      applyRecord(thing)
      editing.value = false
      customer.value = (thing.expand?.customer as Customer) || null
      await loadScoped(thing.customer)
      await loadTickets()
    } else {
      editing.value = true
    }
  } catch (err: any) {
    error.value = err?.message || 'Failed to load thing'
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
    type: form.value.type,
    location: form.value.location,
    notes: form.value.notes.trim(),
    retired: form.value.retired,
    metadata: form.value.metadata,
  }
  try {
    if (isEdit.value) {
      record.value = await pb.collection('things').update<Thing>(id.value!, data, { expand: 'customer' })
      customer.value = (record.value.expand?.customer as Customer) || customer.value
      editing.value = false
    } else {
      const created = await pb.collection('things').create<Thing>(data)
      router.replace(`/staff/things/${created.id}`)
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
  if (
    !confirm(
      `Delete “${form.value.name}”? Tickets referencing it lose the link — their device note keeps ` +
        `whatever text was there. Retiring instead keeps the history intact.`,
    )
  )
    return
  error.value = ''
  try {
    await pb.collection('things').delete(id.value!)
    router.push('/staff/things')
  } catch (err: any) {
    error.value = err?.data?.message || err?.message || 'Failed to delete'
  }
}

onMounted(load)
// The create flow router.replace()s from /new to /:id, which reuses this
// component instance (onMounted won't refire) — reload so the freshly created
// record's expands and ticket card populate.
watch(() => route.params.id, load)
</script>

<template>
  <div v-if="loading" class="flex justify-center p-12"><span class="loading loading-spinner loading-lg"></span></div>

  <div v-else class="space-y-4">
    <div class="breadcrumbs text-sm">
      <ul>
        <li><a @click="router.push('/staff/things')">Things</a></li>
        <li>{{ isEdit ? form.name || 'Thing' : 'New thing' }}</li>
      </ul>
    </div>

    <div class="flex items-center justify-between gap-2 flex-wrap">
      <h1 class="text-2xl font-bold">{{ isEdit ? form.name || 'Thing' : 'New thing' }}</h1>
      <div class="flex gap-2">
        <template v-if="editing">
          <button class="btn btn-ghost btn-sm" :disabled="saving" @click="cancelEdit">Cancel</button>
          <button class="btn btn-primary btn-sm" :disabled="saving || !form.customer || !form.name.trim()" @click="save">
            <span v-if="saving" class="loading loading-spinner loading-xs"></span>
            Save
          </button>
        </template>
        <template v-else>
          <!-- No code, no label: the QR payload IS the code (ADR 0002). -->
          <button v-if="isEdit && form.code" class="btn btn-ghost btn-sm" @click="labelOpen = true">Label</button>
          <button class="btn btn-sm" @click="startEdit">Edit</button>
        </template>
      </div>
    </div>

    <QrLabelModal
      v-if="labelOpen"
      :code="form.code"
      :name="form.name"
      kind="thing"
      :customer-name="customer?.name"
      @close="labelOpen = false"
    />


    <div v-if="error" class="alert alert-error py-2 text-sm">{{ error }}</div>

    <div class="grid grid-cols-1 lg:grid-cols-2 gap-4 items-start">
      <!-- Details -->
      <div class="card bg-base-100 shadow-sm">
        <div class="card-body p-4 space-y-3">
          <h2 class="card-title text-base">Details</h2>

          <div class="form-control">
            <label class="label py-1"><span class="label-text text-xs">Customer *</span></label>
            <!-- Pinned after creation: re-pointing a thing at another customer
                 would orphan its (customer, code) join and every ticket on it. -->
            <SearchSelect
              v-if="!isEdit"
              v-model="form.customer"
              :options="customerOptions"
              size="sm"
              placeholder="Type to find a customer…"
              :disabled="saving"
              @update:model-value="onCustomerChange"
            />
            <input v-else type="text" class="input input-bordered input-sm" :value="customer?.name || '—'" disabled />
          </div>

          <div class="flex gap-2">
            <div class="form-control flex-1 min-w-0">
              <label class="label py-1"><span class="label-text text-xs">Name *</span></label>
              <input v-model="form.name" type="text" maxlength="200" class="input input-bordered input-sm" :disabled="!editing || saving" />
            </div>
            <div class="form-control w-32">
              <label class="label py-1"><span class="label-text text-xs">Code</span></label>
              <input v-model="form.code" type="text" maxlength="100" class="input input-bordered input-sm font-mono" :disabled="!editing || saving" />
            </div>
          </div>
          <p v-if="form.code && customer" class="text-xs text-base-content/50 font-mono -mt-1">
            platform join: {{ customer.name }} / {{ form.code }}
          </p>

          <div class="form-control">
            <label class="label py-1">
              <span class="label-text text-xs">Type</span>
              <router-link v-if="editing" to="/staff/thing-types" class="label-text-alt link link-hover">manage types →</router-link>
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

          <div class="form-control">
            <label class="label py-1">
              <span class="label-text text-xs">Location</span>
              <router-link v-if="form.location" :to="`/staff/locations/${form.location}`" class="label-text-alt link link-hover">view →</router-link>
            </label>
            <SearchSelect
              v-model="form.location"
              :options="locationOptions"
              size="sm"
              empty-label="None"
              placeholder="Where does it live?"
              :disabled="!editing || saving || !form.customer"
            />
          </div>

          <div class="form-control">
            <label class="label py-1"><span class="label-text text-xs">Notes</span></label>
            <textarea v-model="form.notes" rows="3" maxlength="2000" class="textarea textarea-bordered textarea-sm" :disabled="!editing || saving"></textarea>
          </div>

          <div class="form-control">
            <label class="label cursor-pointer justify-start gap-3 py-1">
              <input v-model="form.retired" type="checkbox" class="toggle toggle-sm toggle-primary" :disabled="!editing || saving" />
              <span class="label-text text-xs">
                Retired
                <span class="text-base-content/50">— decommissioned; hidden from pickers, history kept</span>
              </span>
            </label>
          </div>

          <div v-if="isEdit && auth.isAdmin && editing" class="pt-2">
            <button class="btn btn-ghost btn-sm text-error" @click="remove">Delete thing</button>
          </div>
        </div>
      </div>

      <div class="space-y-4">
        <!-- Metadata -->
        <div class="card bg-base-100 shadow-sm">
          <div class="card-body p-4">
            <h2 class="card-title text-base">Metadata</h2>
            <p class="text-xs text-base-content/60 mb-2">
              <template v-if="activeSchema">Fields defined by this thing's type.</template>
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

        <!-- Tickets for this thing — the reason the relation exists. -->
        <div v-if="isEdit" class="card bg-base-100 shadow-sm">
          <div class="card-body p-4">
            <div class="flex items-center justify-between gap-2">
              <h2 class="card-title text-base">Tickets for this thing</h2>
              <!-- Carries the customer too: the queue's thing picker is gated on
                   it, so without it the filter would apply with an empty,
                   disabled control and no visible sign of what's filtering. -->
              <router-link
                v-if="ticketTotal"
                :to="`/staff/tickets?customer=${form.customer}&thing=${id}`"
                class="text-xs link link-hover"
              >View all →</router-link>
            </div>
            <p v-if="ticketTotal" class="text-xs text-base-content/60">
              {{ ticketTotal }} ticket{{ ticketTotal === 1 ? '' : 's' }} · {{ openTotal }} open
            </p>

            <div v-if="!tickets.length" class="text-sm text-base-content/60 italic">
              No tickets for this thing yet.
            </div>
            <div v-else class="divide-y divide-base-200">
              <router-link
                v-for="t in tickets"
                :key="t.id"
                :to="`/staff/tickets/${t.id}`"
                class="flex items-center gap-2 py-2 hover:bg-base-200/40 -mx-2 px-2 rounded"
              >
                <span class="font-mono text-xs text-base-content/60 shrink-0">#{{ t.number }}</span>
                <span class="text-sm truncate flex-1 min-w-0">{{ t.title }}</span>
                <TicketBadges :status="t.status" :priority="t.priority" />
              </router-link>
            </div>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>
