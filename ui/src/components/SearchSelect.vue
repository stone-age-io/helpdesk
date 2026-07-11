<script setup lang="ts">
// Typeahead replacement for raw <select> on long lists (customers,
// requesters, staff): type a few characters, pick from the narrowed list.
// Options are provided by the parent (already-loaded records) — filtering
// is client-side, which is plenty at MSP scale.
import { computed, ref, watch } from 'vue'

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
  }>(),
  { placeholder: 'Type to search…', emptyLabel: '', disabled: false, size: 'md' },
)

const emit = defineEmits<{ (e: 'update:modelValue', v: string): void }>()

const open = ref(false)
const query = ref('')
const highlighted = ref(0)
const inputEl = ref<HTMLInputElement | null>(null)

const selected = computed(() => props.options.find((o) => o.id === props.modelValue) || null)

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

watch(filtered, () => {
  if (highlighted.value >= filtered.value.length) highlighted.value = 0
})

function openList() {
  if (props.disabled) return
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
      highlighted.value = Math.max(highlighted.value - 1, 0)
      break
    case 'Enter':
      e.preventDefault()
      if (filtered.value[highlighted.value]) choose(filtered.value[highlighted.value].id)
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
        class="input input-bordered w-full pr-8"
        :class="size === 'sm' ? 'input-sm' : ''"
        :value="display"
        :placeholder="selected ? selected.label : placeholder"
        :disabled="disabled"
        autocomplete="off"
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
        @mousedown.prevent="choose('')"
      >✕</button>
      <span v-else class="absolute right-2.5 top-1/2 -translate-y-1/2 text-base-content/40 pointer-events-none text-xs">▾</span>
    </div>

    <ul
      v-if="open"
      class="absolute z-30 mt-1 w-full max-h-60 overflow-y-auto menu menu-sm p-1 bg-base-100 rounded-box shadow-lg border border-base-300 flex-nowrap"
    >
      <li v-if="emptyLabel && !query.trim()">
        <a
          class="text-base-content/60 italic"
          :class="{ active: highlighted === -1 }"
          @mousedown.prevent="choose('')"
        >{{ emptyLabel }}</a>
      </li>
      <li v-for="(o, i) in filtered" :key="o.id">
        <a
          :class="{ active: i === highlighted, 'font-medium': o.id === modelValue }"
          @mousedown.prevent="choose(o.id)"
          @mousemove="highlighted = i"
        >
          <span class="truncate">{{ o.label }}</span>
          <span v-if="o.sublabel" class="text-xs text-base-content/50 truncate">{{ o.sublabel }}</span>
        </a>
      </li>
      <li v-if="filtered.length === 0" class="p-2 text-sm text-base-content/50">No matches.</li>
      <li v-if="options.length > MAX_SHOWN && filtered.length === MAX_SHOWN" class="p-2 text-xs text-base-content/40">
        Keep typing to narrow…
      </li>
    </ul>
  </div>
</template>
