<script setup lang="ts">
// Field-only quick-pick for common on-site durations. Tapping a chip emits the
// minutes and lights up as active. Field agents shouldn't summon a keyboard for
// the 90% case — the desk WorkCard keeps the min/hr MinutesInput instead, so
// chips stay a field affordance (per the agreed split).
withDefaults(defineProps<{ modelValue: number | null; options?: number[]; disabled?: boolean }>(), {
  options: () => [15, 30, 45, 60, 90],
  disabled: false,
})
const emit = defineEmits<{ 'update:modelValue': [number] }>()

function label(min: number): string {
  if (min < 60) return `${min}m`
  return min % 60 ? `${(min / 60).toFixed(1)}h` : `${min / 60}h`
}
</script>

<template>
  <div class="flex flex-wrap gap-2">
    <button
      v-for="opt in options"
      :key="opt"
      type="button"
      class="btn btn-sm flex-1 min-w-[3.25rem]"
      :class="modelValue === opt ? 'btn-primary' : 'btn-outline'"
      :disabled="disabled"
      @click="emit('update:modelValue', opt)"
    >{{ label(opt) }}</button>
  </div>
</template>
