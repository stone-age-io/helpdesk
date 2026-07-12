<script setup lang="ts">
// Month overview: a 6×7 grid of day cells showing a few visit chips each.
// Deliberately low-resolution (no utilization) — the zoom-out companion to the
// week board. Clicking a chip opens the drawer; clicking a day jumps to its
// week. Presentational: the parent owns the visit data and navigation.
import { computed } from 'vue'
import type { Visit } from '@/types'
import VisitChip from '@/components/VisitChip.vue'
import {
  eachDayOfInterval,
  endOfMonth,
  endOfWeek,
  format,
  isSameDay,
  isSameMonth,
  isToday,
  startOfMonth,
  startOfWeek,
} from 'date-fns'

const props = defineProps<{ visits: Visit[]; focusDate: Date }>()
const emit = defineEmits<{ select: [id: string]; openDay: [date: Date]; prev: []; next: []; today: [] }>()

const days = computed(() =>
  eachDayOfInterval({
    start: startOfWeek(startOfMonth(props.focusDate), { weekStartsOn: 1 }),
    end: endOfWeek(endOfMonth(props.focusDate), { weekStartsOn: 1 }),
  }),
)
const weekdayLabels = ['Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat', 'Sun']

function visitsOn(day: Date): Visit[] {
  return props.visits
    .filter((v) => v.scheduled_at && isSameDay(new Date(v.scheduled_at), day))
    .sort((a, b) => (a.scheduled_at || '').localeCompare(b.scheduled_at || ''))
}
</script>

<template>
  <div class="space-y-3">
    <!-- Month navigator -->
    <div class="flex items-center gap-2 flex-wrap">
      <div class="join">
        <button class="btn btn-sm join-item" title="Previous month" @click="emit('prev')">‹</button>
        <button class="btn btn-sm join-item" @click="emit('today')">Today</button>
        <button class="btn btn-sm join-item" title="Next month" @click="emit('next')">›</button>
      </div>
      <span class="font-medium">{{ format(focusDate, 'MMMM yyyy') }}</span>
    </div>

    <div class="overflow-x-auto">
      <div class="min-w-[56rem]">
        <div class="grid grid-cols-7 gap-2 mb-1">
          <div v-for="w in weekdayLabels" :key="w" class="text-center text-xs uppercase tracking-wide text-base-content/50">{{ w }}</div>
        </div>
        <div class="grid grid-cols-7 gap-2">
          <div
            v-for="day in days"
            :key="day.toISOString()"
            class="rounded-box border border-base-300 min-h-24 p-1 flex flex-col"
            :class="isSameMonth(day, focusDate) ? '' : 'bg-base-200/40'"
          >
            <button
              type="button"
              class="self-start text-xs px-1 rounded hover:bg-base-200"
              :class="isToday(day) ? 'text-primary font-semibold' : isSameMonth(day, focusDate) ? '' : 'text-base-content/40'"
              title="View this week"
              @click="emit('openDay', day)"
            >
              {{ format(day, 'd') }}
            </button>
            <div class="space-y-1 mt-1">
              <VisitChip v-for="v in visitsOn(day).slice(0, 3)" :key="v.id" :visit="v" @select="(id) => emit('select', id)" />
              <button
                v-if="visitsOn(day).length > 3"
                type="button"
                class="w-full text-left text-xs text-base-content/60 hover:text-base-content px-1"
                @click="emit('openDay', day)"
              >
                +{{ visitsOn(day).length - 3 }} more
              </button>
            </div>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>
