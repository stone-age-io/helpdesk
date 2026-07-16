<script setup lang="ts">
// Small numeric input for a duration stored in MINUTES. Staff toggle between
// entering plain minutes or (decimal) hours; the component always emits minutes,
// so callers never deal with the unit. Storage stays minutes everywhere
// (time_entries.minutes, tickets.estimated_minutes) — this is input sugar only.
//
// Used by the WorkCard manual time-log and the ticket "Estimated effort" field.
import { ref, watch } from 'vue'

const props = withDefaults(
  defineProps<{
    modelValue: number | null
    size?: 'sm' | 'xs'
    placeholder?: string
    disabled?: boolean
    // Floor applied to any positive entry (minutes). Matches the DB Min:1.
    min?: number
  }>(),
  { modelValue: null, size: 'sm', placeholder: 'min', disabled: false, min: 1 },
)
const emit = defineEmits<{ 'update:modelValue': [number | null] }>()

const unit = ref<'min' | 'hr'>('min')
const text = ref('')

// minutes → the text shown for the current unit (hours trimmed to 2 decimals).
function toText(mins: number | null): string {
  if (mins == null) return ''
  if (unit.value === 'hr') return String(Math.round((mins / 60) * 100) / 100)
  return String(mins)
}

// current field text → minutes (null if blank/invalid), clamped to `min`.
function textToMinutes(): number | null {
  const n = parseFloat(text.value)
  if (isNaN(n) || n <= 0) return null
  const mins = unit.value === 'hr' ? Math.round(n * 60) : Math.round(n)
  return mins < props.min ? props.min : mins
}

// Sync from outside — but don't clobber an in-progress entry (e.g. "1." while
// typing 1.5) when the incoming value is just our own emit round-tripping.
watch(
  () => props.modelValue,
  (v) => {
    if (v === textToMinutes()) return
    text.value = toText(v)
  },
  { immediate: true },
)

function switchUnit(u: 'min' | 'hr') {
  if (u === unit.value) return
  unit.value = u
  text.value = toText(props.modelValue)
}

function onInput() {
  emit('update:modelValue', textToMinutes())
}
</script>

<template>
  <div class="join">
    <input
      v-model="text"
      type="number"
      min="0"
      step="any"
      :placeholder="placeholder"
      :disabled="disabled"
      class="input input-bordered join-item w-16"
      :class="size === 'xs' ? 'input-xs' : 'input-sm'"
      @input="onInput"
    />
    <button
      type="button"
      class="btn join-item"
      :class="[size === 'xs' ? 'btn-xs' : 'btn-sm', unit === 'min' ? 'btn-primary' : 'btn-ghost']"
      :disabled="disabled"
      @click="switchUnit('min')"
    >min</button>
    <button
      type="button"
      class="btn join-item"
      :class="[size === 'xs' ? 'btn-xs' : 'btn-sm', unit === 'hr' ? 'btn-primary' : 'btn-ghost']"
      :disabled="disabled"
      @click="switchUnit('hr')"
    >hr</button>
  </div>
</template>
