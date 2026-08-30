<script setup lang="ts">
// Scan a label, resolve the code, go to the record.
//
// The resolution rule is the load-bearing part, and it is not the obvious one
// (ADR 0002):
//
//   Resolve GLOBALLY across everything the scanner can see, then disambiguate.
//   Never resolve within a sticky customer context.
//
// Those sound similar and are not. Helpdesk staff have no customer field — they
// are cross-customer by design — so a tech standing in a hallway has no ambient
// tenant at all. Codes like DOOR-1, RDR-01 and AP-1 are exactly what every
// customer independently invents, so filtering by "the customer I was last
// looking at" would confidently show a different tenant's door. Searching
// everything is either right or visibly ambiguous, and visibly ambiguous is a
// picker rather than a wrong answer.
//
// Context still earns its keep: the customers this tech has scheduled visits
// with float to the top of the list. That SORTS the matches. It never removes
// one.
//
// Things and locations carry separate unique indexes on (customer, code), so one
// customer may legitimately hold a location and a thing with the same code. Both
// are searched and both can appear in the picker.
import { onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { pb } from '@/pb'
import type { Customer, Location, Thing } from '@/types'
import QrScanner from '@/components/QrScanner.vue'
import { useVisitContext } from '@/composables/useVisitContext'

interface Match {
  kind: 'thing' | 'location'
  id: string
  code: string
  name: string
  customerId: string
  customerName: string
  detail: string
  inContext: boolean
}

const router = useRouter()
// Shared with the Locations and Things rosters, where the same set is offered as an
// optional narrowing. Here it is sort-only — see the header.
const context = useVisitContext()

const scanned = ref('')
const matches = ref<Match[]>([])
const searching = ref(false)
const searched = ref(false)
const error = ref('')

// PocketBase filters are string-built here, so a code containing a quote would
// break the expression. Codes are slugs by convention but nothing enforces that
// on the scanned input, which can be anything a sticker holds.
function quote(value: string): string {
  return value.replace(/'/g, "\\'")
}

async function resolve(code: string) {
  scanned.value = code
  searching.value = true
  searched.value = false
  error.value = ''
  matches.value = []

  const q = quote(code)
  try {
    const [things, locations] = await Promise.all([
      pb.collection('things').getFullList<Thing>({ filter: `code = '${q}'`, expand: 'customer,location' }),
      pb.collection('locations').getFullList<Location>({ filter: `code = '${q}'`, expand: 'customer' }),
    ])

    const found: Match[] = [
      ...things.map((t) => ({
        kind: 'thing' as const,
        id: t.id,
        code: t.code || '',
        name: t.name,
        customerId: t.customer,
        customerName: (t.expand?.customer as Customer | undefined)?.name || '—',
        // A retired thing still resolves: you cannot file against it, but
        // reading its history is the reason the row is kept.
        detail: [(t.expand?.location as Location | undefined)?.name, t.retired ? 'Retired' : '']
          .filter(Boolean)
          .join(' · '),
        inContext: context.includes(t.customer),
      })),
      ...locations.map((l) => ({
        kind: 'location' as const,
        id: l.id,
        code: l.code || '',
        name: l.name,
        customerId: l.customer,
        customerName: (l.expand?.customer as Customer | undefined)?.name || '—',
        detail: l.address || '',
        inContext: context.includes(l.customer),
      })),
    ]

    found.sort((a, b) => {
      if (a.inContext !== b.inContext) return a.inContext ? -1 : 1
      if (a.customerName !== b.customerName) return a.customerName.localeCompare(b.customerName)
      return a.kind.localeCompare(b.kind)
    })

    matches.value = found
    // One match is unambiguous, so skip the picker entirely — the common case
    // should be scan-then-arrive, not scan-then-tap.
    if (found.length === 1) {
      open(found[0])
      return
    }
  } catch (err: any) {
    error.value = err?.message || 'Lookup failed'
  } finally {
    searching.value = false
    searched.value = true
  }
}

function open(m: Match) {
  router.push(m.kind === 'thing' ? `/staff/things/${m.id}` : `/staff/locations/${m.id}`)
}

function reset() {
  scanned.value = ''
  matches.value = []
  searched.value = false
  error.value = ''
}

onMounted(context.load)
</script>

<template>
  <div class="max-w-md mx-auto space-y-4">
    <div>
      <h1 class="text-2xl font-bold">Scan</h1>
      <p class="text-sm text-base-content/60 mt-1">
        Point at the label on a thing or location to open its record.
      </p>
    </div>

    <QrScanner @decoded="resolve" />

    <div v-if="searching" class="flex justify-center p-6">
      <span class="loading loading-spinner loading-md"></span>
    </div>

    <div v-if="error" class="alert alert-error py-2 text-sm">{{ error }}</div>

    <div v-if="searched && !searching" class="space-y-3">
      <div v-if="matches.length === 0" class="card bg-base-100 shadow-sm">
        <div class="card-body items-center text-center py-6">
          <div class="text-3xl" aria-hidden="true">🔍</div>
          <p class="font-medium">
            Nothing matches <span class="font-mono">{{ scanned }}</span>
          </p>
          <p class="text-sm text-base-content/60">
            The code may belong to gear that is not in the catalog yet, or the label may be
            mis-typed. Add it from Things or Locations and the label will resolve.
          </p>
          <button class="btn btn-ghost btn-sm mt-1" @click="reset">Scan again</button>
        </div>
      </div>

      <!--
        More than one match. Not an error state: two customers legitimately using
        DOOR-1 is the expected case, and so is one customer holding a location and a
        thing under the same code.
      -->
      <template v-else>
        <div class="text-sm text-base-content/60">
          <span class="font-mono">{{ scanned }}</span> matches {{ matches.length }} records — pick one.
        </div>
        <button
          v-for="m in matches"
          :key="`${m.kind}-${m.id}`"
          class="card bg-base-100 shadow-sm w-full text-left hover:bg-base-200 transition-colors"
          @click="open(m)"
        >
          <div class="card-body p-3 gap-1">
            <div class="flex items-center gap-2 flex-wrap">
              <span class="badge badge-sm badge-soft">{{ m.kind === 'thing' ? 'Thing' : 'Location' }}</span>
              <span class="font-medium truncate">{{ m.name }}</span>
              <span v-if="m.inContext" class="badge badge-sm badge-primary badge-soft">Your visit</span>
            </div>
            <div class="text-xs text-base-content/60 truncate">
              {{ m.customerName }}<span v-if="m.detail"> · {{ m.detail }}</span>
            </div>
          </div>
        </button>
      </template>
    </div>
  </div>
</template>
