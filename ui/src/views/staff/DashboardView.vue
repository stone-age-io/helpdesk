<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { pb } from '@/pb'
import { useAuthStore } from '@/stores/auth'
import type { Ticket } from '@/types'
import TicketBadges from '@/components/TicketBadges.vue'

const auth = useAuthStore()

const counts = ref({ open: 0, in_progress: 0, waiting: 0, urgent: 0, unassigned: 0 })
// Backlog aging: active tickets bucketed by age since creation. Answers
// "how much is going stale?" — the dashboard's point-in-time counts can't.
const aging = ref({ d0_2: 0, d3_7: 0, d7plus: 0 })
// New-ticket inflow, oldest→newest over the last 8 weeks (created-based).
const weeks = ref<number[]>([])
const weekMax = computed(() => Math.max(...weeks.value, 1))
const mine = ref<Ticket[]>([])
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
    const active = `status != 'resolved' && status != 'closed'`
    const [open, inProgress, waiting, urgent, unassigned] = await Promise.all([
      countOf(`status = 'open'`),
      countOf(`status = 'in_progress'`),
      countOf(`status = 'waiting'`),
      countOf(`priority = 'urgent' && ${active}`),
      countOf(`assignee = '' && ${active}`),
    ])
    counts.value = { open, in_progress: inProgress, waiting, urgent, unassigned }

    // Aging buckets (created-based) over the active backlog.
    const now = Date.now()
    const isoAgo = (days: number) => new Date(now - days * 864e5).toISOString().replace('T', ' ')
    const c2 = isoAgo(2)
    const c7 = isoAgo(7)
    const [a02, a37, a7] = await Promise.all([
      countOf(`${active} && created >= '${c2}'`),
      countOf(`${active} && created < '${c2}' && created >= '${c7}'`),
      countOf(`${active} && created < '${c7}'`),
    ])
    aging.value = { d0_2: a02, d3_7: a37, d7plus: a7 }

    // Inflow: tickets created per week for the last 8 weeks, one fetch,
    // bucketed client-side (oldest bucket first).
    const recent = await pb.collection('tickets').getFullList<Ticket>({
      filter: `created >= '${isoAgo(56)}'`,
      fields: 'created',
      sort: 'created',
    })
    const w = Array(8).fill(0)
    for (const t of recent) {
      const idx = Math.floor((now - new Date(t.created).getTime()) / (7 * 864e5))
      if (idx >= 0 && idx < 8) w[7 - idx] += 1
    }
    weeks.value = w

    mine.value = (
      await pb.collection('tickets').getList<Ticket>(1, 10, {
        filter: `assignee = '${auth.record?.id}' && ${active}`,
        sort: '-updated',
        expand: 'customer',
      })
    ).items
  } finally {
    if (!quiet) loading.value = false
  }
}

// Live counts: any ticket change refreshes after a short collapse window.
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
    <h1 class="text-2xl font-bold">Dashboard</h1>

    <div v-if="loading" class="flex justify-center p-12"><span class="loading loading-spinner loading-lg"></span></div>

    <template v-else>
      <!-- Each tile links to the queue pre-filtered to what it counts. -->
      <div class="stats stats-vertical sm:stats-horizontal shadow bg-base-100 w-full">
        <router-link to="/staff/tickets?status=open" class="stat hover:bg-base-200 transition-colors">
          <div class="stat-title">Open</div>
          <div class="stat-value text-info">{{ counts.open }}</div>
        </router-link>
        <router-link to="/staff/tickets?status=in_progress" class="stat hover:bg-base-200 transition-colors">
          <div class="stat-title">In Progress</div>
          <div class="stat-value text-primary">{{ counts.in_progress }}</div>
        </router-link>
        <router-link to="/staff/tickets?status=waiting" class="stat hover:bg-base-200 transition-colors">
          <div class="stat-title">Waiting</div>
          <div class="stat-value text-warning">{{ counts.waiting }}</div>
        </router-link>
        <router-link to="/staff/tickets?priority=urgent" class="stat hover:bg-base-200 transition-colors">
          <div class="stat-title">Urgent</div>
          <div class="stat-value text-error">{{ counts.urgent }}</div>
        </router-link>
        <router-link to="/staff/tickets?assignee=unassigned" class="stat hover:bg-base-200 transition-colors">
          <div class="stat-title">Unassigned</div>
          <div class="stat-value">{{ counts.unassigned }}</div>
        </router-link>
      </div>

      <div class="grid grid-cols-1 lg:grid-cols-2 gap-6">
        <!-- Backlog aging: how old is the open work? -->
        <div class="card bg-base-100 shadow-sm">
          <div class="card-body">
            <h2 class="card-title text-base">Backlog age</h2>
            <div class="stats stats-horizontal bg-base-100 w-full">
              <div class="stat px-3">
                <div class="stat-title text-xs">0–2 days</div>
                <div class="stat-value text-2xl text-success">{{ aging.d0_2 }}</div>
              </div>
              <div class="stat px-3">
                <div class="stat-title text-xs">3–7 days</div>
                <div class="stat-value text-2xl text-warning">{{ aging.d3_7 }}</div>
              </div>
              <div class="stat px-3">
                <div class="stat-title text-xs">Over 7 days</div>
                <div class="stat-value text-2xl text-error">{{ aging.d7plus }}</div>
              </div>
            </div>
            <p class="text-xs text-base-content/50">Active tickets by age since created.</p>
          </div>
        </div>

        <!-- Inflow: new tickets per week, last 8 weeks. -->
        <div class="card bg-base-100 shadow-sm">
          <div class="card-body">
            <h2 class="card-title text-base">New tickets / week</h2>
            <div class="flex items-end gap-1 h-24 mt-2">
              <div
                v-for="(n, i) in weeks"
                :key="i"
                class="flex-1 bg-primary/70 hover:bg-primary rounded-t transition-all"
                :style="{ height: Math.max((n / weekMax) * 100, 3) + '%' }"
                :title="`${n} ticket${n === 1 ? '' : 's'}`"
              ></div>
            </div>
            <p class="text-xs text-base-content/50">Last 8 weeks (oldest → newest). Hover for counts.</p>
          </div>
        </div>
      </div>

      <div class="card bg-base-100 shadow-sm">
        <div class="card-body">
          <h2 class="card-title text-base">My Active Tickets</h2>
          <div class="divide-y divide-base-200">
            <router-link
              v-for="t in mine"
              :key="t.id"
              :to="`/staff/tickets/${t.id}`"
              class="flex items-center gap-3 py-2 hover:bg-base-200/50 -mx-2 px-2 rounded"
            >
              <span class="font-mono text-xs text-base-content/50 w-10">#{{ t.number }}</span>
              <span class="flex-1 truncate">{{ t.title }}</span>
              <span class="text-xs text-base-content/60 hidden sm:block">{{ t.expand?.customer?.name }}</span>
              <TicketBadges :status="t.status" :priority="t.priority" />
            </router-link>
            <p v-if="mine.length === 0" class="py-3 text-sm text-base-content/50">Nothing assigned to you. Nice.</p>
          </div>
        </div>
      </div>
    </template>
  </div>
</template>
