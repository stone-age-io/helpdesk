<script setup lang="ts">
// Week scheduling board: seven day-columns of time-sorted visit chips, with a
// per-technician utilization readout above. Deliberately NOT an hourly grid —
// day-precision is low value at MSP grain; chips carry the time and duration.
// Presentational: the parent owns the visit data and week navigation.
import { computed } from 'vue'
import type { Staff, Visit } from '@/types'
import VisitChip from '@/components/VisitChip.vue'
import { addDays, format, isBefore, isSameDay, isToday, startOfDay } from 'date-fns'

const props = withDefaults(
  defineProps<{ visits: Visit[]; weekStart: Date; staff: Staff[]; capacityMinutes?: number }>(),
  { capacityMinutes: 2400 },
)
const emit = defineEmits<{ select: [id: string]; prev: []; next: []; today: [] }>()

const days = computed(() => Array.from({ length: 7 }, (_, i) => addDays(props.weekStart, i)))
const weekEnd = computed(() => addDays(props.weekStart, 6))

function visitsOn(day: Date): Visit[] {
  return props.visits
    .filter((v) => v.scheduled_at && isSameDay(new Date(v.scheduled_at), day))
    .sort((a, b) => (a.scheduled_at || '').localeCompare(b.scheduled_at || ''))
}

const staffName = (id: string) => props.staff.find((s) => s.id === id)?.name || 'Unknown'

// Committed minutes per technician for the visible week. Scheduled visits
// always carry an assignee (guard hook), so every hour attributes to someone;
// completed visits count too — they consumed the week's capacity.
const utilization = computed(() => {
  const byTech = new Map<string, number>()
  for (const v of props.visits) {
    if (!v.assignee) continue
    byTech.set(v.assignee, (byTech.get(v.assignee) || 0) + (v.duration_minutes || 0))
  }
  return [...byTech.entries()]
    .map(([id, minutes]) => ({ id, name: staffName(id), minutes }))
    .sort((a, b) => b.minutes - a.minutes)
})

function fmtHours(min: number): string {
  return `${(min / 60).toFixed(min % 60 ? 1 : 0)}h`
}
</script>

<template>
  <div class="space-y-3">
    <!-- Week navigator -->
    <div class="flex items-center gap-2 flex-wrap">
      <div class="join">
        <button class="btn btn-sm join-item" title="Previous week" @click="emit('prev')">‹</button>
        <button class="btn btn-sm join-item" @click="emit('today')">Today</button>
        <button class="btn btn-sm join-item" title="Next week" @click="emit('next')">›</button>
      </div>
      <span class="font-medium">{{ format(weekStart, 'MMM d') }} – {{ format(weekEnd, 'MMM d, yyyy') }}</span>
    </div>

    <!-- Per-technician utilization for the week -->
    <div v-if="utilization.length" class="rounded-box border border-base-300 p-3">
      <div class="text-xs uppercase tracking-wide text-base-content/60 mb-2">Utilization · {{ Math.round(capacityMinutes / 60) }}h/wk</div>
      <div class="grid gap-x-4 gap-y-2 sm:grid-cols-2 lg:grid-cols-3">
        <div v-for="u in utilization" :key="u.id" class="flex items-center gap-2 text-sm">
          <span class="w-28 truncate" :title="u.name">{{ u.name }}</span>
          <progress
            class="progress flex-1"
            :class="u.minutes > capacityMinutes ? 'progress-error' : 'progress-success'"
            :value="Math.min(u.minutes, capacityMinutes)"
            :max="capacityMinutes"
          ></progress>
          <span class="w-12 text-right tabular-nums" :class="u.minutes > capacityMinutes ? 'text-error font-medium' : 'text-base-content/70'">{{ fmtHours(u.minutes) }}</span>
        </div>
      </div>
    </div>

    <!-- Seven-day grid; scrolls horizontally on narrow screens. -->
    <div class="overflow-x-auto">
      <div class="grid grid-cols-7 gap-2 min-w-[56rem]">
        <div v-for="day in days" :key="day.toISOString()" class="rounded-box border border-base-300 min-h-32 flex flex-col overflow-hidden">
          <div
            class="px-2 py-1 text-center border-b border-base-300"
            :class="isToday(day) ? 'bg-primary/10 text-primary font-semibold' : isBefore(day, startOfDay(new Date())) ? 'text-base-content/40' : ''"
          >
            <div class="text-sm font-medium">{{ format(day, 'EEE') }}</div>
            <div class="text-xs">{{ format(day, 'MMM d') }}</div>
          </div>
          <div class="p-1 space-y-1 flex-1">
            <VisitChip v-for="v in visitsOn(day)" :key="v.id" :visit="v" show-tech @select="(id) => emit('select', id)" />
          </div>
        </div>
      </div>
    </div>
  </div>
</template>
