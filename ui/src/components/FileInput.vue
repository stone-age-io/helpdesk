<script setup lang="ts">
import { ref } from 'vue'

// Thin wrapper over <input type="file" multiple> with v-model:files (File[])
// and a chosen-file list. The parent passes the File[] straight to the
// PocketBase SDK, which sends multipart when it sees File values.
const props = defineProps<{ files: File[]; disabled?: boolean }>()
const emit = defineEmits<{ 'update:files': [File[]] }>()

const input = ref<HTMLInputElement | null>(null)

function onChange(e: Event) {
  const picked = Array.from((e.target as HTMLInputElement).files ?? [])
  emit('update:files', [...props.files, ...picked])
  if (input.value) input.value.value = '' // allow re-picking the same file
}

function removeAt(i: number) {
  emit(
    'update:files',
    props.files.filter((_, idx) => idx !== i),
  )
}
</script>

<template>
  <div class="space-y-1">
    <input
      ref="input"
      type="file"
      multiple
      class="file-input file-input-bordered file-input-sm w-full"
      :disabled="disabled"
      @change="onChange"
    />
    <ul v-if="files.length" class="flex flex-wrap gap-1">
      <li v-for="(f, i) in files" :key="i" class="badge badge-ghost gap-1">
        📎 <span class="max-w-[10rem] truncate">{{ f.name }}</span>
        <button type="button" class="text-error" :disabled="disabled" @click="removeAt(i)">✕</button>
      </li>
    </ul>
  </div>
</template>
