<script setup lang="ts">
// Field week view: seven day-sections stacked vertically — no horizontal scroll
// (the desk VisitWeekCalendar is a 896px 7-column board built for a wide
// screen). Presentational: the parent owns data + week navigation.
import { computed } from 'vue'
import type { Visit, VisitStatus } from '@/types'
import { addDays, format, isSameDay, isToday } from 'date-fns'

const props = defineProps<{ visits: Visit[]; weekStart: Date }>()
const emit = defineEmits<{ select: [id: string] }>()

const days = computed(() => Array.from({ length: 7 }, (_, i) => addDays(props.weekStart, i)))

function visitsOn(day: Date): Visit[] {
  return props.visits
    .filter((v) => v.scheduled_at && isSameDay(new Date(v.scheduled_at), day))
    .sort((a, b) => (a.scheduled_at || '').localeCompare(b.scheduled_at || ''))
}

const statusClass: Record<VisitStatus, string> = {
  requested: 'badge-soft-warning',
  scheduled: 'badge-soft-info',
  completed: 'badge-soft-success',
  canceled: 'badge-soft-neutral',
}
const fmtTime = (v: Visit) => (v.scheduled_at ? format(new Date(v.scheduled_at), 'HH:mm') : '—')
</script>

<template>
  <div class="space-y-3">
    <div v-for="day in days" :key="day.toISOString()">
      <div class="flex items-center gap-2 mb-1">
        <span class="text-sm font-semibold" :class="isToday(day) ? 'text-primary' : ''">{{ format(day, 'EEE') }}</span>
        <span class="text-xs text-base-content/50">{{ format(day, 'MMM d') }}</span>
        <span v-if="isToday(day)" class="badge-soft badge-soft-info text-[10px]">today</span>
      </div>
      <ul v-if="visitsOn(day).length" class="space-y-1">
        <li v-for="v in visitsOn(day)" :key="v.id">
          <button
            class="w-full flex items-center gap-2 rounded-lg border border-base-300 bg-base-100 p-2 text-left hover:bg-base-200/50 transition-colors"
            @click="emit('select', v.id)"
          >
            <span class="font-mono text-sm w-12 shrink-0">{{ fmtTime(v) }}</span>
            <span class="flex-1 min-w-0 truncate text-sm">
              <span class="font-mono text-base-content/50">#{{ v.expand?.ticket?.number }}</span>
              {{ v.expand?.ticket?.title }}
            </span>
            <span class="badge-soft shrink-0" :class="statusClass[v.status]">{{ v.status }}</span>
          </button>
        </li>
      </ul>
      <p v-else class="text-xs text-base-content/30 pl-1">—</p>
    </div>
  </div>
</template>
