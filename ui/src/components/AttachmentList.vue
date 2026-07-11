<script setup lang="ts">
import { computed } from 'vue'
import { pb } from '@/pb'

// Renders a record's file-field attachments as a row of links; image files
// get an inline preview. `record` must be a real PocketBase record (carries
// collectionId + id so the SDK can build the file URL); `field` names the
// file field (defaults to "attachments").
const props = withDefaults(
  defineProps<{ record: Record<string, any>; files?: string[]; field?: string }>(),
  { field: 'attachments' },
)

const names = computed<string[]>(() => props.files ?? props.record?.[props.field] ?? [])
const isImage = (name: string) => /\.(png|jpe?g|gif|webp|bmp|svg)$/i.test(name)
const url = (name: string) => pb.files.getURL(props.record, name)
// Files carry a random suffix (…_a1b2c3.png); show the human part.
const label = (name: string) => name.replace(/_[a-z0-9]{10}(\.[^.]+)?$/i, '$1')
</script>

<template>
  <div v-if="names.length" class="flex flex-wrap gap-2 mt-1">
    <a
      v-for="name in names"
      :key="name"
      :href="url(name)"
      target="_blank"
      rel="noopener"
      class="block"
      :title="label(name)"
    >
      <img
        v-if="isImage(name)"
        :src="url(name)"
        :alt="label(name)"
        class="max-h-28 max-w-[10rem] rounded border border-base-300 object-cover"
      />
      <span v-else class="badge badge-outline gap-1 max-w-[12rem] truncate">📎 {{ label(name) }}</span>
    </a>
  </div>
</template>
