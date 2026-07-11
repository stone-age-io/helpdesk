<script setup lang="ts">
defineProps<{ active: boolean }>()
</script>

<template>
  <span class="badge badge-sm" :class="active ? 'badge-success' : 'badge-ghost'">
    {{ active ? 'active' : 'inactive' }}
  </span>
</template>
