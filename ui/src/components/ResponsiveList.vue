<script setup lang="ts" generic="T extends { id: string }">
// Dual-render list lifted from the access-control sibling: a dense table on
// lg+ screens, stacked cards below. Driven by a column config; per-cell
// overrides via `cell-{key}` (table) and `card-{key}` (card) slots — card
// slots fall back to the cell slot so views only write a card override when
// the mobile rendering genuinely differs. The first column becomes the card
// header on mobile.
import { computed, ref, watchEffect } from 'vue'

export interface Column<T = any> {
  key: string
  label: string
  format?: (value: any, item: T) => string
  class?: string
  mobileLabel?: string
  /** Skip this column in the mobile card grid (e.g. when the card header slot already shows it). */
  hideOnMobile?: boolean
  /** Make the desktop header a click-to-sort control (emits `sort` with this key). */
  sortable?: boolean
}

interface Props {
  items: T[]
  columns: Column<T>[]
  clickable?: boolean
  /** Opt-in row selection (checkbox column on desktop, checkbox in the card on mobile). */
  selectable?: boolean
  /** Selected item ids (v-model:selected). Selection can span pages. */
  selected?: string[]
  /** Active sort column key + direction, for the sortable-header arrows. */
  sortKey?: string
  sortDir?: 'asc' | 'desc'
}

const props = withDefaults(defineProps<Props>(), {
  clickable: true,
  selectable: false,
  selected: () => [],
  sortKey: '',
  sortDir: 'desc',
})

const emit = defineEmits<{
  'row-click': [item: T]
  'update:selected': [ids: string[]]
  sort: [key: string]
}>()

function get(obj: any, path: string): any {
  return path.split('.').reduce((acc, part) => acc?.[part], obj)
}

const cardColumns = computed(() => props.columns.slice(1).filter((c) => !c.hideOnMobile))

// --- selection (only meaningful when `selectable`) ---
// Set-backed lookup: `selected` can span pages and grow large, and
// isSelected runs per row per render in both trees.
const selectedSet = computed(() => new Set(props.selected))
function isSelected(id: string): boolean {
  return selectedSet.value.has(id)
}
const allSelected = computed(
  () => props.items.length > 0 && props.items.every((i) => selectedSet.value.has(i.id)),
)
const someSelected = computed(
  () => props.items.some((i) => selectedSet.value.has(i.id)) && !allSelected.value,
)
// The select-all boxes (table header + mobile bar) show a third "partial"
// state when some-but-not-all rows are ticked.
const selectAllEl = ref<HTMLInputElement | null>(null)
const selectAllElMobile = ref<HTMLInputElement | null>(null)
watchEffect(() => {
  if (selectAllEl.value) selectAllEl.value.indeterminate = someSelected.value
  if (selectAllElMobile.value) selectAllElMobile.value.indeterminate = someSelected.value
})

function toggle(id: string) {
  const set = new Set(props.selected)
  if (set.has(id)) set.delete(id)
  else set.add(id)
  emit('update:selected', [...set])
}
// Select-all toggles only the *current page's* rows, preserving any off-page selection.
function toggleAll() {
  const pageIds = props.items.map((i) => i.id)
  if (allSelected.value) {
    const drop = new Set(pageIds)
    emit('update:selected', props.selected.filter((id) => !drop.has(id)))
  } else {
    const set = new Set(props.selected)
    pageIds.forEach((id) => set.add(id))
    emit('update:selected', [...set])
  }
}

function handleClick(item: T) {
  if (props.clickable) emit('row-click', item)
}

function handleKey(e: KeyboardEvent, item: T) {
  if (!props.clickable) return
  // Only act on keys aimed at the row itself — Space/Enter bubbling up from
  // a nested control (selection checkbox, select, button) must keep its
  // native behavior, not turn into navigation.
  if (e.target !== e.currentTarget) return
  if (e.key === 'Enter' || e.key === ' ') {
    e.preventDefault()
    emit('row-click', item)
  }
}
</script>

<template>
  <div class="w-full">
    <!-- DESKTOP: table -->
    <div v-if="items.length > 0" class="hidden lg:block overflow-x-auto bg-base-100 rounded-lg shadow-sm">
      <table class="table table-sm w-full">
        <thead>
          <tr class="border-b border-base-300">
            <th v-if="selectable" class="w-8">
              <input
                ref="selectAllEl"
                type="checkbox"
                class="checkbox checkbox-sm align-middle"
                :checked="allSelected"
                aria-label="Select all on this page"
                @click.stop
                @change="toggleAll"
              />
            </th>
            <th v-for="col in columns" :key="col.key" :class="col.class" class="text-[11px] uppercase tracking-wider opacity-60">
              <button
                v-if="col.sortable"
                type="button"
                class="inline-flex items-center gap-1 uppercase tracking-wider hover:opacity-100"
                @click.stop="emit('sort', col.key)"
              >
                {{ col.label }}
                <span v-if="sortKey === col.key" class="text-primary">{{ sortDir === 'asc' ? '▲' : '▼' }}</span>
              </button>
              <template v-else>{{ col.label }}</template>
            </th>
            <th v-if="$slots.actions" class="text-right text-[11px] uppercase tracking-wider opacity-60">Actions</th>
          </tr>
        </thead>
        <tbody>
          <tr
            v-for="item in items"
            :key="item.id"
            :class="[{ 'hover cursor-pointer': clickable }, { 'bg-primary/5': selectable && isSelected(item.id) }]"
            class="border-b border-base-200/50 last:border-0 focus-visible:outline focus-visible:outline-2 focus-visible:outline-primary/60"
            :tabindex="clickable ? 0 : undefined"
            :role="clickable ? 'button' : undefined"
            @click="handleClick(item)"
            @keydown="handleKey($event, item)"
          >
            <td v-if="selectable" class="w-8 py-3" @click.stop>
              <input
                type="checkbox"
                class="checkbox checkbox-sm align-middle"
                :checked="isSelected(item.id)"
                :aria-label="`Select ${item.id}`"
                @change="toggle(item.id)"
              />
            </td>
            <td v-for="col in columns" :key="col.key" :class="col.class" class="py-3">
              <slot :name="`cell-${col.key}`" :item="item" :value="get(item, col.key)">
                <span class="text-sm">
                  {{ col.format ? col.format(get(item, col.key), item) : get(item, col.key) || '—' }}
                </span>
              </slot>
            </td>
            <td v-if="$slots.actions" @click.stop class="py-3">
              <div class="flex justify-end gap-2">
                <slot name="actions" :item="item" />
              </div>
            </td>
          </tr>
        </tbody>
      </table>
    </div>

    <!-- MOBILE: high-density cards -->
    <div v-if="items.length > 0" class="lg:hidden space-y-2">
      <label v-if="selectable" class="flex items-center gap-2 px-1 cursor-pointer w-fit">
        <input
          ref="selectAllElMobile"
          type="checkbox"
          class="checkbox checkbox-sm"
          :checked="allSelected"
          aria-label="Select all on this page"
          @change="toggleAll"
        />
        <span class="text-xs font-medium opacity-60">Select all on this page</span>
      </label>

      <div
        v-for="item in items"
        :key="item.id"
        :class="[
          'card bg-base-100 border shadow-sm transition-all duration-200',
          selectable && isSelected(item.id) ? 'border-primary/60 bg-primary/5' : 'border-base-300',
          { 'cursor-pointer active:scale-[0.98] hover:border-primary/40 focus-visible:outline focus-visible:outline-2 focus-visible:outline-primary/60': clickable },
        ]"
        :tabindex="clickable ? 0 : undefined"
        :role="clickable ? 'button' : undefined"
        @click="handleClick(item)"
        @keydown="handleKey($event, item)"
      >
        <div class="card-body p-3 gap-0">
          <!-- Header: selection + identity. min-w-0 + truncate clips long slot
               content with an ellipsis instead of overflowing. -->
          <div class="min-w-0 flex items-center gap-2">
            <input
              v-if="selectable"
              type="checkbox"
              class="checkbox checkbox-sm shrink-0"
              :checked="isSelected(item.id)"
              :aria-label="`Select ${item.id}`"
              @click.stop
              @change="toggle(item.id)"
            />
            <div class="min-w-0 flex-1 truncate">
              <slot :name="`card-${columns[0].key}`" :item="item" :value="get(item, columns[0].key)">
                <div class="text-sm font-bold text-primary truncate">
                  {{ columns[0].format ? columns[0].format(get(item, columns[0].key), item) : get(item, columns[0].key) || 'Unnamed' }}
                </div>
              </slot>
            </div>
          </div>

          <div
            v-if="cardColumns.length > 0"
            class="grid grid-cols-2 gap-x-3 gap-y-1 border-t border-base-200/60 mt-2 pt-2"
          >
            <div
              v-for="col in cardColumns"
              :key="col.key"
              class="flex items-center gap-1.5 overflow-hidden"
            >
              <span class="text-[10px] uppercase font-bold opacity-50 tracking-tight shrink-0">
                {{ col.mobileLabel || col.label }}:
              </span>
              <div class="flex-1 truncate">
                <slot :name="`card-${col.key}`" :item="item" :value="get(item, col.key)">
                  <slot :name="`cell-${col.key}`" :item="item" :value="get(item, col.key)">
                    <span class="text-xs font-medium text-base-content/80">
                      {{ col.format ? col.format(get(item, col.key), item) : get(item, col.key) || '—' }}
                    </span>
                  </slot>
                </slot>
              </div>
            </div>
          </div>

          <!-- Row actions get their own row: text labels stay legible on
               touch screens, where title-attribute tooltips never show. -->
          <div v-if="$slots.actions" class="flex flex-wrap justify-end gap-1 border-t border-base-200/60 mt-2 pt-2" @click.stop>
            <slot name="actions" :item="item" />
          </div>
        </div>
      </div>
    </div>

    <!-- EMPTY -->
    <div v-if="items.length === 0" class="text-center py-12 bg-base-200/30 rounded-xl border-2 border-dashed border-base-300">
      <slot name="empty">
        <div class="flex flex-col items-center gap-2 opacity-40">
          <span class="text-4xl">📭</span>
          <span class="text-sm font-bold uppercase tracking-widest">No items found</span>
        </div>
      </slot>
    </div>
  </div>
</template>

<style scoped>
.card-body {
  min-height: unset;
}
</style>
