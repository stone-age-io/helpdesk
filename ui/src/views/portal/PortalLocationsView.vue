<script setup lang="ts">
// The requester's locations, and what is happening at each one.
//
// This page exists for the question the ticket list cannot answer: "who is
// coming to Building B, and when". The counts on each card are launchers into
// the filtered ticket history; the card itself opens the location detail, which
// is where the things installed there live.
//
// It stays a CARD GRID while the Things roster beside it is a ResponsiveList,
// and that is deliberate rather than drift. A location catalog is bounded by
// physical reality — five here, dozens at the top end — where Things is a
// forty-row catalog that reads better dense; and the card carries an address,
// the next visit, and two count launchers, which is more than a row's worth of
// slots at the width a phone gives you.
//
// What the card does NOT do any more is grow with the schedule: it shows the
// SOONEST visit and rolls the rest into Visits, which owns the full filterable
// history. Cards that grew a row per booking were what made this grid ragged.
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
const search = ref('')

// Four queries total, regardless of how many locations there are. The per-location
// counts are folded client-side from two id-only lists rather than issued as a
// count query per row — a customer with two dozen locations would otherwise pay two
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
    error.value = e?.message || 'Failed to load locations'
  } finally {
    loading.value = false
  }
}

// Search rather than paging: a customer's location catalog is bounded by
// physical reality (dozens, not thousands) and we already hold all of it to
// fold the counts, so filtering in memory costs nothing and a Pager would put
// a round trip in front of a list you can see the end of.
const filtered = computed(() => {
  const q = search.value.trim().toLowerCase()
  if (!q) return locations.value
  return locations.value.filter((l) =>
    [l.name, l.address, l.code, l.expand?.parent?.name].some((v) => v?.toLowerCase().includes(q)),
  )
})

const openCount = computed(() => tally(openTickets.value))
const thingCount = computed(() => tally(things.value))

function tally(rows: { location?: string }[]): Record<string, number> {
  const out: Record<string, number> = {}
  for (const r of rows) if (r.location) out[r.location] = (out[r.location] || 0) + 1
  return out
}

// A visit belongs to a location through its ticket, so the grouping key is the
// ticket's location; visits on tickets with no location simply don't appear here.
const visitsByLocation = computed(() => {
  const out: Record<string, Visit[]> = {}
  for (const v of upcoming.value) {
    const loc = v.expand?.ticket?.location
    if (!loc) continue
    ;(out[loc] ||= []).push(v)
  }
  return out
})

// The soonest scheduled visit at a location, or undefined. The list is already
// sorted by scheduled_at, so this is the head of it — the card shows one line
// and rolls the rest into Visits.
const nextVisit = (locationId: string): Visit | undefined => visitsByLocation.value[locationId]?.[0]

const fmtWhen = (v?: string) => (v ? format(new Date(v), 'EEE, MMM d · HH:mm') : '')
const parentName = (l: Location) => l.expand?.parent?.name || ''

onMounted(load)
</script>

<template>
  <div class="space-y-4">
    <div>
      <h1 class="text-2xl font-bold">Locations</h1>
      <p class="text-sm text-base-content/60 mt-1">Your locations, what's open at each, and who's coming out.</p>
    </div>

    <div v-if="error" class="alert alert-error py-2 text-sm">{{ error }}</div>
    <div v-if="loading" class="flex justify-center p-12"><span class="loading loading-spinner loading-lg"></span></div>

    <!-- No locations on file is the normal state for a new customer, not an error:
         tickets carry a free-text location note until someone maps them. -->
    <div v-else-if="locations.length === 0" class="card bg-base-100 shadow-sm">
      <div class="card-body items-center text-center py-10">
        <p class="text-base-content/60">No locations on file yet.</p>
        <p class="text-sm text-base-content/50">
          Your tickets can still describe a place in their own words — ask our team to add your locations
          here and they'll become filterable.
        </p>
      </div>
    </div>

    <template v-else>
      <input
        v-model="search"
        type="search"
        placeholder="Search locations…"
        class="input input-bordered input-sm w-full sm:w-72"
      />

      <!-- items-start, not the default stretch: a grid row sizes to its tallest
           cell, so a location with no visits and no address was padded out with
           dead space to match the one beside it. The cards are also built to a
           fixed set of slots — address, next visit, footer — each of which
           renders whether or not it has content, so two cards in a row differ in
           height only by how long their text is. -->
      <div class="grid grid-cols-1 lg:grid-cols-2 gap-3 items-start">
        <div v-for="l in filtered" :key="l.id" class="card bg-base-100 shadow-sm">
          <div class="card-body p-4 gap-3">
            <router-link :to="`/portal/locations/${l.id}`" class="-m-1 p-1 rounded hover:bg-base-200/50">
              <div class="flex items-baseline gap-2 flex-wrap">
                <span class="font-semibold">{{ l.name }}</span>
                <span v-if="parentName(l)" class="text-xs text-base-content/50">in {{ parentName(l) }}</span>
              </div>
              <!-- The slot is always rendered, empty or not: an address-less
                   location used to be a card one line shorter than its
                   neighbour for no reason a reader could see. It reserves the
                   line with a space rather than a dash — a sub-location
                   legitimately has no address of its own, and a "—" sitting
                   under the title reads as a stray mark rather than as an
                   absence. -->
              <div class="text-sm text-base-content/60">
                <template v-if="l.address">{{ l.address }}</template>
                <template v-else>&nbsp;</template>
              </div>
            </router-link>

            <!-- The reason this page exists. ONE line, not two: Visits carries
                 the full filterable schedule, so the card only has to answer
                 "is anyone coming, and when" — and a card that grew a row per
                 booked visit was what made this grid ragged in the first place. -->
            <div class="rounded-box bg-base-200/60 px-3 py-2 space-y-1">
              <div class="text-xs uppercase tracking-wide text-base-content/50">Coming up</div>
              <template v-if="nextVisit(l.id)">
                <div class="text-sm">
                  <span class="font-medium">{{ fmtWhen(nextVisit(l.id)!.scheduled_at) }}</span>
                  <span class="text-base-content/60">
                    · #{{ nextVisit(l.id)!.expand?.ticket?.number }} {{ nextVisit(l.id)!.expand?.ticket?.title }}
                  </span>
                </div>
                <router-link
                  v-if="visitsByLocation[l.id].length > 1"
                  :to="`/portal/visits?location=${l.id}`"
                  class="link link-hover text-xs"
                >
                  +{{ visitsByLocation[l.id].length - 1 }} more scheduled →
                </router-link>
              </template>
              <div v-else class="text-sm text-base-content/50">No visits scheduled.</div>
            </div>

            <div class="flex items-center gap-2 flex-wrap text-sm">
              <router-link :to="`/portal/tickets?location=${l.id}`" class="btn btn-xs btn-ghost">
                {{ openCount[l.id] || 0 }} open ticket{{ (openCount[l.id] || 0) === 1 ? '' : 's' }} →
              </router-link>
              <router-link v-if="thingCount[l.id]" :to="`/portal/things?location=${l.id}`" class="btn btn-xs btn-ghost">
                {{ thingCount[l.id] }} thing{{ thingCount[l.id] === 1 ? '' : 's' }} →
              </router-link>
              <router-link :to="`/portal/locations/${l.id}`" class="btn btn-xs btn-ghost ml-auto">
                Details →
              </router-link>
            </div>
          </div>
        </div>

        <p v-if="filtered.length === 0" class="text-sm text-base-content/50 py-4">No locations match “{{ search }}”.</p>
      </div>
    </template>
  </div>
</template>
