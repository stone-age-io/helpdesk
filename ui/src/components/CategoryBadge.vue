<script setup lang="ts">
// Soft category chip: a colored dot + name over a low-alpha tint of the
// category's color, so it stays legible in both light and dark themes. The
// color is a hex string stored on the category; empty falls back to neutral.
const props = defineProps<{ name?: string; color?: string }>()

// 8-bit alpha suffix on the hex → ~13% tint for the background.
const bg = props.color ? props.color + '22' : undefined
</script>

<template>
  <span
    v-if="name"
    class="badge badge-sm gap-1 border-transparent font-medium"
    :style="color ? { backgroundColor: bg, color: color } : {}"
  >
    <span class="w-1.5 h-1.5 rounded-full shrink-0" :style="{ backgroundColor: color || 'currentColor' }"></span>
    <span class="truncate">{{ name }}</span>
  </span>
</template>
