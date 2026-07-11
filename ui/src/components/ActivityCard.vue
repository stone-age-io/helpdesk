<script setup lang="ts">
import { onMounted, onUnmounted, ref } from 'vue'
import { pb } from '@/pb'
import type { TicketEvent } from '@/types'
import { formatDistanceToNow } from 'date-fns'

// Read-only audit timeline for one ticket: who changed status / priority /
// assignee, and when. Fed by ticket_events (staff-only), written server-side
// by internal/activity.
const props = defineProps<{ ticketId: string }>()

const events = ref<TicketEvent[]>([])

async function load() {
  try {
    events.value = await pb.collection('ticket_events').getFullList<TicketEvent>({
      filter: `ticket = '${props.ticketId}'`,
      sort: '-created',
      expand: 'actor_staff,actor_user',
    })
  } catch {
    // Optional context card.
  }
}

function actor(e: TicketEvent): string {
  return e.expand?.actor_staff?.name || e.expand?.actor_user?.name || e.expand?.actor_user?.email || 'System'
}
const fieldLabel: Record<string, string> = { status: 'status', priority: 'priority', assignee: 'assignee' }

// Live: any audited change on any ticket refreshes; the filtered reload keeps
// it to this ticket's events.
let timer: ReturnType<typeof setTimeout> | undefined
let unsub: (() => void) | null = null
function schedule() {
  clearTimeout(timer)
  timer = setTimeout(load, 400)
}

onMounted(async () => {
  await load()
  try {
    unsub = await pb.collection('ticket_events').subscribe('*', schedule)
  } catch {
    // Realtime is progressive enhancement.
  }
})
onUnmounted(() => {
  clearTimeout(timer)
  unsub?.()
})
</script>

<template>
  <div v-if="events.length" class="card bg-base-100 shadow-sm">
    <div class="card-body py-4 px-4 space-y-2">
      <h2 class="font-semibold text-sm">Activity</h2>
      <ul class="space-y-2">
        <li v-for="e in events" :key="e.id" class="text-xs leading-snug">
          <span class="font-semibold text-base-content">{{ actor(e) }}</span>
          changed {{ fieldLabel[e.field] || e.field }}
          <span class="text-base-content/60">{{ e.old_value || '—' }}</span>
          →
          <span class="font-medium">{{ e.new_value || '—' }}</span>
          <div class="text-base-content/40">{{ formatDistanceToNow(new Date(e.created), { addSuffix: true }) }}</div>
        </li>
      </ul>
    </div>
  </div>
</template>
