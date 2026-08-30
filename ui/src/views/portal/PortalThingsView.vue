<script setup lang="ts">
// The requester's things: the equipment, doors, and systems their tickets are
// filed against. The portal could already FILTER by thing — the intake form and
// the ticket list both offer a picker — but there was nowhere to browse the
// catalog, so "which of our gear keeps failing" had no surface at all. That is
// the question the relation replaced free text to answer, and it is as much the
// customer's question as ours.
//
// A ResponsiveList rather than the card grid Locations uses: this is a roster
// with a uniform column set and no per-row block to hold, so it reads better
// dense. Locations keeps cards because its "coming up" panel needs the room.
//
// Retired things are listed (badged) rather than hidden — you cannot file a new
// ticket against decommissioned gear, but reading its history is the whole
// point of keeping the row. Same call the ticket filters and the staff roster
// make. `notes` is deliberately absent: on a thing that is our service text,
// not the customer's record.
import { computed, onMounted, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import { pb } from '@/pb'
import { useQuerySync, useQueryValue } from '@/composables/useQuerySync'
import type { Location, Thing } from '@/types'
import ResponsiveList, { type Column } from '@/components/ResponsiveList.vue'

const router = useRouter()
const q = useQueryValue()

const things = ref<Thing[]>([])
const locations = ref<Location[]>([])
const openCounts = ref<Record<string, number>>({})
const loading = ref(true)
const error = ref('')

const search = ref('')
// Seeded from the Locations page and its per-location "N things →" link.
const location = ref(q('location'))
// Type filters key on the type NAME, not its id — the same call RosterFilters
// makes on the staff rosters, and for a weaker version of the same reason: a
// customer may hold several type rows that mean one concept, and a requester
// asking "our card readers" means all of them.
const type = ref(q('type'))
const showRetired = ref(q('retired') === '1')

// The boolean rides the URL as a computed '1', never as the ref itself:
// String(true) is 'true', which the `=== '1'` read above rejects, so the flag
// would write a query param that could not survive a reload — and String(false)
// is 'false', which the omit-defaults rule sees as a set value and would pin
// ?retired=false onto every otherwise-clean link.
const retiredParam = computed(() => (showRetired.value ? '1' : ''))
useQuerySync({ location, type, retired: retiredParam })

const columns: Column<Thing>[] = [
  { key: 'name', label: 'Name' },
  { key: 'code', label: 'Code', class: 'font-mono text-xs text-base-content/60' },
  { key: 'type', label: 'Type', format: (_, t) => t.expand?.type?.name || '—' },
  { key: 'location', label: 'Location', class: 'max-w-40 truncate', format: (_, t) => t.expand?.location?.name || '—' },
  { key: 'open', label: 'Open', class: 'w-16', format: (_, t) => String(openCounts.value[t.id] || 0) },
]

// Two queries, not one per row: the open-ticket count per thing is folded from
// a single id-only list of the customer's active tickets. Same shape as the
// Locations page, and for the same reason.
async function load() {
  loading.value = true
  error.value = ''
  try {
    const [thgs, locs, tix] = await Promise.all([
      pb.collection('things').getFullList<Thing>({ sort: 'retired,name', expand: 'type,location' }),
      pb.collection('locations').getFullList<Location>({ sort: 'name' }),
      pb.collection('tickets').getFullList({
        filter: `status != 'resolved' && status != 'closed' && thing != ''`,
        fields: 'id,thing',
      }),
    ])
    things.value = thgs
    locations.value = locs
    const counts: Record<string, number> = {}
    for (const t of tix as { thing?: string }[]) if (t.thing) counts[t.thing] = (counts[t.thing] || 0) + 1
    openCounts.value = counts
  } catch (e: any) {
    error.value = e?.message || 'Failed to load things'
  } finally {
    loading.value = false
  }
}

// Type options derive from the rows in scope rather than the full taxonomy, so
// picking a location narrows the type list — a dependent list, not a dependent
// gate. A narrowing that orphans the selected type clears it.
const scopedByLocation = computed(() =>
  location.value ? things.value.filter((t) => t.location === location.value) : things.value,
)
const typeOptions = computed(() =>
  [...new Set(scopedByLocation.value.map((t) => t.expand?.type?.name).filter(Boolean) as string[])].sort(),
)
watch(typeOptions, (opts) => {
  if (type.value && !opts.includes(type.value)) type.value = ''
})

const filtered = computed(() => {
  const needle = search.value.trim().toLowerCase()
  return scopedByLocation.value.filter((t) => {
    if (!showRetired.value && t.retired) return false
    if (type.value && t.expand?.type?.name !== type.value) return false
    if (!needle) return true
    return [t.name, t.code, t.expand?.type?.name, t.expand?.location?.name].some((v) =>
      v?.toLowerCase().includes(needle),
    )
  })
})

const retiredCount = computed(() => things.value.filter((t) => t.retired).length)

onMounted(load)
</script>

<template>
  <div class="space-y-4">
    <div>
      <h1 class="text-2xl font-bold">Things</h1>
      <p class="text-sm text-base-content/60 mt-1">
        The equipment, doors, and systems your tickets are filed against.
      </p>
    </div>

    <div v-if="error" class="alert alert-error py-2 text-sm">{{ error }}</div>
    <div v-if="loading" class="flex justify-center p-12"><span class="loading loading-spinner loading-lg"></span></div>

    <!-- An empty catalog is the normal state for a new customer, not an error:
         tickets carry a free-text thing note until someone maps the gear. -->
    <div v-else-if="things.length === 0" class="card bg-base-100 shadow-sm">
      <div class="card-body items-center text-center py-10">
        <p class="text-base-content/60">No things on file yet.</p>
        <p class="text-sm text-base-content/50">
          Your tickets can still describe the equipment in their own words — ask our team to add your
          things here and they'll become filterable.
        </p>
      </div>
    </div>

    <template v-else>
      <div class="flex flex-col sm:flex-row sm:flex-wrap gap-2">
        <input v-model="search" type="search" placeholder="Search things…" class="input input-bordered input-sm w-full sm:w-64" />
        <select v-if="locations.length > 1" v-model="location" class="select select-bordered select-sm w-full sm:w-auto">
          <option value="">All locations</option>
          <option v-for="l in locations" :key="l.id" :value="l.id">{{ l.name }}</option>
        </select>
        <select v-if="typeOptions.length > 1" v-model="type" class="select select-bordered select-sm w-full sm:w-auto">
          <option value="">All types</option>
          <option v-for="t in typeOptions" :key="t" :value="t">{{ t }}</option>
        </select>
        <button
          v-if="retiredCount"
          class="btn btn-sm self-start sm:self-auto"
          :class="showRetired ? 'btn-primary' : 'btn-ghost'"
          @click="showRetired = !showRetired"
        >
          Show retired ({{ retiredCount }})
        </button>
      </div>

      <ResponsiveList
        :items="filtered"
        :columns="columns"
        @row-click="(t: Thing) => router.push(`/portal/things/${t.id}`)"
      >
        <template #cell-name="{ item }">
          <span class="flex items-center gap-2">
            <span class="font-medium text-sm truncate">{{ item.name }}</span>
            <span v-if="item.retired" class="badge badge-sm badge-soft shrink-0">Retired</span>
          </span>
        </template>
        <template #card-name="{ item }">
          <div class="flex items-start gap-2">
            <div class="text-sm font-bold truncate flex-1">{{ item.name }}</div>
            <span v-if="item.retired" class="badge badge-sm badge-soft shrink-0">Retired</span>
          </div>
        </template>
        <template #cell-open="{ item }">
          <span :class="openCounts[item.id] ? 'font-semibold' : 'text-base-content/40'">
            {{ openCounts[item.id] || 0 }}
          </span>
        </template>
        <template #empty><span class="text-base-content/60">No things match.</span></template>
      </ResponsiveList>
    </template>
  </div>
</template>
