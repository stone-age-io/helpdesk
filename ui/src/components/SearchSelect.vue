<script setup lang="ts">
// Typeahead replacement for raw <select> on long lists (customers,
// requesters, staff): type a few characters, pick from the narrowed list.
// Options are provided by the parent (already-loaded records) — filtering
// is client-side, which is plenty at MSP scale.
//
// The popup is deliberately NOT tied to the input's width. Filter inputs sit in
// narrow columns (`sm:w-52` on the Reports and roster filter rows), and a
// `w-full` popup inherits that 13rem — then daisyUI's `menu` lays each row out
// as a flex ROW, so label and sublabel truncate while splitting it. That cuts
// exactly the half that disambiguates: two of a customer's locations both render as
// "Brightpath…". So the list sizes to its content (`w-max`, floored at the
// input, capped below the viewport) and each row stacks label over sublabel,
// giving both the full row width. `title` on every row is the last resort for
// names still too long to fit the cap.
//
// The explicit `flex` on the row is load-bearing: daisyUI's menu lays each item
// out as `display: grid; grid-auto-flow: column`, so `flex-col` on its own is
// inert (grid ignores flex-direction) and the two spans stay side by side.
//
// It is a custom control standing in for <select>, so it carries the ARIA
// combobox contract by hand: the input owns the listbox, and the highlighted
// row is announced through `aria-activedescendant` rather than by moving focus
// (focus must stay in the input for typing to keep working).
import { computed, ref, useId, watch } from 'vue'

export interface SelectOption {
  id: string
  label: string
  sublabel?: string
}

const props = withDefaults(
  defineProps<{
    modelValue: string
    options: SelectOption[]
    placeholder?: string
    // When set, an explicit "none" choice with this label is offered and a
    // clear (✕) affordance shows while something is selected.
    emptyLabel?: string
    disabled?: boolean
    size?: 'sm' | 'md'
    // When set, and the typed query matches no existing option, a
    // "＋ {createLabel} “query”" row is offered; picking it emits `create`
    // with the trimmed query so the parent can mint the record and select it.
    createLabel?: string
  }>(),
  { placeholder: 'Type to search…', emptyLabel: '', disabled: false, size: 'md', createLabel: '' },
)

const emit = defineEmits<{
  (e: 'update:modelValue', v: string): void
  (e: 'create', label: string): void
}>()

const open = ref(false)
const query = ref('')
const highlighted = ref(0)
const inputEl = ref<HTMLInputElement | null>(null)
const listEl = ref<HTMLUListElement | null>(null)

// Keep the keyboard highlight inside the scrolled list. Without this, arrowing
// past the sixth option moves an invisible highlight — the list stays parked at
// the top and Enter picks a row you never saw. Nudging the popup's own
// scrollTop rather than calling scrollIntoView keeps the page itself still, and
// the offsets come from bounding rects because daisyUI's menu makes each `li`
// positioned, so the anchor's `offsetTop` is 0 relative to its own row.
watch(highlighted, (i) => {
  const ul = listEl.value
  const el = ul?.querySelector<HTMLElement>(`[data-idx="${i}"]`)
  if (!ul || !el) return
  const list = ul.getBoundingClientRect()
  const row = el.getBoundingClientRect()
  if (row.top < list.top) ul.scrollTop -= list.top - row.top
  else if (row.bottom > list.bottom) ul.scrollTop += row.bottom - list.bottom
})

const selected = computed(() => props.options.find((o) => o.id === props.modelValue) || null)

// Stable per-instance ids so the input can point at its own listbox and rows.
const uid = useId()
const listId = `${uid}-list`
const optionId = (i: number) => `${uid}-opt-${i}`
// Only claim an active descendant when the row it names is actually rendered —
// a dangling reference is worse for a screen reader than none at all.
const activeId = computed(() => {
  if (!open.value) return undefined
  if (highlighted.value < 0) return hasEmptyRow.value ? optionId(-1) : undefined
  return filtered.value[highlighted.value] ? optionId(highlighted.value) : undefined
})

// What the input shows: the live query while the list is open, otherwise
// the current selection's label.
const display = computed(() => (open.value ? query.value : selected.value?.label || ''))

const MAX_SHOWN = 50
const filtered = computed(() => {
  const q = query.value.trim().toLowerCase()
  const matches = q
    ? props.options.filter(
        (o) => o.label.toLowerCase().includes(q) || o.sublabel?.toLowerCase().includes(q),
      )
    : props.options
  return matches.slice(0, MAX_SHOWN)
})

// The explicit "none" row sits above the options, so it is index -1. It is only
// offered on an empty query, and typing must therefore pull the highlight back
// onto a real option rather than leave it parked on a row that no longer exists.
const hasEmptyRow = computed(() => !!props.emptyLabel && !query.value.trim())

watch(filtered, () => {
  if (highlighted.value >= filtered.value.length) highlighted.value = 0
})
watch(hasEmptyRow, (has) => {
  if (!has && highlighted.value < 0) highlighted.value = 0
})

// Offer to create when a non-empty query matches no existing option's label.
const canCreate = computed(() => {
  const q = query.value.trim()
  if (!props.createLabel || !q) return false
  return !props.options.some((o) => o.label.toLowerCase() === q.toLowerCase())
})

function doCreate() {
  emit('create', query.value.trim())
  close()
  inputEl.value?.blur()
}

// The list is 15rem tall and used to always open downward, which put it off the
// bottom of the window for the lower pickers on a tall form (the ticket form's
// thing picker starts below the fold outright). Measured once at open time —
// the list only flips up when there is genuinely more room above.
const LIST_HEIGHT = 240
const dropUp = ref(false)

function openList() {
  if (props.disabled) return
  const r = inputEl.value?.getBoundingClientRect()
  dropUp.value = !!r && window.innerHeight - r.bottom < LIST_HEIGHT && r.top > window.innerHeight - r.bottom
  open.value = true
  query.value = ''
  highlighted.value = 0
}

function close() {
  open.value = false
  query.value = ''
}

function choose(id: string) {
  emit('update:modelValue', id)
  close()
  inputEl.value?.blur()
}

function onKeydown(e: KeyboardEvent) {
  if (!open.value && (e.key === 'ArrowDown' || e.key === 'Enter')) {
    e.preventDefault()
    openList()
    return
  }
  if (!open.value) return
  switch (e.key) {
    case 'ArrowDown':
      e.preventDefault()
      highlighted.value = Math.min(highlighted.value + 1, filtered.value.length - 1)
      break
    case 'ArrowUp':
      e.preventDefault()
      highlighted.value = Math.max(highlighted.value - 1, hasEmptyRow.value ? -1 : 0)
      break
    case 'Enter':
      e.preventDefault()
      if (highlighted.value < 0) choose('')
      else if (filtered.value[highlighted.value]) choose(filtered.value[highlighted.value].id)
      else if (canCreate.value) doCreate()
      break
    case 'Escape':
      close()
      inputEl.value?.blur()
      break
  }
}
</script>

<template>
  <div class="relative">
    <div class="relative">
      <input
        ref="inputEl"
        type="text"
        class="input input-bordered w-full pr-8 text-ellipsis"
        :class="size === 'sm' ? 'input-sm' : ''"
        :value="display"
        :title="open ? '' : selected?.label || ''"
        :placeholder="selected ? selected.label : placeholder"
        :disabled="disabled"
        autocomplete="off"
        role="combobox"
        aria-autocomplete="list"
        :aria-expanded="open"
        :aria-controls="open ? listId : undefined"
        :aria-activedescendant="activeId"
        @focus="openList"
        @blur="close"
        @input="query = ($event.target as HTMLInputElement).value; open = true"
        @keydown="onKeydown"
      />
      <button
        v-if="emptyLabel && modelValue && !open"
        type="button"
        class="absolute right-1 top-1/2 -translate-y-1/2 btn btn-ghost btn-xs px-1.5"
        :disabled="disabled"
        tabindex="-1"
        :aria-label="emptyLabel"
        @mousedown.prevent="choose('')"
      >✕</button>
      <span v-else aria-hidden="true" class="absolute right-2.5 top-1/2 -translate-y-1/2 text-base-content/40 pointer-events-none text-xs">▾</span>
    </div>

    <ul
      v-if="open"
      ref="listEl"
      :id="listId"
      role="listbox"
      class="absolute z-30 min-w-full w-max max-w-[min(22rem,calc(100vw-2rem))] max-h-60 overflow-y-auto menu menu-sm p-1 bg-base-100 rounded-box shadow-lg border border-base-300 flex-nowrap"
      :class="dropUp ? 'bottom-full mb-1' : 'top-full mt-1'"
    >
      <li v-if="hasEmptyRow" role="presentation">
        <a
          class="italic opacity-70"
          data-idx="-1"
          role="option"
          :id="optionId(-1)"
          :aria-selected="!modelValue"
          :class="{ active: highlighted === -1 }"
          @mousedown.prevent="choose('')"
          @mousemove="highlighted = -1"
        >{{ emptyLabel }}</a>
      </li>
      <li v-for="(o, i) in filtered" :key="o.id" role="presentation">
        <a
          class="flex flex-col items-start gap-0"
          :data-idx="i"
          role="option"
          :id="optionId(i)"
          :aria-selected="o.id === modelValue"
          :class="{ active: i === highlighted, 'font-medium': o.id === modelValue }"
          :title="o.sublabel ? `${o.label} — ${o.sublabel}` : o.label"
          @mousedown.prevent="choose(o.id)"
          @mousemove="highlighted = i"
        >
          <span class="w-full truncate">{{ o.label }}</span>
          <!-- Dimmed with opacity, NOT with a `text-base-content/50` colour.
               daisyUI's `.active` row paints itself neutral and sets the text to
               neutral-content; a hardcoded base-content overrides that and, in
               the light theme where both neutral and base-content are dark, the
               sublabel vanishes into the highlight. Dark mode hid the bug —
               there both colours are light. Inheriting keeps it legible in every
               theme and in both states. -->
          <span v-if="o.sublabel" class="w-full truncate text-xs opacity-60">{{ o.sublabel }}</span>
        </a>
      </li>
      <li v-if="filtered.length === 0 && !canCreate" class="p-2 text-sm text-base-content/50">No matches.</li>
      <li v-if="canCreate">
        <a class="text-primary" @mousedown.prevent="doCreate">
          <span class="truncate">＋ {{ createLabel }} “{{ query.trim() }}”</span>
        </a>
      </li>
      <li v-if="options.length > MAX_SHOWN && filtered.length === MAX_SHOWN" class="p-2 text-xs text-base-content/40">
        Keep typing to narrow…
      </li>
    </ul>
  </div>
</template>
