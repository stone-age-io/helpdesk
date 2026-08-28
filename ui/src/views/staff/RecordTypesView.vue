<script setup lang="ts">
// Admin-managed thing / location types — the classifier a thing or location
// belongs to, mirroring the platform's thing_types / location_types minus their
// NATS-contract fields (capabilities, subject_prefix, operations, nats_role).
//
// ONE view for both collections: once the control-plane fields are stripped the
// two are the same shape, so parameterising on the collection name beats two
// 250-line copies that would drift. They stay two COLLECTIONS because that is
// the platform's shape and it keeps an export→seed a 1:1 map.
//
// `metadata_schema` is the reason this collection exists. It is a JSON Schema
// saying which keys records of this type track, which is what stops `metadata`
// from becoming a bag of drifting key spellings (serial / Serial / sn). It is
// edited here as raw JSON, deliberately: the helpdesk is DOWNSTREAM of schema
// authoring — schemas are written in the platform console (which has a visual
// builder) and arrive by seed. A null schema is not an error; it just means the
// record form falls back to free-form key/value rows.
//
// Reads as a roster (read-only rows, edit via a panel above the list), matching
// CategoriesView and the other staff list views via the shared ResponsiveList.
import { computed, nextTick, onMounted, ref, watch } from 'vue'
import { pb } from '@/pb'
import type { Customer, RecordType } from '@/types'
import SearchSelect from '@/components/SearchSelect.vue'
import ResponsiveList, { type Column } from '@/components/ResponsiveList.vue'

const props = defineProps<{
  /** PocketBase collection name: `thing_types` or `location_types`. */
  collection: string
  /** What records of this type are, for headings and help text: "Thing", "Location". */
  noun: string
}>()

const columns: Column<RecordType>[] = [
  { key: 'name', label: 'Name' },
  { key: 'expand.customer.name', label: 'Customer' },
  { key: 'code', label: 'Code' },
  { key: 'description', label: 'Description' },
  { key: 'metadata_schema', label: 'Schema' },
]

const types = ref<RecordType[]>([])
const customers = ref<Customer[]>([])
const loading = ref(true)
const error = ref('')
const saving = ref(false)
const search = ref('')

// Client-side filter — the list is loaded whole (getFullList, no pager). A type
// taxonomy is small by nature; if it ever isn't, follow TicketQueueView's
// server-side getList + buildFilter rather than growing this.
const filtered = computed(() => {
  const q = search.value.trim().toLowerCase()
  if (!q) return types.value
  return types.value.filter((t) =>
    (t.name || '').toLowerCase().includes(q) ||
    (t.code || '').toLowerCase().includes(q) ||
    (t.description || '').toLowerCase().includes(q) ||
    ((t.expand?.customer as Customer | undefined)?.name || '').toLowerCase().includes(q),
  )
})

const customerOptions = computed(() => customers.value.map((c) => ({ id: c.id, label: c.name })))

// New-type form.
const newName = ref('')
const newCode = ref('')
const newCustomer = ref('')
const creating = ref(false)

// Slug the code from the name until the admin edits it, matching how the
// platform console mints a code. Keeps the join key predictable without
// forcing a second field on the common path.
const codeTouched = ref(false)
watch(newName, (name) => {
  if (!codeTouched.value) newCode.value = slugify(name)
})

function slugify(s: string): string {
  return s.toLowerCase().trim().replace(/[^a-z0-9]+/g, '-').replace(/^-+|-+$/g, '')
}

// Inline row editing, in a panel above the list.
const editing = ref<RecordType | null>(null)
const editForm = ref({ name: '', code: '', description: '' })
const schemaText = ref('')
const schemaError = ref('')

// Field count for the roster column: enough to tell "typed" from "free-form" at
// a glance without rendering the schema.
function schemaFieldCount(t: RecordType): number {
  const s = t.metadata_schema
  if (!s || typeof s !== 'object' || !s.properties) return 0
  return Object.keys(s.properties).length
}

async function load() {
  loading.value = true
  error.value = ''
  try {
    types.value = await pb.collection(props.collection).getFullList<RecordType>({
      sort: 'name',
      expand: 'customer',
    })
  } catch (err: any) {
    error.value = err?.message || `Failed to load ${props.noun.toLowerCase()} types`
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
    // The picker degrades to empty; the error surfaces on save instead.
  }
}

async function create() {
  const name = newName.value.trim()
  if (!name || !newCustomer.value) return
  creating.value = true
  error.value = ''
  try {
    await pb.collection(props.collection).create({
      customer: newCustomer.value,
      name,
      code: newCode.value.trim(),
      metadata_schema: null,
    })
    newName.value = ''
    newCode.value = ''
    codeTouched.value = false
    await load()
  } catch (err: any) {
    error.value = err?.message || 'Failed to create (code must be unique per customer)'
  } finally {
    creating.value = false
  }
}

// The edit panel renders above the list, which can be off-screen when the
// triggering row is below the fold — bring it into view.
const editCard = ref<HTMLElement | null>(null)
function startEdit(t: RecordType) {
  editing.value = t
  editForm.value = {
    name: t.name || '',
    code: t.code || '',
    description: t.description || '',
  }
  schemaText.value = t.metadata_schema ? JSON.stringify(t.metadata_schema, null, 2) : ''
  schemaError.value = ''
  nextTick(() => editCard.value?.scrollIntoView({ behavior: 'smooth', block: 'nearest' }))
}

// Parse-validate before save, the same posture the notification template editor
// takes: a bad schema is caught here, not after it has been stored and the
// record form silently stops rendering typed fields.
function parseSchema(): { ok: boolean; value: Record<string, any> | null } {
  const text = schemaText.value.trim()
  if (!text) return { ok: true, value: null }
  let parsed: any
  try {
    parsed = JSON.parse(text)
  } catch (err: any) {
    schemaError.value = err.message
    return { ok: false, value: null }
  }
  if (typeof parsed !== 'object' || parsed === null || Array.isArray(parsed)) {
    schemaError.value = 'A schema must be a JSON object.'
    return { ok: false, value: null }
  }
  if (parsed.type !== 'object' || !parsed.properties || typeof parsed.properties !== 'object') {
    schemaError.value = 'Expected a JSON Schema of the form {"type":"object","properties":{…}}.'
    return { ok: false, value: null }
  }
  // A property-less schema is not a schema — store null so the record form falls
  // back to free-form rows instead of rendering an empty, un-addable form.
  if (Object.keys(parsed.properties).length === 0) return { ok: true, value: null }
  schemaError.value = ''
  return { ok: true, value: parsed }
}

async function saveEdit() {
  if (!editing.value) return
  const schema = parseSchema()
  if (!schema.ok) return
  saving.value = true
  error.value = ''
  try {
    await pb.collection(props.collection).update(editing.value.id, {
      name: editForm.value.name.trim(),
      code: editForm.value.code.trim(),
      description: editForm.value.description.trim(),
      metadata_schema: schema.value,
    })
    editing.value = null
    await load()
  } catch (err: any) {
    error.value = err?.message || 'Failed to save (code must be unique per customer)'
  } finally {
    saving.value = false
  }
}

async function remove(t: RecordType) {
  if (
    !confirm(
      `Delete “${t.name}”? Records already using it keep their metadata but lose the typed form. ` +
        `Their type field renders blank until re-classified.`,
    )
  )
    return
  error.value = ''
  try {
    await pb.collection(props.collection).delete(t.id)
    if (editing.value?.id === t.id) editing.value = null
    await load()
  } catch (err: any) {
    error.value = err?.message || 'Failed to delete'
  }
}

// Both routes render this component; switching between them reuses the instance,
// so onMounted alone would leave the second showing the first's rows.
watch(() => props.collection, load)
onMounted(() => {
  load()
  loadCustomers()
})
</script>

<template>
  <div class="space-y-4">
    <h1 class="text-2xl font-bold">{{ noun }} Types</h1>
    <p class="text-sm text-base-content/60">
      What a {{ noun.toLowerCase() }} <em>is</em>, and which metadata fields it tracks. Mirrors the
      platform's {{ collection }} — <code class="font-mono text-xs">code</code> is the join key, so
      keep it matching upstream. A type with a schema gives its records a typed
      metadata form; one without falls back to free-form key/value rows.
    </p>

    <div v-if="error" class="alert alert-error py-2 text-sm">{{ error }}</div>

    <!-- New type -->
    <form class="flex flex-col sm:flex-row gap-2 sm:items-end" @submit.prevent="create">
      <div class="form-control">
        <label class="label py-1"><span class="label-text text-xs">Customer *</span></label>
        <div class="w-full sm:w-56">
          <SearchSelect
            v-model="newCustomer"
            :options="customerOptions"
            size="sm"
            placeholder="Type to find…"
            :disabled="creating"
          />
        </div>
      </div>
      <div class="form-control">
        <label class="label py-1"><span class="label-text text-xs">New type</span></label>
        <input
          v-model="newName"
          type="text"
          :placeholder="noun === 'Thing' ? 'e.g. Door Controller' : 'e.g. Building'"
          class="input input-bordered input-sm w-full sm:w-56"
          :disabled="creating"
        />
      </div>
      <div class="form-control">
        <label class="label py-1"><span class="label-text text-xs">Code</span></label>
        <input
          v-model="newCode"
          type="text"
          class="input input-bordered input-sm w-full sm:w-40 font-mono"
          :disabled="creating"
          @input="codeTouched = true"
        />
      </div>
      <button
        type="submit"
        class="btn btn-primary btn-sm"
        :disabled="creating || !newName.trim() || !newCustomer"
      >
        Add
      </button>
    </form>

    <!-- Edit panel: lives above the list (an inline table row can't render
         inside the mobile card layout). -->
    <div v-if="editing" ref="editCard" class="card bg-base-100 shadow-sm">
      <div class="card-body p-4 space-y-2">
        <h2 class="card-title text-sm">Edit {{ editing.name }}</h2>
        <form class="space-y-3" @submit.prevent="saveEdit">
          <div class="flex flex-col sm:flex-row sm:flex-wrap gap-2 items-stretch sm:items-end">
            <div class="form-control">
              <label class="label py-1"><span class="label-text text-xs">Name *</span></label>
              <input v-model="editForm.name" type="text" class="input input-bordered input-sm w-full sm:w-48" :disabled="saving" />
            </div>
            <div class="form-control">
              <label class="label py-1"><span class="label-text text-xs">Code</span></label>
              <input v-model="editForm.code" type="text" class="input input-bordered input-sm w-full sm:w-40 font-mono" :disabled="saving" />
            </div>
            <div class="form-control flex-1 min-w-0">
              <label class="label py-1"><span class="label-text text-xs">Description</span></label>
              <input v-model="editForm.description" type="text" class="input input-bordered input-sm w-full" :disabled="saving" />
            </div>
          </div>

          <div class="form-control">
            <label class="label py-1">
              <span class="label-text text-xs">Metadata schema (JSON Schema)</span>
              <span class="label-text-alt text-xs text-base-content/50">blank = free-form fields</span>
            </label>
            <textarea
              v-model="schemaText"
              rows="10"
              class="textarea textarea-bordered textarea-sm font-mono text-xs"
              :class="{ 'textarea-error': !!schemaError }"
              placeholder='{"type":"object","properties":{"serial":{"type":"string","title":"Serial number"}}}'
              :disabled="saving"
            ></textarea>
            <label v-if="schemaError" class="label py-1">
              <span class="label-text-alt text-error">{{ schemaError }}</span>
            </label>
            <label v-else class="label py-1">
              <span class="label-text-alt text-base-content/50">
                Authored in the platform console and copied here — this app renders schemas, it doesn't build them.
              </span>
            </label>
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

    <input
      v-model="search"
      type="search"
      placeholder="Filter by name, code, or customer…"
      class="input input-bordered input-sm w-full sm:w-72"
    />

    <div v-if="loading" class="flex justify-center p-12"><span class="loading loading-spinner loading-lg"></span></div>

    <ResponsiveList v-else :items="filtered" :columns="columns" :clickable="false">
      <template #cell-name="{ item }"><span class="font-medium text-sm">{{ item.name }}</span></template>
      <template #cell-code="{ value }"><span class="font-mono text-xs">{{ value || '—' }}</span></template>
      <template #cell-description="{ value }">
        <span class="text-sm text-base-content/70">{{ value || '—' }}</span>
      </template>
      <template #cell-metadata_schema="{ item }">
        <span v-if="schemaFieldCount(item)" class="badge badge-sm badge-soft-info">
          {{ schemaFieldCount(item) }} field{{ schemaFieldCount(item) === 1 ? '' : 's' }}
        </span>
        <span v-else class="text-xs text-base-content/50">free-form</span>
      </template>
      <template #actions="{ item }">
        <button class="btn btn-ghost btn-xs" @click="editing?.id === item.id ? (editing = null) : startEdit(item)">
          {{ editing?.id === item.id ? 'Cancel' : 'Edit' }}
        </button>
        <button class="btn btn-ghost btn-xs text-error" @click="remove(item)">Delete</button>
      </template>
      <template #empty>
        <span class="text-base-content/60">No {{ noun.toLowerCase() }} types{{ search ? ' match.' : ' yet.' }}</span>
      </template>
    </ResponsiveList>
  </div>
</template>
