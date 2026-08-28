<script setup lang="ts">
// The requester's sites, and what is happening at each one.
//
// This page exists for the question the ticket list cannot answer: "who is
// coming to Building B, and when". Everything else it shows — open ticket
// count, device count — is a launcher into /portal/tickets with the site or
// device filter applied, which is where the actual history lives. That is also
// why there is no per-site detail view: a location has almost no fields a
// requester cares about, so a detail page would just be the filtered ticket
// list with a heading.
//
// Collection rules scope every query here to the caller's customer. Visits
// expand `ticket` but never `assignee` — the MSP roster stays hidden, same as
// the portal visits and project views.
import { computed, onMounted, ref } from 'vue'
import { pb } from '@/pb'
import type { Location, Thing, Ticket, Visit } from '@/types'
import { format, startOfToday } from 'date-fns'

const locations = ref<Location[]>([])
const openTickets = ref<Pick<Ticket, 'id' | 'location'>[]>([])
const things = ref<Pick<Thing, 'id' | 'location'>[]>([])
const upcoming = ref<Visit[]>([])
const loading = ref(true)
const error = ref('')

// Four queries total, regardless of how many sites there are. The per-site
// counts are folded client-side from two id-only lists rather than issued as a
// count query per row — a customer with two dozen sites would otherwise pay two
// dozen round trips to render one page.
async function load() {
  loading.value = true
  error.value = ''
  try {
    const todayStart = startOfToday().toISOString().replace('T', ' ')
    const [locs, tix, thgs, vis] = await Promise.all([
      pb.collection('locations').getFullList<Location>({ sort: 'name', expand: 'parent' }),
      pb.collection('tickets').getFullList<Ticket>({
        filter: `status != 'resolved' && status != 'closed' && location != ''`,
        fields: 'id,location',
      }),
      pb.collection('things').getFullList<Thing>({
        filter: `retired = false && location != ''`,
        fields: 'id,location',
      }),
      pb.collection('visits').getFullList<Visit>({
        filter: `status = 'scheduled' && scheduled_at >= '${todayStart}'`,
        sort: 'scheduled_at',
        expand: 'ticket',
      }),
    ])
    locations.value = locs
    openTickets.value = tix
    things.value = thgs
    upcoming.value = vis
  } catch (e: any) {
    error.value = e?.message || 'Failed to load sites'
  } finally {
    loading.value = false
  }
}

const openCount = computed(() => tally(openTickets.value))
const deviceCount = computed(() => tally(things.value))

function tally(rows: { location?: string }[]): Record<string, number> {
  const out: Record<string, number> = {}
  for (const r of rows) if (r.location) out[r.location] = (out[r.location] || 0) + 1
  return out
}

// A visit belongs to a site through its ticket, so the grouping key is the
// ticket's location; visits on tickets with no site simply don't appear here.
const visitsBySite = computed(() => {
  const out: Record<string, Visit[]> = {}
  for (const v of upcoming.value) {
    const site = v.expand?.ticket?.location
    if (!site) continue
    ;(out[site] ||= []).push(v)
  }
  return out
})

const fmtWhen = (v?: string) => (v ? format(new Date(v), 'EEE, MMM d · HH:mm') : '')
const parentName = (l: Location) => l.expand?.parent?.name || ''

onMounted(load)
</script>

<template>
  <div class="space-y-4">
    <div>
      <h1 class="text-2xl font-bold">Sites</h1>
      <p class="text-sm text-base-content/60 mt-1">Your locations, what's open at each, and who's coming out.</p>
    </div>

    <div v-if="error" class="alert alert-error py-2 text-sm">{{ error }}</div>
    <div v-if="loading" class="flex justify-center p-12"><span class="loading loading-spinner loading-lg"></span></div>

    <!-- No sites on file is the normal state for a new customer, not an error:
         tickets carry a free-text location note until someone maps the sites. -->
    <div v-else-if="locations.length === 0" class="card bg-base-100 shadow-sm">
      <div class="card-body items-center text-center py-10">
        <p class="text-base-content/60">No sites on file yet.</p>
        <p class="text-sm text-base-content/50">
          Your tickets can still describe a location in their own words — ask our team to add your sites
          here and they'll become filterable.
        </p>
      </div>
    </div>

    <div v-else class="grid grid-cols-1 lg:grid-cols-2 gap-3">
      <div v-for="l in locations" :key="l.id" class="card bg-base-100 shadow-sm">
        <div class="card-body p-4 gap-3">
          <div>
            <div class="flex items-baseline gap-2 flex-wrap">
              <span class="font-semibold">{{ l.name }}</span>
              <span v-if="parentName(l)" class="text-xs text-base-content/50">in {{ parentName(l) }}</span>
            </div>
            <div v-if="l.address" class="text-sm text-base-content/60">{{ l.address }}</div>
          </div>

          <!-- The reason this page exists. Two visits shown; the rest roll up
               into the Visits view, which has the full filterable history. -->
          <div v-if="visitsBySite[l.id]?.length" class="rounded-box bg-base-200/60 px-3 py-2 space-y-1">
            <div class="text-xs uppercase tracking-wide text-base-content/50">Coming up</div>
            <div v-for="v in visitsBySite[l.id].slice(0, 2)" :key="v.id" class="text-sm">
              <span class="font-medium">{{ fmtWhen(v.scheduled_at) }}</span>
              <span class="text-base-content/60">
                · #{{ v.expand?.ticket?.number }} {{ v.expand?.ticket?.title }}
              </span>
            </div>
            <router-link
              v-if="visitsBySite[l.id].length > 2"
              :to="`/portal/visits?site=${l.id}`"
              class="link link-hover text-xs"
            >
              +{{ visitsBySite[l.id].length - 2 }} more scheduled →
            </router-link>
          </div>
          <div v-else class="text-sm text-base-content/50">No visits scheduled.</div>

          <div class="flex items-center gap-2 flex-wrap text-sm">
            <router-link :to="`/portal/tickets?location=${l.id}`" class="btn btn-xs btn-ghost">
              {{ openCount[l.id] || 0 }} open ticket{{ (openCount[l.id] || 0) === 1 ? '' : 's' }} →
            </router-link>
            <span v-if="deviceCount[l.id]" class="text-base-content/50">
              {{ deviceCount[l.id] }} device{{ deviceCount[l.id] === 1 ? '' : 's' }}
            </span>
            <router-link :to="`/portal/visits?site=${l.id}`" class="btn btn-xs btn-ghost ml-auto">
              Visits →
            </router-link>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>
