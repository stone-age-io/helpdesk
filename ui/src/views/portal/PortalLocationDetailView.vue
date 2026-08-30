<script setup lang="ts">
// Read-only location detail for requesters.
//
// The Locations page used to be a dead end on purpose, on the reasoning that a
// detail view would be the filtered ticket list with a heading. That stopped
// being true when `things` became a relation: "what is installed here" is a
// question only this page can answer, and it is the one asked first — the same
// reason the staff LocationDetailView grew its "Things here" card.
//
// What is deliberately NOT shown: `notes`. On a location that field is the
// access notes our technicians write for each other (gate codes, which door to
// use, who to avoid) — our operational text about the customer's building, not
// the customer's own record of it. Address and site contact are theirs and do
// show. Visits never name a technician, matching the portal visit, project and
// locations views.
import { computed, onMounted, ref } from 'vue'
import { useRoute } from 'vue-router'
import { pb } from '@/pb'
import type { Location, Thing, Ticket, Visit, VisitStatus } from '@/types'
import TicketListRow from '@/components/TicketListRow.vue'
import { format, startOfToday } from 'date-fns'

const route = useRoute()
const id = route.params.id as string

const location = ref<Location | null>(null)
const things = ref<Thing[]>([])
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

// Maps deep link, coordinates preferred and the free-text address as fallback —
// the same pair the staff ticket card uses.
const mapsUrl = computed(() => {
  const l = location.value
  if (!l) return ''
  if (l.lat != null && l.lng != null) return `https://www.google.com/maps/search/?api=1&query=${l.lat},${l.lng}`
  return l.address ? `https://www.google.com/maps/search/?api=1&query=${encodeURIComponent(l.address)}` : ''
})

async function load() {
  loading.value = true
  error.value = ''
  try {
    location.value = await pb.collection('locations').getOne<Location>(id, { expand: 'parent,type' })
    // Retired gear is listed (badged) rather than hidden: you cannot file a new
    // ticket against decommissioned equipment, but "what used to be on this
    // door" is exactly the question this card is opened for. Same call the
    // staff view and the portal ticket filters make.
    const [thgs, page, open, vis] = await Promise.all([
      pb.collection('things').getFullList<Thing>({
        filter: `location = '${id}'`,
        sort: 'retired,name',
        expand: 'type',
      }),
      pb.collection('tickets').getList<Ticket>(1, 10, { filter: `location = '${id}'`, sort: '-created' }),
      pb.collection('tickets').getList<Ticket>(1, 1, {
        filter: `location = '${id}' && status != 'resolved' && status != 'closed'`,
      }),
      // Visits reach a location through their ticket. Bounded at 5: this is the
      // "who is coming here" answer, not the schedule — that lives in Visits.
      pb.collection('visits').getList<Visit>(1, 5, {
        filter: `ticket.location = '${id}' && status = 'scheduled' && scheduled_at >= '${startOfToday()
          .toISOString()
          .replace('T', ' ')}'`,
        sort: 'scheduled_at',
        expand: 'ticket',
      }),
    ])
    things.value = thgs
    tickets.value = page.items
    ticketTotal.value = page.totalItems
    openTotal.value = open.totalItems
    visits.value = vis.items
  } catch (err: any) {
    error.value = err?.message || 'Failed to load location'
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
        <li><router-link to="/portal/locations">Locations</router-link></li>
        <li>{{ location?.name || '…' }}</li>
      </ul>
    </div>

    <div v-if="error" class="alert alert-error py-2 text-sm">{{ error }}</div>
    <div v-if="loading" class="flex justify-center p-12"><span class="loading loading-spinner loading-lg"></span></div>

    <template v-else-if="location">
      <div class="card bg-base-100 shadow-sm">
        <div class="card-body gap-2">
          <div class="flex items-center gap-2 flex-wrap">
            <h1 class="text-xl font-bold">{{ location.name }}</h1>
            <span v-if="location.expand?.type?.name" class="badge badge-sm badge-soft">
              {{ location.expand.type.name }}
            </span>
          </div>
          <div class="text-sm text-base-content/60 space-y-0.5">
            <div v-if="location.expand?.parent">
              in
              <router-link :to="`/portal/locations/${location.parent}`" class="link link-hover">
                {{ location.expand.parent.name }}
              </router-link>
            </div>
            <div v-if="location.address">
              📍 {{ location.address }}
              <a v-if="mapsUrl" :href="mapsUrl" target="_blank" rel="noopener" class="link link-hover ml-1">Map →</a>
            </div>
            <div v-if="location.contact || location.contact_phone">
              👤 {{ location.contact }}
              <a v-if="location.contact_phone" :href="`tel:${location.contact_phone}`" class="link link-hover">
                {{ location.contact_phone }}
              </a>
            </div>
          </div>
          <div class="flex gap-2 flex-wrap pt-1">
            <router-link :to="`/portal/tickets?location=${location.id}`" class="btn btn-xs btn-ghost">
              {{ openTotal }} open · {{ ticketTotal }} total →
            </router-link>
            <router-link :to="`/portal/visits?location=${location.id}`" class="btn btn-xs btn-ghost">Visits →</router-link>
            <router-link to="/portal/tickets/new" class="btn btn-xs btn-primary ml-auto">New ticket</router-link>
          </div>
        </div>
      </div>

      <div class="grid grid-cols-1 lg:grid-cols-2 gap-4 items-start">
        <!-- Things installed here — the card this whole view was added for. -->
        <div class="card bg-base-100 shadow-sm">
          <div class="card-body p-4">
            <div class="flex items-center justify-between gap-2">
              <h2 class="card-title text-base">Things here</h2>
              <span v-if="things.length" class="text-xs text-base-content/50">{{ things.length }}</span>
            </div>
            <div class="divide-y divide-base-200">
              <router-link
                v-for="t in things"
                :key="t.id"
                :to="`/portal/things/${t.id}`"
                class="flex items-center gap-3 py-2 hover:bg-base-200/50 -mx-2 px-2 rounded"
              >
                <span class="font-mono text-xs text-base-content/50 w-20 truncate">{{ t.code || '—' }}</span>
                <span class="flex-1 truncate text-sm">{{ t.name }}</span>
                <span v-if="t.expand?.type?.name" class="badge badge-sm badge-soft hidden sm:inline-flex">
                  {{ t.expand.type.name }}
                </span>
                <span v-if="t.retired" class="badge badge-sm badge-soft">Retired</span>
              </router-link>
              <p v-if="things.length === 0" class="py-3 text-sm text-base-content/50">
                Nothing is filed at this location yet.
              </p>
            </div>
          </div>
        </div>

        <!-- Upcoming visits. No technician named, same as everywhere else in
             the portal. -->
        <div class="card bg-base-100 shadow-sm">
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
              <p v-if="visits.length === 0" class="py-3 text-sm text-base-content/50">No visits scheduled here.</p>
            </div>
          </div>
        </div>
      </div>

      <div class="card bg-base-100 shadow-sm">
        <div class="card-body p-4">
          <div class="flex items-center justify-between gap-2">
            <h2 class="card-title text-base">Recent tickets</h2>
            <router-link
              v-if="ticketTotal > tickets.length"
              :to="`/portal/tickets?location=${location.id}&status=all`"
              class="text-xs link link-hover"
            >View all {{ ticketTotal }} →</router-link>
          </div>
          <div class="divide-y divide-base-200">
            <TicketListRow v-for="t in tickets" :key="t.id" :ticket="t" :to="`/portal/tickets/${t.id}`" show-age />
            <p v-if="tickets.length === 0" class="py-3 text-sm text-base-content/50">No tickets at this location yet.</p>
          </div>
        </div>
      </div>
    </template>
  </div>
</template>
