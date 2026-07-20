<script setup lang="ts">
// Compact billable indicator + inline toggle for one time entry. Billable is
// the default, so a billable entry shows only a faint marker while a
// non-billable one shows a prominent badge — the list stays quiet and only the
// exceptions stand out. When editable (own entry or admin) clicking flips it in
// place, so a mis-flag is fixed without delete + re-add. Self-contained: it
// writes the change and emits `changed` for the parent to reload.
import { pb } from '@/pb'
import type { TimeEntry } from '@/types'
import { ref } from 'vue'

const props = defineProps<{ entry: TimeEntry; editable?: boolean }>()
const emit = defineEmits<{ changed: [] }>()
const busy = ref(false)

async function toggle() {
  if (!props.editable || busy.value) return
  busy.value = true
  try {
    await pb.collection('time_entries').update(props.entry.id, { non_billable: !props.entry.non_billable })
    emit('changed')
  } catch {
    // Non-critical; the list stays as-is on failure.
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <button
    v-if="editable"
    type="button"
    class="shrink-0 whitespace-nowrap"
    :class="entry.non_billable
      ? 'badge-soft badge-soft-warning text-xs cursor-pointer'
      : 'text-xs text-base-content/30 hover:text-base-content/70'"
    :disabled="busy"
    :title="entry.non_billable ? 'Non-billable — click to make billable' : 'Billable — click to mark non-billable'"
    @click.stop="toggle"
  >{{ entry.non_billable ? 'non-billable' : '$' }}</button>
  <span
    v-else-if="entry.non_billable"
    class="badge-soft badge-soft-warning text-xs shrink-0 whitespace-nowrap"
    title="Non-billable"
  >non-billable</span>
</template>
