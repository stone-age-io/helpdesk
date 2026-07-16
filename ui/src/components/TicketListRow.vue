<script setup lang="ts">
// One ticket summary row for the dashboard / detail "recent tickets" panels
// (staff + portal). Kept in one place so the staff dashboard, portal dashboard,
// and project ticket lists can't drift. Breathes on mobile: identity on its own
// line with the badges (and optional age) beneath; collapses to a single row on
// sm+. `showCustomer` needs `customer` expanded on the ticket.
import type { Ticket } from '@/types'
import TicketBadges from '@/components/TicketBadges.vue'
import { formatDistanceToNow } from 'date-fns'

defineProps<{
  ticket: Ticket
  to: string
  showCustomer?: boolean
  showAge?: boolean
}>()
</script>

<template>
  <router-link
    :to="to"
    class="flex flex-col gap-1 py-2.5 -mx-2 px-2 rounded hover:bg-base-200/50 sm:flex-row sm:items-center sm:gap-3"
  >
    <div class="flex items-center gap-2 min-w-0 flex-1">
      <span class="font-mono text-xs text-base-content/50 shrink-0">#{{ ticket.number }}</span>
      <span class="truncate">{{ ticket.title }}</span>
    </div>
    <div class="flex items-center gap-2 pl-6 sm:pl-0">
      <span
        v-if="showCustomer && ticket.expand?.customer?.name"
        class="hidden sm:block text-xs text-base-content/60 truncate max-w-[9rem]"
      >{{ ticket.expand.customer.name }}</span>
      <TicketBadges :status="ticket.status" :priority="ticket.priority" />
      <span
        v-if="showAge"
        class="ml-auto sm:ml-0 text-xs text-base-content/50 whitespace-nowrap"
      >{{ formatDistanceToNow(new Date(ticket.created)) }} ago</span>
    </div>
  </router-link>
</template>
