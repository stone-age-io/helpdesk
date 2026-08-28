<!-- ui/src/components/MetadataEditor.vue -->
<script setup lang="ts">
import { ref, computed, watch } from 'vue'
import JsonSchemaForm from '@/components/JsonSchemaForm.vue'
import { useToastStore } from '@/stores/toast'

// MetadataEditor — edits (or displays) a free-form `metadata` object with a
// Form / JSON toggle: the form is a convenience over the document, the JSON view
// is always the escape hatch, and anything the form cannot represent says so
// instead of silently dropping it.
//
// PORTED from the sibling platform app (ui/src/components/common/MetadataEditor.vue)
// per CLAUDE.md's "lift proven patterns from the siblings" convention, so a
// thing's metadata looks and behaves the same in both consoles. Two deliberate
// departures: the platform's JsonViewer (a v-html syntax highlighter wired to its
// own CSS variables) is replaced by a plain <pre>, and its schema-AUTHORING stack
// (SchemaBuilder / SchemaFieldEditor) is not ported at all — the helpdesk renders
// a type's metadata_schema but never builds one. Schemas are authored upstream
// and arrive by seed; the type roster here edits them as raw JSON.
//
// Two form modes, chosen by whether a schema was supplied:
//
//   schema given → JsonSchemaForm renders typed inputs. This is the
//     thing_types / location_types `metadata_schema` path: an admin describes
//     the fields tracked for a class of record once, and a member fills in a
//     form.
//
//   no schema → key/value rows. The generic fallback, and the only mode
//     available to a type with no schema.
//
// NESTING is handled per key, not by a recursive editor. A row whose value is
// an object or array gets a small JSON textarea — exactly what JsonSchemaForm
// already does for its own non-primitive properties. A tree editor was the
// obvious alternative and is not worth it: the JSON tab already edits nesting
// perfectly, and the real complaint was never "I can't author a nested object",
// it was that ONE nested key used to disable the row editor for every flat key
// in the document. Per-row typing fixes that; a tree would fix it at ten times
// the size.
//
// With `disabled`, the same component is the read-only viewer used on the
// detail pages, so the locked and unlocked states of a record's metadata cannot
// drift apart in layout or in what they choose to show.
//
// The model is an OBJECT, not a JSON string. The form views used to hold
// metadata as text and JSON.parse it at submit, which is why an invalid blob
// was only caught after the user hit Save.

interface Props {
  modelValue: Record<string, any> | null
  /** JSON Schema describing the expected fields. Omit for free-form key/value. */
  schema?: any
  /** Read-only: renders the viewer used by the detail pages. */
  disabled?: boolean
  /**
   * Show the field count in the toolbar. Set false when the surrounding card
   * header already carries the count, so it is not rendered twice.
   */
  showCount?: boolean
}

const props = withDefaults(defineProps<Props>(), { schema: null, disabled: false, showCount: true })
const emit = defineEmits<{
  (e: 'update:modelValue', value: Record<string, any> | null): void
}>()

const toast = useToastStore()

type RowType = 'text' | 'number' | 'boolean' | 'date' | 'json'

interface Row {
  key: string
  type: RowType
  value: any
  /** Textarea buffer for `json` rows, so an in-progress edit isn't reparsed. */
  text?: string
  /** Parse error for `json` rows. Blocks commit() while set. */
  error?: string
}

const activeTab = ref<'form' | 'json'>('form')
const jsonText = ref('')
const jsonError = ref('')
const rows = ref<Row[]>([])

let suppressNextWatch = false

const doc = computed<Record<string, any>>(() => props.modelValue ?? {})
const isEmpty = computed(() => Object.keys(doc.value).length === 0)

// Does this document have a schema to render typed fields from? An empty or
// property-less schema is treated as absent — JsonSchemaForm would render
// nothing and the user would have no way to add a field.
const hasSchema = computed(() => {
  const s = props.schema
  return !!(s && s.type === 'object' && s.properties && Object.keys(s.properties).length > 0)
})

// Keys present on the record but absent from the schema. JsonSchemaForm spreads
// the existing model on every edit so these survive a save — but they are
// invisible in the form, and silently carrying data the user cannot see is the
// same failure the platform’s schema-builder notice exists to prevent.
const extraKeys = computed(() => {
  if (!hasSchema.value) return []
  const known = new Set(Object.keys(props.schema.properties))
  return Object.keys(doc.value).filter(k => !known.has(k))
})

function isContainer(v: any): boolean {
  return typeof v === 'object' && v !== null
}

function inferType(v: any): RowType {
  if (isContainer(v)) return 'json'
  if (typeof v === 'boolean') return 'boolean'
  if (typeof v === 'number') return 'number'
  // An ISO calendar date round-trips through <input type="date"> unchanged, so
  // treating it as a date is safe; anything else stays text.
  if (typeof v === 'string' && /^\d{4}-\d{2}-\d{2}$/.test(v)) return 'date'
  return 'text'
}

function rowsFromDoc(d: Record<string, any>): Row[] {
  return Object.entries(d).map(([key, value]) => {
    const type = inferType(value)
    return {
      key,
      type,
      value: type === 'json' ? value : (value ?? ''),
      text: type === 'json' ? JSON.stringify(value, null, 2) : undefined,
      error: '',
    }
  })
}

watch(
  () => props.modelValue,
  () => {
    if (suppressNextWatch) { suppressNextWatch = false; return }
    rows.value = rowsFromDoc(doc.value)
    if (activeTab.value !== 'json') refreshJsonFromDoc()
  },
  { immediate: true, deep: true },
)

function refreshJsonFromDoc() {
  jsonText.value = isEmpty.value ? '' : JSON.stringify(doc.value, null, 2)
}

// Rows → document. Blank keys are skipped (an empty row is a row in progress,
// not an instruction to write ""), and on a duplicate key the last row wins —
// which matches what the JSON view would produce from the same text.
function emitFromRows() {
  const next: Record<string, any> = {}
  for (const r of rows.value) {
    const k = r.key.trim()
    if (!k) continue
    next[k] = r.value
  }
  suppressNextWatch = true
  emit('update:modelValue', Object.keys(next).length ? next : null)
}

const duplicateKeys = computed(() => {
  const seen = new Set<string>()
  const dupes = new Set<string>()
  for (const r of rows.value) {
    const k = r.key.trim()
    if (!k) continue
    if (seen.has(k)) dupes.add(k)
    seen.add(k)
  }
  return dupes
})

const rowErrors = computed(() => rows.value.filter(r => r.error).length)

function addRow() {
  rows.value.push({ key: '', type: 'text', value: '', error: '' })
}

function removeRow(i: number) {
  rows.value.splice(i, 1)
  emitFromRows()
}

function setRowKey(i: number, key: string) {
  rows.value[i].key = key
  emitFromRows()
}

// An emptied number input reports NaN, which is not JSON — store '' so the key
// round-trips as an empty value rather than serialising to null behind the user.
function numberOrBlank(n: number): number | string {
  return Number.isNaN(n) ? '' : n
}

function setRowValue(i: number, value: any) {
  rows.value[i].value = value
  emitFromRows()
}

// `json` rows keep their own text buffer so a half-typed object isn't parsed on
// every keystroke. Parse on input, but only emit when it succeeds — the last
// good value stays in the model, and commit() refuses to save while any row is
// still in error.
function setRowJson(i: number, raw: string) {
  const r = rows.value[i]
  r.text = raw
  if (!raw.trim()) {
    r.value = {}
    r.error = ''
    emitFromRows()
    return
  }
  try {
    const parsed = JSON.parse(raw)
    if (!isContainer(parsed)) {
      r.error = 'Expected an object or array — use another field type for a single value.'
      return
    }
    r.value = parsed
    r.error = ''
    emitFromRows()
  } catch (err: any) {
    r.error = err.message
  }
}

// Changing a row's type re-casts the value it already holds, so switching
// text→number on "42" keeps 42 rather than blanking the field.
function setRowType(i: number, type: RowType) {
  const r = rows.value[i]
  const old = r.value
  const wasContainer = isContainer(old)
  r.type = type
  r.error = ''

  if (type === 'json') {
    r.value = wasContainer ? old : {}
    r.text = JSON.stringify(r.value, null, 2)
    emitFromRows()
    return
  }

  r.text = undefined
  // Leaving `json`: an object has no sensible scalar cast (String() yields
  // "[object Object]"), so start the new type empty rather than write nonsense.
  if (wasContainer) {
    r.value = type === 'boolean' ? false : ''
  } else if (type === 'number') {
    const n = typeof old === 'number' ? old : parseFloat(String(old))
    r.value = Number.isNaN(n) ? '' : n
  } else if (type === 'boolean') {
    r.value = old === true || old === 'true'
  } else if (type === 'date') {
    r.value = /^\d{4}-\d{2}-\d{2}$/.test(String(old)) ? String(old) : ''
  } else {
    r.value = old === null || old === undefined ? '' : String(old)
  }
  emitFromRows()
}

function onSchemaFormUpdate(next: Record<string, any>) {
  suppressNextWatch = true
  emit('update:modelValue', Object.keys(next).length ? next : null)
}

function onJsonBlur() {
  const text = jsonText.value.trim()
  if (!text) {
    jsonError.value = ''
    suppressNextWatch = true
    emit('update:modelValue', null)
    return
  }
  try {
    const parsed = JSON.parse(text)
    if (typeof parsed !== 'object' || parsed === null || Array.isArray(parsed)) {
      jsonError.value = 'Metadata must be a JSON object.'
      return
    }
    jsonError.value = ''
    suppressNextWatch = true
    emit('update:modelValue', Object.keys(parsed).length ? parsed : null)
  } catch (err: any) {
    jsonError.value = err.message
  }
}

// Leaving the JSON tab with a parse error is refused, because the alternative
// is discarding what the user typed.
function switchTab(tab: 'form' | 'json') {
  if (tab === activeTab.value) return
  if (activeTab.value === 'json') {
    onJsonBlur()
    if (jsonError.value) {
      toast.error('JSON has errors — fix them before switching views')
      return
    }
    rows.value = rowsFromDoc(doc.value)
  } else {
    refreshJsonFromDoc()
  }
  activeTab.value = tab
}

// Read-only rendering. Ordered schema-first so a described field keeps a stable
// position, with its schema title as the label when it has one.
const viewEntries = computed(() => {
  const entries = Object.entries(doc.value)
  if (!hasSchema.value) return entries.map(([k, v]) => ({ key: k, label: k, value: v }))
  const order = Object.keys(props.schema.properties)
  const rank = (k: string) => {
    const i = order.indexOf(k)
    return i === -1 ? order.length : i
  }
  return entries
    .sort((a, b) => rank(a[0]) - rank(b[0]))
    .map(([k, v]) => ({
      key: k,
      label: props.schema.properties[k]?.title || k,
      value: v,
    }))
})

// Booleans render as `true` / `false`, not Yes/No. These values round-trip to and
// from the platform, so the display should say what is actually stored — a reader
// comparing this against the platform console should not have to translate.
function displayValue(v: any): string {
  if (v === null || v === undefined || v === '') return '—'
  if (typeof v === 'boolean') return v ? 'true' : 'false'
  if (isContainer(v)) return JSON.stringify(v, null, 2)
  return String(v)
}

// A record with many keys would otherwise make the read-only list unusably long
// and push whatever follows it on the detail page below the fold. Filtering is
// offered above a threshold rather than always, so the common short document
// keeps a clean header.
const viewFilter = ref('')
const FILTER_THRESHOLD = 10

const showViewFilter = computed(() => Object.keys(doc.value).length > FILTER_THRESHOLD)

const filteredViewEntries = computed(() => {
  const q = viewFilter.value.toLowerCase().trim()
  if (!q) return viewEntries.value
  return viewEntries.value.filter(e =>
    e.key.toLowerCase().includes(q) ||
    e.label.toLowerCase().includes(q) ||
    displayValue(e.value).toLowerCase().includes(q),
  )
})

// The parent calls this before submitting so a JSON tab left mid-edit is
// committed (or the save is refused) rather than silently ignored.
function commit(): boolean {
  if (activeTab.value === 'json') {
    onJsonBlur()
    if (jsonError.value) {
      toast.error('Metadata is not valid JSON')
      return false
    }
    return true
  }
  if (rowErrors.value > 0) {
    toast.error('A metadata field contains invalid JSON')
    return false
  }
  return true
}

defineExpose({ commit })
</script>

<template>
  <div>
    <div class="flex items-center justify-between gap-2 mb-4">
      <div role="tablist" class="tabs tabs-bordered">
        <a
          role="tab"
          class="tab"
          :class="{ 'tab-active': activeTab === 'form' }"
          @click="switchTab('form')"
        >{{ disabled ? 'Fields' : 'Form' }}</a>
        <a
          role="tab"
          class="tab"
          :class="{ 'tab-active': activeTab === 'json' }"
          @click="switchTab('json')"
        >JSON</a>
      </div>

      <span v-if="!isEmpty && showCount" class="text-xs text-base-content/50 shrink-0">
        {{ Object.keys(doc).length }} field{{ Object.keys(doc).length === 1 ? '' : 's' }}
      </span>
    </div>

    <!-- ---------- Form / Fields tab ---------- -->
    <template v-if="activeTab === 'form'">
      <!-- Read-only viewer (detail pages). Same tab pair as the editor, so the
           locked and unlocked states of a record look like each other. -->
      <template v-if="disabled">
        <div v-if="isEmpty" class="text-sm text-base-content/60 italic">
          No metadata recorded.
        </div>
        <template v-else>
          <input
            v-if="showViewFilter"
            v-model="viewFilter"
            type="text"
            class="input input-bordered input-sm w-full mb-2"
            placeholder="Filter fields…"
          />

          <div
            v-if="filteredViewEntries.length === 0"
            class="text-sm text-base-content/60 italic"
          >
            No field matches “{{ viewFilter }}”.
          </div>

          <!-- Capped and scrolled from `sm` up, unbounded below it. The cap keeps
               a long document from pushing whatever follows it on the page (Live
               State, on the Thing detail) below the fold — but on touch, a
               bounded scroller inside a scrolling page is a trap: the swipe goes
               to whichever of the two the thumb landed on. On mobile the card's
               collapse solves the same problem without that cost, so the cap
               would only be buying a second copy of a fix we already have. -->
          <dl v-else class="divide-y divide-base-300 sm:max-h-[500px] sm:overflow-y-auto">
            <div
              v-for="e in filteredViewEntries"
              :key="e.key"
              class="py-2 flex flex-col sm:flex-row sm:gap-4 sm:items-baseline"
            >
              <dt class="text-xs font-medium text-base-content/60 sm:w-1/3 shrink-0 font-mono break-all">
                {{ e.label }}
              </dt>
              <dd class="text-sm min-w-0 flex-1">
                <pre
                  v-if="typeof e.value === 'object' && e.value !== null"
                  class="font-mono text-xs bg-base-200 rounded p-2 overflow-x-auto"
                >{{ displayValue(e.value) }}</pre>
                <span v-else class="break-words">{{ displayValue(e.value) }}</span>
              </dd>
            </div>
          </dl>
        </template>
      </template>

      <!-- (b) Schema-driven: typed fields defined on the type. -->
      <template v-else-if="hasSchema">
        <JsonSchemaForm
          :schema="schema"
          :model-value="doc"
          @update:model-value="onSchemaFormUpdate"
        />

        <div v-if="extraKeys.length" class="alert alert-info text-xs mt-4">
          <span>
            {{ extraKeys.length }} field{{ extraKeys.length === 1 ? '' : 's' }} on this record
            {{ extraKeys.length === 1 ? 'is' : 'are' }} not described by this type's schema
            (<template v-for="(k, i) in extraKeys" :key="k"
              ><code>{{ k }}</code><span v-if="i < extraKeys.length - 1">, </span
            ></template>).
            {{ extraKeys.length === 1 ? 'It is' : 'They are' }} kept on save — switch to
            <strong>JSON</strong> to see or edit {{ extraKeys.length === 1 ? 'it' : 'them' }}.
          </span>
        </div>
      </template>

      <!-- (a) Free-form key/value rows. -->
      <template v-else>
        <div v-if="rows.length === 0" class="text-sm text-base-content/60 italic mb-3">
          No metadata yet. Add a field to record something about this record —
          a service date, an asset tag, a warranty reference.
        </div>

        <!-- Capped and scrolled from `sm` up, for the same reason and with the
             same mobile exception as the read-only list above. Unbounded here
             means the header's Save can scroll away, which is what the mobile
             action bar at the foot of MetadataCard is for. -->
        <div v-else class="space-y-2 mb-3 sm:max-h-[500px] sm:overflow-y-auto sm:pr-1">
          <!-- One field per row on a wide screen; two lines on a narrow one.
               Four controls do not fit in ~330px: at `flex-1` each, the key and
               the value ended up ~81px wide apiece while the type select took
               112px — the dropdown was wider than the data. Stacked, the key gets
               the full width it needs (they are long and mono, and they are what
               you scan), and type sits next to the value it types.

               The two wrappers are `sm:contents`, so above `sm` they dissolve and
               the four controls are direct children of the same single flex line
               this row has always been. One markup path, not two. The delete
               button's DOM position is inside the first wrapper, hence
               `sm:order-last` to put it back on the right at desktop. -->
          <div
            v-for="(r, i) in rows"
            :key="i"
            class="flex flex-col gap-2 rounded-lg bg-base-200/40 p-2
                   sm:flex-row sm:items-start sm:rounded-none sm:bg-transparent sm:p-0"
          >
            <div class="flex items-center gap-2 sm:contents">
              <input
                type="text"
                class="input input-bordered input-sm font-mono flex-1 min-w-0"
                :class="{ 'input-error': duplicateKeys.has(r.key.trim()) }"
                placeholder="key"
                :value="r.key"
                @input="setRowKey(i, ($event.target as HTMLInputElement).value)"
              />

              <button
                type="button"
                class="btn btn-ghost btn-square btn-sm shrink-0 sm:order-last"
                title="Remove field"
                @click="removeRow(i)"
              >
                ✕
              </button>
            </div>

            <div class="flex flex-wrap items-start gap-2 sm:contents">
              <select
                class="select select-bordered select-sm w-24 shrink-0 sm:w-28"
                :value="r.type"
                @change="setRowType(i, ($event.target as HTMLSelectElement).value as RowType)"
              >
                <option value="text">Text</option>
                <option value="number">Number</option>
                <option value="boolean">Boolean</option>
                <option value="date">Date</option>
                <option value="json">Object</option>
              </select>

              <!-- An Object row's textarea wraps to a line of its own on mobile:
                   sharing with the 96px select left it 159px wide, which renders
                   `{ "key": "value",` four characters at a time. Every other type
                   fits beside the select. -->
              <div
                class="form-control flex-1 min-w-0"
                :class="{ 'basis-full sm:basis-auto': r.type === 'json' }"
              >
                <!-- The literal stored value sits next to the toggle: this is what
                     round-trips to the platform, so show it rather than Yes/No. -->
                <label v-if="r.type === 'boolean'" class="flex items-center gap-2 h-8">
                  <input
                    type="checkbox"
                    class="toggle toggle-primary"
                    :checked="!!r.value"
                    @change="setRowValue(i, ($event.target as HTMLInputElement).checked)"
                  />
                  <code class="text-xs text-base-content/60">{{ r.value ? 'true' : 'false' }}</code>
                </label>
                <input
                  v-else-if="r.type === 'date'"
                  type="date"
                  class="input input-bordered input-sm w-full"
                  :value="r.value"
                  @input="setRowValue(i, ($event.target as HTMLInputElement).value)"
                />
                <input
                  v-else-if="r.type === 'number'"
                  type="number"
                  step="any"
                  class="input input-bordered input-sm font-mono w-full"
                  :value="r.value"
                  @input="setRowValue(i, numberOrBlank(($event.target as HTMLInputElement).valueAsNumber))"
                />
                <!-- Nested object / array: a JSON textarea for this key alone. -->
                <template v-else-if="r.type === 'json'">
                  <textarea
                    class="textarea textarea-bordered textarea-sm font-mono text-xs w-full"
                    :class="{ 'textarea-error': !!r.error }"
                    rows="4"
                    placeholder='{"nested": "value"}'
                    :value="r.text"
                    @input="setRowJson(i, ($event.target as HTMLTextAreaElement).value)"
                  ></textarea>
                  <span v-if="r.error" class="text-[10px] text-error mt-0.5">{{ r.error }}</span>
                </template>
                <input
                  v-else
                  type="text"
                  class="input input-bordered input-sm w-full"
                  placeholder="value"
                  :value="r.value"
                  @input="setRowValue(i, ($event.target as HTMLInputElement).value)"
                />
              </div>
            </div>
          </div>
        </div>

        <div v-if="duplicateKeys.size" class="text-xs text-error mb-2">
          Duplicate key{{ duplicateKeys.size === 1 ? '' : 's' }}:
          <code>{{ [...duplicateKeys].join(', ') }}</code> — the last row wins.
        </div>

        <button type="button" class="btn btn-sm w-full sm:w-auto" @click="addRow">
          + Add Field
        </button>
      </template>
    </template>

    <!-- ---------- JSON tab ---------- -->
    <template v-else>
      <div v-if="disabled">
        <div v-if="isEmpty" class="text-sm text-base-content/60 italic">
          No metadata recorded.
        </div>
        <div v-else class="bg-base-200 rounded-lg p-4 border border-base-300 overflow-hidden">
          <!-- Horizontal scroll stays at every width — a JSON line is as long as
               it is. The vertical cap is desktop-only, as above. -->
          <div class="overflow-x-auto sm:max-h-[500px] sm:overflow-y-auto">
            <pre class="font-mono text-xs leading-relaxed whitespace-pre-wrap break-words">{{ JSON.stringify(modelValue, null, 2) }}</pre>
          </div>
        </div>
      </div>
      <div v-else class="form-control">
        <textarea
          v-model="jsonText"
          class="textarea textarea-bordered font-mono text-xs"
          rows="12"
          placeholder='{"last_service": "2026-03-14"}'
          @blur="onJsonBlur"
        ></textarea>
        <label v-if="jsonError" class="label">
          <span class="label-text-alt text-error">{{ jsonError }}</span>
        </label>
      </div>
    </template>
  </div>
</template>
