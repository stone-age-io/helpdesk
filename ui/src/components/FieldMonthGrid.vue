<script setup lang="ts">
// Field month view: a fits-width 7-column grid (aspect-square cells) showing a
// day number + up to three status dots — the mobile-native counterpart to the
// desk VisitMonthCalendar (which needs 896px). Tapping a day asks the parent to
// drill into that day's week. Presentational.
import { computed } from 'vue'
import type { Visit } from '@/types'
import { eachDayOfInterval, endOfMonth, endOfWeek, format, isSameDay, isSameMonth, isToday, startOfMonth, startOfWeek } from 'date-fns'

const props = defineProps<{ visits: Visit[]; focusDate: Date }>()
const emit = defineEmits<{ openDay: [date: Date] }>()

const days = computed(() =>
  eachDayOfInterval({
    start: startOfWeek(startOfMonth(props.focusDate), { weekStartsOn: 1 }),
    end: endOfWeek(endOfMonth(props.focusDate), { weekStartsOn: 1 }),
  }),
)
const weekdayLabels = ['M', 'T', 'W', 'T', 'F', 'S', 'S']

function visitsOn(day: Date): Visit[] {
  return props.visits.filter((v) => v.scheduled_at && isSameDay(new Date(v.scheduled_at), day))
}
// completed = success dot, otherwise the primary accent.
function dotClass(v: Visit): string {
  return v.status === 'completed' ? 'bg-success' : 'bg-primary'
}
</script>

<template>
  <div>
    <div class="grid grid-cols-7 gap-1 mb-1">
      <div v-for="(l, i) in weekdayLabels" :key="i" class="text-center text-[10px] uppercase tracking-wide text-base-content/40">{{ l }}</div>
    </div>
    <div class="grid grid-cols-7 gap-1">
      <button
        v-for="day in days"
        :key="day.toISOString()"
        type="button"
        class="aspect-square rounded-lg border p-1 flex flex-col items-center gap-1 text-xs hover:bg-base-200/50 transition-colors"
        :class="[
          isToday(day) ? 'border-primary text-primary font-semibold' : 'border-base-300',
          isSameMonth(day, focusDate) ? '' : 'opacity-40',
        ]"
        @click="emit('openDay', day)"
      >
        <span>{{ format(day, 'd') }}</span>
        <span v-if="visitsOn(day).length" class="flex flex-wrap justify-center gap-0.5">
          <span v-for="(v, i) in visitsOn(day).slice(0, 3)" :key="i" class="h-1.5 w-1.5 rounded-full" :class="dotClass(v)"></span>
        </span>
      </button>
    </div>
  </div>
</template>
