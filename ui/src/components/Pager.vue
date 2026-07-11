<script setup lang="ts">
// Compact prev/next pager shared across the list views. Renders nothing when
// there's a single page. Use with v-model:page.
defineProps<{ page: number; totalPages: number }>()
const emit = defineEmits<{ 'update:page': [n: number] }>()
</script>

<template>
  <div v-if="totalPages > 1" class="flex justify-center gap-2">
    <button class="btn btn-sm" :disabled="page <= 1" @click="emit('update:page', page - 1)">«</button>
    <span class="btn btn-sm btn-ghost no-animation">{{ page }} / {{ totalPages }}</span>
    <button class="btn btn-sm" :disabled="page >= totalPages" @click="emit('update:page', page + 1)">»</button>
  </div>
</template>
