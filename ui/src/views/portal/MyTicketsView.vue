<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { pb } from '@/pb'
import type { Ticket } from '@/types'
import TicketBadges from '@/components/TicketBadges.vue'
import { formatDistanceToNow } from 'date-fns'

const tickets = ref<Ticket[]>([])
const loading = ref(true)
const error = ref('')

async function load() {
  loading.value = true
  try {
    // Collection rules scope this to the requester's own customer.
    tickets.value = await pb.collection('tickets').getFullList<Ticket>({ sort: '-created' })
  } catch (err: any) {
    error.value = err?.message || 'Failed to load tickets'
  } finally {
    loading.value = false
  }
}

onMounted(load)
</script>

<template>
  <div class="space-y-4">
    <div class="flex justify-between items-center">
      <h1 class="text-2xl font-bold">My Tickets</h1>
      <router-link to="/portal/tickets/new" class="btn btn-primary btn-sm">New Ticket</router-link>
    </div>

    <div v-if="loading" class="flex justify-center p-12"><span class="loading loading-spinner loading-lg"></span></div>
    <div v-else-if="error" class="alert alert-error">{{ error }}</div>
    <div v-else-if="tickets.length === 0" class="text-center p-12 text-base-content/60">
      No tickets yet. <router-link to="/portal/tickets/new" class="link">Open one</router-link>.
    </div>

    <div v-else class="space-y-2">
      <router-link
        v-for="t in tickets"
        :key="t.id"
        :to="`/portal/tickets/${t.id}`"
        class="card bg-base-100 shadow-sm hover:shadow transition-shadow"
      >
        <div class="card-body py-3 px-4 flex-row items-center gap-3">
          <span class="font-mono text-sm text-base-content/60">#{{ t.number }}</span>
          <span class="flex-1 font-medium truncate">{{ t.title }}</span>
          <TicketBadges :status="t.status" />
          <span class="text-xs text-base-content/50 whitespace-nowrap">{{ formatDistanceToNow(new Date(t.created)) }} ago</span>
        </div>
      </router-link>
    </div>
  </div>
</template>
