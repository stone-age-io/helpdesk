<script setup lang="ts">
// One visit row, shared by the field Today list and the weekly Schedule agenda
// so the two can't drift (they had). Shows time · duration, ticket #/title,
// customer · site, status, and an optional live-timer marker; emits `select`
// with the visit id on tap. Needs the visit loaded with
// expand=ticket,ticket.customer,ticket.location.
import type { Visit, VisitStatus } from '@/types'
import { format } from 'date-fns'

defineProps<{ visit: Visit; timing?: boolean }>()
const emit = defineEmits<{ select: [id: string] }>()

const statusClass: Record<VisitStatus, string> = {
  requested: 'badge-soft-warning',
  scheduled: 'badge-soft-info',
  completed: 'badge-soft-success',
  canceled: 'badge-soft-neutral',
}
const fmtTime = (v: Visit) => (v.scheduled_at ? format(new Date(v.scheduled_at), 'HH:mm') : '—')
function fmtDuration(min?: number): string {
  if (!min) return ''
  const h = Math.floor(min / 60)
  const m = min % 60
  return h > 0 ? (m ? `${h}h ${m}m` : `${h}h`) : `${m}m`
}
</script>

<template>
  <button
    class="w-full flex gap-3 items-start rounded-2xl border p-3 text-left bg-base-100 hover:bg-base-200/50 transition-colors"
    :class="timing ? 'border-success' : 'border-base-300'"
    @click="emit('select', visit.id)"
  >
    <div class="text-center shrink-0 w-12">
      <div class="font-semibold">{{ fmtTime(visit) }}</div>
      <div class="text-[11px] text-base-content/50">{{ fmtDuration(visit.duration_minutes) }}</div>
    </div>
    <div class="flex-1 min-w-0">
      <div class="font-medium truncate">
        <span class="font-mono text-base-content/60">#{{ visit.expand?.ticket?.number }}</span>
        {{ visit.expand?.ticket?.title }}
      </div>
      <div class="text-xs text-base-content/60 truncate">
        {{ visit.expand?.ticket?.expand?.customer?.name }}
        <template v-if="visit.expand?.ticket?.expand?.location?.name"> · {{ visit.expand.ticket.expand.location.name }}</template>
      </div>
      <div class="mt-1 flex items-center gap-2">
        <span class="badge-soft" :class="statusClass[visit.status]">{{ visit.status }}</span>
        <span v-if="timing" class="inline-flex items-center gap-1 text-xs text-success">
          <span class="inline-flex h-2 w-2 rounded-full bg-success animate-pulse"></span> timing now
        </span>
      </div>
    </div>
    <span class="text-base-content/30 self-center" aria-hidden="true">›</span>
  </button>
</template>
