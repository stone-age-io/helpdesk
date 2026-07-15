<script setup lang="ts">
defineProps<{ active: boolean }>()
</script>

<template>
  <span class="badge-soft" :class="active ? 'badge-soft-success' : 'badge-soft-neutral'">
    <span class="badge-dot"></span>{{ active ? 'active' : 'inactive' }}
  </span>
</template>
