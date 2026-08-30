<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { pb } from '@/pb'
import { useAuthStore } from '@/stores/auth'
import type { Ticket } from '@/types'
import TicketListRow from '@/components/TicketListRow.vue'

const auth = useAuthStore()

const counts = ref({ open: 0, in_progress: 0, waiting: 0, urgent: 0, unassigned: 0 })
// Backlog aging: active tickets bucketed by age since creation. Answers
// "how much is going stale?" — the dashboard's point-in-time counts can't.
const aging = ref({ d0_2: 0, d2_7: 0, d7plus: 0 })
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
    aging.value = { d0_2: a02, d2_7: a37, d7plus: a7 }

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
  <div class="space-y-4">
    <h1 class="text-2xl font-bold">Dashboard</h1>

    <div v-if="loading" class="flex justify-center p-12"><span class="loading loading-spinner loading-lg"></span></div>

    <template v-else>
      <!-- Each tile links to the queue pre-filtered to what it counts.
           Sized and weighted to match the Reports totals row — same element,
           same job — but the colours stay: they are the TicketBadges status and
           priority palette, so a tile reads as the chip you will meet in the
           queue rather than as decoration. -->
      <div class="stats stats-vertical sm:stats-horizontal shadow-sm bg-base-100 w-full">
        <router-link to="/staff/tickets?status=open" class="stat hover:bg-base-200 transition-colors">
          <div class="stat-title">Open</div>
          <div class="stat-value text-2xl tabular-nums text-info">{{ counts.open }}</div>
        </router-link>
        <router-link to="/staff/tickets?status=in_progress" class="stat hover:bg-base-200 transition-colors">
          <div class="stat-title">In Progress</div>
          <div class="stat-value text-2xl tabular-nums text-primary">{{ counts.in_progress }}</div>
        </router-link>
        <router-link to="/staff/tickets?status=waiting" class="stat hover:bg-base-200 transition-colors">
          <div class="stat-title">Waiting</div>
          <div class="stat-value text-2xl tabular-nums text-warning">{{ counts.waiting }}</div>
        </router-link>
        <router-link to="/staff/tickets?priority=urgent" class="stat hover:bg-base-200 transition-colors">
          <div class="stat-title">Urgent</div>
          <div class="stat-value text-2xl tabular-nums text-error">{{ counts.urgent }}</div>
        </router-link>
        <router-link to="/staff/tickets?assignee=unassigned" class="stat hover:bg-base-200 transition-colors">
          <div class="stat-title">Unassigned</div>
          <div class="stat-value text-2xl tabular-nums">{{ counts.unassigned }}</div>
        </router-link>
      </div>

      <div class="grid grid-cols-1 lg:grid-cols-2 gap-4 items-start">
        <!-- Backlog aging: how old is the open work? -->
        <div class="card bg-base-100 shadow-sm">
          <div class="card-body p-4 space-y-2">
            <h2 class="font-semibold text-sm">Backlog age</h2>
            <!-- Links, like the tiles above: these look identical to them, so
                 leaving them inert made the card promise a click it could not
                 honour. The queue's `age` buckets use the same created-based
                 boundaries as the counts here, and its status defaults to
                 active, so the number and the queue it opens agree. -->
            <div class="stats stats-horizontal bg-base-100 w-full">
              <router-link to="/staff/tickets?age=0-2" class="stat px-3 hover:bg-base-200 transition-colors">
                <div class="stat-title text-xs">0–2 days</div>
                <div class="stat-value text-2xl tabular-nums text-success">{{ aging.d0_2 }}</div>
              </router-link>
              <router-link to="/staff/tickets?age=2-7" class="stat px-3 hover:bg-base-200 transition-colors">
                <div class="stat-title text-xs">2–7 days</div>
                <div class="stat-value text-2xl tabular-nums text-warning">{{ aging.d2_7 }}</div>
              </router-link>
              <router-link to="/staff/tickets?age=7plus" class="stat px-3 hover:bg-base-200 transition-colors">
                <div class="stat-title text-xs">Over 7 days</div>
                <div class="stat-value text-2xl tabular-nums text-error">{{ aging.d7plus }}</div>
              </router-link>
            </div>
            <p class="text-xs text-base-content/50">Active tickets by age since created.</p>
          </div>
        </div>

        <!-- Inflow: new tickets per week, last 8 weeks. -->
        <div class="card bg-base-100 shadow-sm">
          <div class="card-body p-4 space-y-2">
            <h2 class="font-semibold text-sm">New tickets / week</h2>
            <!-- The count sits above each bar rather than in a tooltip. Hover is
                 not a thing on the phone this app is also read on, and a chart
                 whose only numbers are hover-only has no numbers there at all —
                 every bar in Reports is likewise a reading aid beside a value,
                 never the sole carrier of it. The label takes its own row so the
                 bar scales against the track that is left, not the whole card. -->
            <div class="flex items-end gap-1 h-28">
              <div v-for="(n, i) in weeks" :key="i" class="flex-1 h-full flex flex-col items-center gap-1">
                <span class="text-[10px] leading-none tabular-nums text-base-content/50">{{ n }}</span>
                <div class="w-full flex-1 min-h-0 flex items-end">
                  <div
                    class="w-full bg-primary/70 hover:bg-primary rounded-t transition-all"
                    :style="{ height: Math.max((n / weekMax) * 100, 3) + '%' }"
                    aria-hidden="true"
                  ></div>
                </div>
              </div>
            </div>
            <p class="text-xs text-base-content/50">Last 8 weeks (oldest → newest).</p>
          </div>
        </div>
      </div>

      <div class="card bg-base-100 shadow-sm">
        <div class="card-body p-4 space-y-2">
          <h2 class="font-semibold text-sm">My Active Tickets</h2>
          <div class="divide-y divide-base-200">
            <TicketListRow v-for="t in mine" :key="t.id" :ticket="t" :to="`/staff/tickets/${t.id}`" show-customer />
            <p v-if="mine.length === 0" class="py-3 text-sm text-base-content/50">Nothing assigned to you. Nice.</p>
          </div>
        </div>
      </div>
    </template>
  </div>
</template>
