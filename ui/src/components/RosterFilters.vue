<script setup lang="ts">
// The filter row shared by the Directory rosters that carry a customer and a
// type (Locations, Things): free-text search, a customer picker, and a type
// picker, plus a slot for whatever else a roster needs.
//
// WHY THE TYPE FILTER KEYS ON NAME, NOT ID
//
// thing_types / location_types are customer-scoped (required `customer`
// relation, mirroring the platform), so eight customers each own their own
// "Door Controller" row. Filtering on the type *id* would therefore need the
// customer chosen first to be unambiguous — a control that is dead until you
// touch another one, and which then answers only "this customer's door
// controllers".
//
// Keying on the type NAME dissolves the problem instead of gating around it:
// the options dedupe, so the list never shows "Door Controller" three times,
// and picking it means every door controller on the roster — which is the
// cross-customer question a roster filter is actually asked. Same-named types
// across customers are the same concept by design (that is what `code`, the
// platform join key, is for), not a collision to be disambiguated.
//
// The customer picker still narrows the type list, because the options are
// derived from the rows currently in scope rather than from the full taxonomy.
// That makes it a dependent LIST, not a dependent gate: always usable, and it
// never offers a type that would return nothing.
import { computed } from 'vue'
import type { Customer } from '@/types'
import SearchSelect from './SearchSelect.vue'

/** The shape both rosters share: a customer id and an expanded `type`. */
export interface RosterItem {
  customer: string
  expand?: Record<string, any>
}

const props = withDefaults(
  defineProps<{
    /** Every loaded row — the type options are derived from these. */
    items: RosterItem[]
    customers: Customer[]
    search: string
    /** Selected customer id ('' = all). */
    customer: string
    /** Selected type NAME ('' = all) — see the note above. */
    type: string
    searchPlaceholder?: string
  }>(),
  { searchPlaceholder: 'Filter…' },
)

const emit = defineEmits<{
  (e: 'update:search', v: string): void
  (e: 'update:customer', v: string): void
  (e: 'update:type', v: string): void
}>()

const customerOptions = computed(() => props.customers.map((c) => ({ id: c.id, label: c.name })))

// Derived from the rows the customer filter leaves in scope, deduped by name.
// A type nobody uses is noise in a filter, and this needs no second request.
const typeOptions = computed(() => {
  const inScope = props.customer
    ? props.items.filter((i) => i.customer === props.customer)
    : props.items
  const seen = new Set<string>()
  for (const i of inScope) {
    const name = i.expand?.type?.name
    if (name) seen.add(name)
  }
  return [...seen].sort((a, b) => a.localeCompare(b)).map((name) => ({ id: name, label: name }))
})

// Narrowing to a customer can strip the selected type out of the option list;
// drop the selection rather than leave a filter that silently matches nothing.
function onCustomer(v: string) {
  emit('update:customer', v)
  if (props.type && !props.items.some((i) => (!v || i.customer === v) && i.expand?.type?.name === props.type)) {
    emit('update:type', '')
  }
}
</script>

<template>
  <div class="flex flex-col sm:flex-row sm:flex-wrap gap-2">
    <input
      :value="search"
      type="search"
      :placeholder="searchPlaceholder"
      class="input input-bordered input-sm w-full sm:w-72"
      @input="emit('update:search', ($event.target as HTMLInputElement).value)"
    />
    <div class="w-full sm:w-52">
      <SearchSelect
        :model-value="customer"
        :options="customerOptions"
        size="sm"
        empty-label="All customers"
        placeholder="Customer…"
        @update:model-value="onCustomer"
      />
    </div>
    <div class="w-full sm:w-52">
      <SearchSelect
        :model-value="type"
        :options="typeOptions"
        size="sm"
        empty-label="All types"
        placeholder="Type…"
        @update:model-value="emit('update:type', $event)"
      />
    </div>
    <slot />
  </div>
</template>
