<script setup lang="ts">
import type { TicketPriority, TicketStatus } from '@/types'

defineProps<{
  status?: TicketStatus
  priority?: TicketPriority
}>()

// Soft-badge variant per status/priority (see .badge-soft-* in style.css).
// Status chips get a leading state dot; priority chips are dot-less, so
// low/normal read as quiet neutrals while high/urgent carry warm/red weight.
const statusClass: Record<TicketStatus, string> = {
  open: 'badge-soft-info',
  in_progress: 'badge-soft-primary',
  waiting: 'badge-soft-warning',
  resolved: 'badge-soft-success',
  closed: 'badge-soft-neutral',
}

const priorityClass: Record<TicketPriority, string> = {
  low: 'badge-soft-neutral',
  normal: 'badge-soft-neutral',
  high: 'badge-soft-warning',
  urgent: 'badge-soft-error',
}
</script>

<template>
  <span v-if="status" class="badge-soft" :class="statusClass[status]">
    <span class="badge-dot"></span>{{ status.replace('_', ' ') }}
  </span>
  <span v-if="priority" class="badge-soft" :class="priorityClass[priority]">{{ priority }}</span>
</template>
