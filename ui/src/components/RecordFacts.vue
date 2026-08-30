<script setup lang="ts">
// Read-only `metadata`, for the PORTAL — the requester-facing counterpart to
// what MetadataEditor shows staff.
//
// IT SHOWS EVERY KEY. The schema, when the record's type has one, decides the
// ORDER and the LABELS; it does not decide what is visible.
//
// That is a deliberate reversal of where this started. The first cut rendered
// only schema-declared keys, on the theory that a schema is an authored
// statement of what a type tracks while an undeclared key is whatever somebody
// typed into the JSON tab — and so the undeclared half was where an internal
// note would end up. The operator's answer was the simpler one: nothing goes in
// a location's or a thing's `metadata` that the customer should not read. It is
// their building and their equipment, and the fields are facts about it —
// serial, firmware, last inspected, square footage, dock doors. The text that
// genuinely is ours lives in `notes`, which is withheld from these views and
// stays withheld; that is the field carrying the access codes and the service
// history, and it is a different field for a reason.
//
// So the rule to hold onto is about `notes` versus `metadata`, not about schema
// versus free-form. If something in `metadata` ever needs hiding, the fix is to
// move it to `notes` (or to a staff-only field), not to add a filter here — a
// filter would make the boundary invisible to whoever writes the value.
//
// Still not a MetadataEditor with `disabled`: that one carries a JSON tab, and
// a whole-document escape hatch with raw types and nesting is a staff
// affordance, not something a requester needs.
import { computed } from 'vue'

const props = withDefaults(
  defineProps<{
    metadata?: Record<string, any> | null
    /**
     * The record type's `metadata_schema`. Optional, and purely presentational:
     * declared keys lead, in the order the schema lists them, and take the
     * property's `title` as their label. Everything else follows.
     */
    schema?: Record<string, any> | null
    heading?: string
  }>(),
  { heading: 'Details' },
)

interface Fact {
  key: string
  label: string
  value: string
  /** Objects and arrays render mono — they are documents, not prose. */
  raw?: boolean
}

// "reader_count" -> "Reader count". The fallback when the schema carries no
// `title`, and the only labelling a free-form key ever gets.
function humanize(key: string): string {
  const s = key.replace(/[_-]+/g, ' ').trim()
  return s.charAt(0).toUpperCase() + s.slice(1)
}

const facts = computed<Fact[]>(() => {
  const doc = props.metadata
  if (!doc || typeof doc !== 'object') return []

  const properties: Record<string, any> =
    props.schema && props.schema.type === 'object' && props.schema.properties ? props.schema.properties : {}

  // Schema order first — that is how the type's author chose to present the
  // fields — then whatever else the document holds, alphabetically, since a
  // document's own key order is an accident of whoever saved it last.
  const declared = Object.keys(properties)
  const rest = Object.keys(doc)
    .filter((k) => !(k in properties))
    .sort()

  const out: Fact[] = []
  for (const key of [...declared, ...rest]) {
    if (!(key in doc)) continue // schema declares it, this record has not set it
    const v = doc[key]
    if (v === null || v === undefined || v === '') continue
    const label = properties[key]?.title || humanize(key)
    if (typeof v === 'boolean') out.push({ key, label, value: v ? 'Yes' : 'No' })
    else if (typeof v === 'number' || typeof v === 'string') out.push({ key, label, value: String(v) })
    // Nested values are shown rather than skipped — the whole point here is
    // that the document is not filtered — but as compact JSON: a nested key is
    // a document, and pretending otherwise would flatten away its shape.
    else out.push({ key, label, value: JSON.stringify(v), raw: true })
  }
  return out
})

defineExpose({ facts })
</script>

<template>
  <!-- The whole section, heading and rule included, so a caller can drop this
       in without guarding it: nothing renders when the record has no metadata,
       and an empty divider would otherwise be left behind. -->
  <div v-if="facts.length" class="pt-2 border-t border-base-200">
    <h3 class="text-xs uppercase tracking-wide text-base-content/50">{{ heading }}</h3>
    <dl class="divide-y divide-base-200">
      <div v-for="f in facts" :key="f.key" class="py-2 flex flex-col sm:flex-row sm:gap-4 sm:items-baseline">
        <dt class="text-sm text-base-content/60 sm:w-44 shrink-0">{{ f.label }}</dt>
        <dd class="text-sm break-words min-w-0" :class="f.raw ? 'font-mono text-xs break-all' : ''">{{ f.value }}</dd>
      </div>
    </dl>
  </div>
</template>
