<script setup lang="ts">
// Category chip: same soft geometry as the status/priority badges, tinted
// with the category's own color. The tint is mixed against the theme's base
// surface (--b1) rather than laid on as a low-alpha overlay, so it stays
// visible in dark mode; the color also drives the leading dot and the label.
// Empty name renders nothing; missing color falls back to the text color.
defineProps<{ name?: string; color?: string }>()
</script>

<template>
  <span
    v-if="name"
    class="badge-soft"
    :style="color ? { background: `color-mix(in srgb, ${color} 16%, oklch(var(--b1)))`, color } : {}"
  >
    <span class="badge-dot" :style="{ backgroundColor: color || 'currentColor' }"></span>
    <span class="truncate">{{ name }}</span>
  </span>
</template>
