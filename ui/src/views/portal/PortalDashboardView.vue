<script setup lang="ts">
import { onMounted, onUnmounted, ref } from 'vue'
import { pb } from '@/pb'
import type { Ticket } from '@/types'
import TicketListRow from '@/components/TicketListRow.vue'

// Collection rules scope every query here to the requester's own customer —
// this is the staff dashboard recipe with nothing staff-only in it.
const counts = ref({ open: 0, in_progress: 0, waiting: 0, resolved: 0 })
const recent = ref<Ticket[]>([])
const loading = ref(true)

async function countOf(filter: string): Promise<number> {
  const res = await pb.collection('tickets').getList(1, 1, { filter })
  return res.totalItems
}

// quiet=true refreshes in place without the spinner swap — used by the
// realtime subscription so live updates don't flash the page.
async function load(quiet = false) {
  if (!quiet) loading.value = true
  try {
    const [open, inProgress, waiting, resolved] = await Promise.all([
      countOf(`status = 'open'`),
      countOf(`status = 'in_progress'`),
      countOf(`status = 'waiting'`),
      countOf(`status = 'resolved'`),
    ])
    counts.value = { open, in_progress: inProgress, waiting, resolved }
    recent.value = (await pb.collection('tickets').getList<Ticket>(1, 8, { sort: '-created' })).items
  } finally {
    if (!quiet) loading.value = false
  }
}

let reloadTimer: ReturnType<typeof setTimeout> | undefined
let unsubscribe: (() => void) | null = null

onMounted(async () => {
  await load()
  try {
    unsubscribe = await pb.collection('tickets').subscribe('*', () => {
      clearTimeout(reloadTimer)
      reloadTimer = setTimeout(() => load(true), 800)
    })
  } catch {
    // Realtime is progressive enhancement.
  }
})

onUnmounted(() => {
  clearTimeout(reloadTimer)
  unsubscribe?.()
})
</script>

<template>
  <div class="space-y-6">
    <div class="flex flex-col sm:flex-row justify-between items-start sm:items-center gap-2">
      <h1 class="text-2xl font-bold">Dashboard</h1>
      <router-link to="/portal/tickets/new" class="btn btn-primary btn-sm w-full sm:w-auto">New Ticket</router-link>
    </div>

    <div v-if="loading" class="flex justify-center p-12"><span class="loading loading-spinner loading-lg"></span></div>

    <template v-else>
      <!-- Each tile links to the tickets view pre-filtered to what it counts. -->
      <div class="stats stats-vertical sm:stats-horizontal shadow bg-base-100 w-full">
        <router-link to="/portal/tickets?status=open" class="stat hover:bg-base-200 transition-colors">
          <div class="stat-title">Open</div>
          <div class="stat-value text-info">{{ counts.open }}</div>
        </router-link>
        <router-link to="/portal/tickets?status=in_progress" class="stat hover:bg-base-200 transition-colors">
          <div class="stat-title">In Progress</div>
          <div class="stat-value text-primary">{{ counts.in_progress }}</div>
        </router-link>
        <router-link to="/portal/tickets?status=waiting" class="stat hover:bg-base-200 transition-colors">
          <div class="stat-title">Waiting</div>
          <div class="stat-value text-warning">{{ counts.waiting }}</div>
        </router-link>
        <router-link to="/portal/tickets?status=resolved" class="stat hover:bg-base-200 transition-colors">
          <div class="stat-title">Resolved</div>
          <div class="stat-value text-success">{{ counts.resolved }}</div>
        </router-link>
      </div>

      <div class="card bg-base-100 shadow-sm">
        <div class="card-body">
          <h2 class="card-title text-base">Recent Tickets</h2>
          <div class="divide-y divide-base-200">
            <TicketListRow v-for="t in recent" :key="t.id" :ticket="t" :to="`/portal/tickets/${t.id}`" show-age />
            <p v-if="recent.length === 0" class="py-3 text-sm text-base-content/50">
              No tickets yet. <router-link to="/portal/tickets/new" class="link">Open one</router-link>.
            </p>
          </div>
        </div>
      </div>
    </template>
  </div>
</template>
