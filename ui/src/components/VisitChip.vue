<script setup lang="ts">
// One scheduled visit as a compact card, shared by the week and month
// calendars. The left border is tinted by ticket priority so an urgent visit
// pops; completed visits dim. Purely presentational — click emits the id and
// the parent opens the drawer.
import type { Visit } from '@/types'
import { format } from 'date-fns'

const props = defineProps<{ visit: Visit; showTech?: boolean }>()
const emit = defineEmits<{ select: [id: string] }>()

const priorityBorder: Record<string, string> = {
  urgent: 'border-error',
  high: 'border-warning',
  normal: 'border-primary',
  low: 'border-base-300',
}

function fmtDuration(min?: number): string {
  if (!min) return ''
  const h = Math.floor(min / 60)
  const m = min % 60
  return h > 0 ? (m ? `${h}h${m}m` : `${h}h`) : `${m}m`
}
</script>

<template>
  <button
    type="button"
    class="w-full text-left rounded border-l-4 bg-base-200/60 hover:bg-base-200 px-2 py-1 text-xs leading-tight transition-colors"
    :class="[
      priorityBorder[props.visit.expand?.ticket?.priority] || 'border-base-300',
      props.visit.status === 'completed' ? 'opacity-50' : '',
    ]"
    @click="emit('select', props.visit.id)"
  >
    <div class="flex items-baseline gap-1">
      <span class="font-semibold whitespace-nowrap">{{ props.visit.scheduled_at ? format(new Date(props.visit.scheduled_at), 'HH:mm') : '' }}</span>
      <span v-if="props.visit.duration_minutes" class="text-base-content/50 whitespace-nowrap">{{ fmtDuration(props.visit.duration_minutes) }}</span>
      <span class="font-mono text-base-content/50">#{{ props.visit.expand?.ticket?.number }}</span>
    </div>
    <div class="truncate">{{ props.visit.expand?.ticket?.title }}</div>
    <div v-if="showTech && props.visit.expand?.assignee?.name" class="truncate text-base-content/60">{{ props.visit.expand?.assignee?.name }}</div>
  </button>
</template>
