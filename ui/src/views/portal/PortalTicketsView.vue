<script setup lang="ts">
import { onMounted, onUnmounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { pb } from '@/pb'
import { useAuthStore } from '@/stores/auth'
import type { Ticket, TicketPriority, TicketStatus } from '@/types'
import { TICKET_PRIORITIES, TICKET_STATUSES } from '@/types'
import TicketBadges from '@/components/TicketBadges.vue'
import ResponsiveList, { type Column } from '@/components/ResponsiveList.vue'
import Pager from '@/components/Pager.vue'
import { formatDistanceToNow } from 'date-fns'

const route = useRoute()
const router = useRouter()
const auth = useAuthStore()

// The collection rules already scope this list to the requester's customer;
// we page and filter server-side so a long-lived company's history stays fast.
const columns: Column<Ticket>[] = [
  { key: 'number', label: '#', class: 'w-16' },
  { key: 'title', label: 'Title', hideOnMobile: true },
  { key: 'status', label: 'Status' },
  { key: 'priority', label: 'Priority' },
  { key: 'created', label: 'Age', class: 'whitespace-nowrap text-base-content/60', format: (v) => formatDistanceToNow(new Date(v)) },
]

const tickets = ref<Ticket[]>([])
const loading = ref(true)
const error = ref('')

const page = ref(1)
const totalPages = ref(1)
const perPage = 30

// Filters. Status defaults to "active" (everything not resolved/closed);
// initial values may come from the URL query (dashboard tiles link here).
const q = (k: string) => (typeof route.query[k] === 'string' ? (route.query[k] as string) : '')
const status = ref<'active' | TicketStatus | ''>((q('status') as any) || 'active')
const priority = ref<TicketPriority | ''>((q('priority') as any) || '')
const search = ref('')
const mineOnly = ref(false)

function buildFilter(): string {
  const parts: string[] = []
  if (status.value === 'active') parts.push(`status != 'resolved' && status != 'closed'`)
  else if (status.value) parts.push(`status = '${status.value}'`)
  if (priority.value) parts.push(`priority = '${priority.value}'`)
  if (mineOnly.value && auth.record?.id) parts.push(`requester = '${auth.record.id}'`)
  if (search.value.trim()) {
    const raw = search.value.trim().replace(/'/g, "\\'")
    parts.push(`(title ~ '${raw}' || body ~ '${raw}')`)
  }
  return parts.join(' && ')
}

// quiet=true refreshes in place without the spinner swap (realtime updates).
async function load(quiet = false) {
  if (!quiet) loading.value = true
  error.value = ''
  try {
    const res = await pb.collection('tickets').getList<Ticket>(page.value, perPage, {
      filter: buildFilter(),
      sort: '-created',
    })
    tickets.value = res.items
    totalPages.value = res.totalPages
  } catch (err: any) {
    error.value = err?.message || 'Failed to load tickets'
  } finally {
    if (!quiet) loading.value = false
  }
}

watch([status, priority, mineOnly], () => {
  page.value = 1
  load()
})
watch(page, () => load())

let searchTimer: ReturnType<typeof setTimeout> | undefined
watch(search, () => {
  clearTimeout(searchTimer)
  searchTimer = setTimeout(() => {
    page.value = 1
    load()
  }, 300)
})

// --- CSV export of the current filter (all pages) — the portal "report" ---
function csvEscape(v: unknown): string {
  const s = String(v ?? '')
  return /[",\n]/.test(s) ? `"${s.replace(/"/g, '""')}"` : s
}
const exporting = ref(false)
async function exportCsv() {
  exporting.value = true
  error.value = ''
  try {
    const rows = await pb.collection('tickets').getFullList<Ticket>({ filter: buildFilter(), sort: '-created' })
    const header = ['number', 'title', 'status', 'priority', 'created', 'updated']
    const lines = [header.join(',')]
    for (const t of rows) {
      lines.push([t.number, t.title, t.status, t.priority, t.created, t.updated || ''].map(csvEscape).join(','))
    }
    const blob = new Blob([lines.join('\n')], { type: 'text/csv;charset=utf-8' })
    const a = document.createElement('a')
    a.href = URL.createObjectURL(blob)
    a.download = `tickets-${new Date().toISOString().slice(0, 10)}.csv`
    a.click()
    URL.revokeObjectURL(a.href)
  } catch (err: any) {
    error.value = err?.message || 'Export failed'
  } finally {
    exporting.value = false
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
  <div class="space-y-4">
    <div class="flex flex-col sm:flex-row justify-between items-start sm:items-center gap-2">
      <h1 class="text-2xl font-bold">Tickets</h1>
      <router-link to="/portal/tickets/new" class="btn btn-primary btn-sm w-full sm:w-auto">New Ticket</router-link>
    </div>

    <div class="flex flex-col sm:flex-row sm:flex-wrap gap-2">
      <input v-model="search" type="search" placeholder="Search title or body…" class="input input-bordered input-sm w-full sm:w-64" />
      <select v-model="status" class="select select-bordered select-sm w-full sm:w-auto">
        <option value="active">Active</option>
        <option value="">All statuses</option>
        <option v-for="s in TICKET_STATUSES" :key="s" :value="s">{{ s.replace('_', ' ') }}</option>
      </select>
      <select v-model="priority" class="select select-bordered select-sm w-full sm:w-auto">
        <option value="">All priorities</option>
        <option v-for="p in TICKET_PRIORITIES" :key="p" :value="p">{{ p }}</option>
      </select>
      <div class="flex gap-2">
        <button class="btn btn-sm flex-1 sm:flex-none" :class="mineOnly ? 'btn-primary' : 'btn-ghost'" @click="mineOnly = !mineOnly">
          Created by me
        </button>
        <button class="btn btn-sm btn-ghost flex-1 sm:flex-none" :disabled="exporting || tickets.length === 0" @click="exportCsv">
          <span v-if="exporting" class="loading loading-spinner loading-xs"></span>
          Export CSV
        </button>
      </div>
    </div>

    <div v-if="loading" class="flex justify-center p-12"><span class="loading loading-spinner loading-lg"></span></div>
    <div v-else-if="error" class="alert alert-error">{{ error }}</div>

    <ResponsiveList
      v-else
      :items="tickets"
      :columns="columns"
      @row-click="(t) => router.push(`/portal/tickets/${t.id}`)"
    >
      <template #cell-number="{ value }">
        <span class="font-mono text-sm">{{ value }}</span>
      </template>
      <template #cell-title="{ value }">
        <span class="block max-w-md truncate font-medium text-sm">{{ value }}</span>
      </template>
      <template #card-number="{ item }">
        <div class="text-sm font-bold truncate">
          <span class="font-mono text-base-content/60">#{{ item.number }}</span>
          {{ item.title }}
        </div>
      </template>
      <template #cell-status="{ value }"><TicketBadges :status="value" /></template>
      <template #cell-priority="{ value }"><TicketBadges :priority="value" /></template>
      <template #empty>
        <span class="text-base-content/60">No tickets match.</span>
      </template>
    </ResponsiveList>

    <Pager v-model:page="page" :total-pages="totalPages" />
  </div>
</template>
