<script setup lang="ts">
// Read-only thing detail for requesters — the customer-facing half of the staff
// ThingDetailView, and the answer to "is this one a repeat offender". The
// header counts are the point; the recent list is bounded and links out to the
// full filtered history.
//
// Deliberately absent: `notes` and `metadata`. Notes on a thing are our service
// text (what we tried, what it's wired to), and metadata is a curated mirror of
// upstream config a requester has no way to act on. Name, code, type and
// location are facts about their own gear and do show. Visits never name a
// technician, matching every other portal view.
import { onMounted, ref } from 'vue'
import { useRoute } from 'vue-router'
import { pb } from '@/pb'
import type { Thing, Ticket, Visit, VisitStatus } from '@/types'
import TicketListRow from '@/components/TicketListRow.vue'
import { format, startOfToday } from 'date-fns'

const route = useRoute()
const id = route.params.id as string

const thing = ref<Thing | null>(null)
const tickets = ref<Ticket[]>([])
const ticketTotal = ref(0)
const openTotal = ref(0)
const visits = ref<Visit[]>([])
const loading = ref(true)
const error = ref('')

const visitBadge: Record<VisitStatus, string> = {
  requested: 'badge-soft-neutral',
  scheduled: 'badge-soft-info',
  completed: 'badge-soft-success',
  canceled: 'badge-soft-neutral opacity-60',
}

const fmtWhen = (v?: string) => (v ? format(new Date(v), 'EEE, MMM d · HH:mm') : '—')

async function load() {
  loading.value = true
  error.value = ''
  try {
    thing.value = await pb.collection('things').getOne<Thing>(id, { expand: 'type,location' })
    const [page, open, vis] = await Promise.all([
      pb.collection('tickets').getList<Ticket>(1, 10, { filter: `thing = '${id}'`, sort: '-created' }),
      pb.collection('tickets').getList<Ticket>(1, 1, {
        filter: `thing = '${id}' && status != 'resolved' && status != 'closed'`,
      }),
      // Visits reach a thing through their ticket. Bounded at 5 — "is anyone
      // coming to look at this", not the schedule.
      pb.collection('visits').getList<Visit>(1, 5, {
        filter: `ticket.thing = '${id}' && status = 'scheduled' && scheduled_at >= '${startOfToday()
          .toISOString()
          .replace('T', ' ')}'`,
        sort: 'scheduled_at',
        expand: 'ticket',
      }),
    ])
    tickets.value = page.items
    ticketTotal.value = page.totalItems
    openTotal.value = open.totalItems
    visits.value = vis.items
  } catch (err: any) {
    error.value = err?.message || 'Failed to load thing'
  } finally {
    loading.value = false
  }
}

onMounted(load)
</script>

<template>
  <div class="space-y-4">
    <div class="breadcrumbs text-sm">
      <ul>
        <li><router-link to="/portal/things">Things</router-link></li>
        <li>{{ thing?.name || '…' }}</li>
      </ul>
    </div>

    <div v-if="error" class="alert alert-error py-2 text-sm">{{ error }}</div>
    <div v-if="loading" class="flex justify-center p-12"><span class="loading loading-spinner loading-lg"></span></div>

    <template v-else-if="thing">
      <div class="card bg-base-100 shadow-sm">
        <div class="card-body gap-2">
          <div class="flex items-center gap-2 flex-wrap">
            <h1 class="text-xl font-bold">{{ thing.name }}</h1>
            <span v-if="thing.expand?.type?.name" class="badge badge-sm badge-soft">{{ thing.expand.type.name }}</span>
            <span v-if="thing.retired" class="badge badge-sm badge-soft">Retired</span>
          </div>
          <div class="text-sm text-base-content/60 space-y-0.5">
            <div v-if="thing.code">🏷️ <span class="font-mono">{{ thing.code }}</span></div>
            <div v-if="thing.expand?.location">
              📍
              <router-link :to="`/portal/locations/${thing.location}`" class="link link-hover">
                {{ thing.expand.location.name }}
              </router-link>
            </div>
          </div>

          <!-- Retired gear keeps its history but stops being a filing target,
               which is exactly what the staff roster and the scanner do too. -->
          <p v-if="thing.retired" class="text-xs text-base-content/50">
            This one is retired — its history stays here, but new tickets should name its replacement.
          </p>

          <div class="flex gap-2 flex-wrap pt-1">
            <router-link :to="`/portal/tickets?thing=${thing.id}&status=all`" class="btn btn-xs btn-ghost">
              {{ openTotal }} open · {{ ticketTotal }} total →
            </router-link>
            <router-link
              v-if="!thing.retired"
              to="/portal/tickets/new"
              class="btn btn-xs btn-primary ml-auto"
            >New ticket</router-link>
          </div>
        </div>
      </div>

      <div v-if="visits.length" class="card bg-base-100 shadow-sm">
        <div class="card-body p-4">
          <h2 class="card-title text-base">Coming up</h2>
          <div class="divide-y divide-base-200">
            <router-link
              v-for="v in visits"
              :key="v.id"
              :to="`/portal/tickets/${v.ticket}`"
              class="flex items-center gap-3 py-2 hover:bg-base-200/50 -mx-2 px-2 rounded"
            >
              <span class="badge-soft" :class="visitBadge[v.status]">{{ v.status }}</span>
              <span class="flex-1 text-sm">
                <span class="font-medium">{{ fmtWhen(v.scheduled_at) }}</span>
                <span class="text-base-content/60">
                  · #{{ v.expand?.ticket?.number }} {{ v.expand?.ticket?.title }}
                </span>
              </span>
            </router-link>
          </div>
        </div>
      </div>

      <div class="card bg-base-100 shadow-sm">
        <div class="card-body p-4">
          <div class="flex items-center justify-between gap-2">
            <h2 class="card-title text-base">Recent tickets</h2>
            <router-link
              v-if="ticketTotal > tickets.length"
              :to="`/portal/tickets?thing=${thing.id}&status=all`"
              class="text-xs link link-hover"
            >View all {{ ticketTotal }} →</router-link>
          </div>
          <div class="divide-y divide-base-200">
            <TicketListRow v-for="t in tickets" :key="t.id" :ticket="t" :to="`/portal/tickets/${t.id}`" show-age />
            <p v-if="tickets.length === 0" class="py-3 text-sm text-base-content/50">
              No tickets have named this one yet.
            </p>
          </div>
        </div>
      </div>
    </template>
  </div>
</template>
