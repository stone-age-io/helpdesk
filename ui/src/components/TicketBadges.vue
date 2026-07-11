<script setup lang="ts">
import type { TicketPriority, TicketStatus } from '@/types'

defineProps<{
  status?: TicketStatus
  priority?: TicketPriority
}>()

const statusClass: Record<TicketStatus, string> = {
  open: 'badge-info',
  in_progress: 'badge-primary',
  waiting: 'badge-warning',
  resolved: 'badge-success',
  closed: 'badge-ghost',
}

const priorityClass: Record<TicketPriority, string> = {
  low: 'badge-ghost',
  normal: 'badge-outline',
  high: 'badge-warning',
  urgent: 'badge-error',
}
</script>

<template>
  <span v-if="status" class="badge badge-sm" :class="statusClass[status]">{{ status.replace('_', ' ') }}</span>
  <span v-if="priority" class="badge badge-sm" :class="priorityClass[priority]">{{ priority }}</span>
</template>
