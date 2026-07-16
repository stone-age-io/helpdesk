<script setup lang="ts">
// Field week view: seven day-sections stacked vertically — no horizontal scroll
// (the desk VisitWeekCalendar is a 896px 7-column board built for a wide
// screen). Rows use the same FieldVisitCard as Today so the two stay in sync.
// Presentational: the parent owns data + week navigation.
import { computed } from 'vue'
import type { Visit } from '@/types'
import FieldVisitCard from '@/components/FieldVisitCard.vue'
import { addDays, format, isSameDay, isToday } from 'date-fns'

const props = defineProps<{ visits: Visit[]; weekStart: Date }>()
const emit = defineEmits<{ select: [id: string] }>()

const days = computed(() => Array.from({ length: 7 }, (_, i) => addDays(props.weekStart, i)))

function visitsOn(day: Date): Visit[] {
  return props.visits
    .filter((v) => v.scheduled_at && isSameDay(new Date(v.scheduled_at), day))
    .sort((a, b) => (a.scheduled_at || '').localeCompare(b.scheduled_at || ''))
}
</script>

<template>
  <div class="space-y-3">
    <div v-for="day in days" :key="day.toISOString()">
      <div class="flex items-center gap-2 mb-1">
        <span class="text-sm font-semibold" :class="isToday(day) ? 'text-primary' : ''">{{ format(day, 'EEE') }}</span>
        <span class="text-xs text-base-content/50">{{ format(day, 'MMM d') }}</span>
        <span v-if="isToday(day)" class="badge-soft badge-soft-info text-[10px]">today</span>
      </div>
      <ul v-if="visitsOn(day).length" class="space-y-2">
        <li v-for="v in visitsOn(day)" :key="v.id">
          <FieldVisitCard :visit="v" @select="(id) => emit('select', id)" />
        </li>
      </ul>
      <p v-else class="text-xs text-base-content/30 pl-1">—</p>
    </div>
  </div>
</template>
