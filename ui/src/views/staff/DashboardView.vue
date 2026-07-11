<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { pb } from '@/pb'
import { useAuthStore } from '@/stores/auth'
import type { Ticket } from '@/types'
import TicketBadges from '@/components/TicketBadges.vue'

const auth = useAuthStore()

const counts = ref({ open: 0, in_progress: 0, waiting: 0, urgent: 0, unassigned: 0 })
const mine = ref<Ticket[]>([])
const loading = ref(true)

async function countOf(filter: string): Promise<number> {
  const res = await pb.collection('tickets').getList(1, 1, { filter })
  return res.totalItems
}

async function load() {
  loading.value = true
  try {
    const active = `status != 'resolved' && status != 'closed'`
    const [open, inProgress, waiting, urgent, unassigned] = await Promise.all([
      countOf(`status = 'open'`),
      countOf(`status = 'in_progress'`),
      countOf(`status = 'waiting'`),
      countOf(`priority = 'urgent' && ${active}`),
      countOf(`assignee = '' && ${active}`),
    ])
    counts.value = { open, in_progress: inProgress, waiting, urgent, unassigned }
    mine.value = (
      await pb.collection('tickets').getList<Ticket>(1, 10, {
        filter: `assignee = '${auth.record?.id}' && ${active}`,
        sort: '-updated',
        expand: 'customer',
      })
    ).items
  } finally {
    loading.value = false
  }
}

onMounted(load)
</script>

<template>
  <div class="space-y-6">
    <h1 class="text-2xl font-bold">Dashboard</h1>

    <div v-if="loading" class="flex justify-center p-12"><span class="loading loading-spinner loading-lg"></span></div>

    <template v-else>
      <div class="stats stats-vertical sm:stats-horizontal shadow bg-base-100 w-full">
        <div class="stat">
          <div class="stat-title">Open</div>
          <div class="stat-value text-info">{{ counts.open }}</div>
        </div>
        <div class="stat">
          <div class="stat-title">In Progress</div>
          <div class="stat-value text-primary">{{ counts.in_progress }}</div>
        </div>
        <div class="stat">
          <div class="stat-title">Waiting</div>
          <div class="stat-value text-warning">{{ counts.waiting }}</div>
        </div>
        <div class="stat">
          <div class="stat-title">Urgent</div>
          <div class="stat-value text-error">{{ counts.urgent }}</div>
        </div>
        <div class="stat">
          <div class="stat-title">Unassigned</div>
          <div class="stat-value">{{ counts.unassigned }}</div>
        </div>
      </div>

      <div class="card bg-base-100 shadow-sm">
        <div class="card-body">
          <h2 class="card-title text-base">My Active Tickets</h2>
          <ul class="space-y-1">
            <li v-for="t in mine" :key="t.id">
              <router-link :to="`/staff/tickets/${t.id}`" class="flex items-center gap-2 text-sm link-hover">
                <span class="font-mono">#{{ t.number }}</span>
                <span class="flex-1 truncate">{{ t.title }}</span>
                <span class="text-base-content/50">{{ t.expand?.customer?.name }}</span>
                <TicketBadges :status="t.status" :priority="t.priority" />
              </router-link>
            </li>
          </ul>
          <p v-if="mine.length === 0" class="text-sm text-base-content/50">Nothing assigned to you. Nice.</p>
        </div>
      </div>
    </template>
  </div>
</template>
