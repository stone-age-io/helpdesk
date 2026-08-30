<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import { pb } from '@/pb'
import { useAuthStore } from '@/stores/auth'
import { useToastStore } from '@/stores/toast'
import { TICKET_PRIORITIES, type Location, type Thing, type TicketPriority } from '@/types'
import FileInput from '@/components/FileInput.vue'
import SearchSelect, { type SelectOption } from '@/components/SearchSelect.vue'

const router = useRouter()
const auth = useAuthStore()
const toast = useToastStore()

const title = ref('')
const body = ref('')
// Priority, location and thing are what the portal create rule leaves open to
// requesters; the triage fields (category / type / project / estimate) stay
// staff-only. Location and thing were staff-only too until migration 1825000000 —
// they are not judgements about the work, they are facts about where it is and
// what it is on, and at intake the requester is the only one who knows them.
const priority = ref<TicketPriority>('normal')
const locationId = ref('')
const locationNote = ref('')
const thingId = ref('')
const thingNote = ref('')
const files = ref<File[]>([])
const loading = ref(false)
const error = ref('')

// The catalogs, scoped to this requester's customer by the collection rules —
// no filter needed here, and none would be trustworthy if it were.
const locations = ref<Location[]>([])
const things = ref<Thing[]>([])

const TITLE_MAX = 300
const canSubmit = computed(() => !!title.value.trim() && !loading.value)

// Short, requester-friendly hints so "urgent" isn't the reflex choice.
const priorityHint: Record<TicketPriority, string> = {
  low: 'Minor — no rush.',
  normal: 'Standard — handled in turn.',
  high: 'Important — affecting work.',
  urgent: 'Critical — work is stopped.',
}

const locationOptions = computed<SelectOption[]>(() =>
  locations.value.map((l) => ({ id: l.id, label: l.name, sublabel: l.address || '' })),
)

// Things at the chosen location sort to the top, but nothing is hidden: a thing's
// `location` is optional, and a requester who knows the thing but not which
// location it is filed under would otherwise hit an empty list. Same call the staff
// form makes, for the same reason.
const thingOptions = computed<SelectOption[]>(() => {
  const siteName = (id?: string) => locations.value.find((l) => l.id === id)?.name || ''
  const rows = things.value.map((t) => ({
    id: t.id,
    label: t.name,
    sublabel: [t.code, siteName(t.location)].filter(Boolean).join(' · '),
    here: !!locationId.value && t.location === locationId.value,
  }))
  rows.sort((a, b) => Number(b.here) - Number(a.here) || a.label.localeCompare(b.label))
  return rows.map(({ id, label, sublabel }) => ({ id, label, sublabel }))
})

// Hide a picker the customer has no rows for rather than showing an empty
// typeahead — most customers have neither catalog populated, and for them this
// form stays exactly the two free-text fields it has always been.
const hasLocations = computed(() => locations.value.length > 0)
const hasThings = computed(() => things.value.length > 0)
// The thing note is the "not in the list" escape hatch, so it only earns space
// while nothing is picked. The location note is different — "Room 214" is worth
// saying even once the location is chosen — so that one always shows.
const showThingNote = computed(() => !thingId.value)

watch(thingId, (id) => {
  if (id) thingNote.value = ''
  // Picking a thing that belongs to a location fills the location in, when the
  // requester hasn't already chosen one. Saves the obvious second step.
  const t = things.value.find((x) => x.id === id)
  if (t?.location && !locationId.value) locationId.value = t.location
})

onMounted(async () => {
  // Best-effort: an empty or failed catalog load just means the pickers stay
  // hidden and the free-text fields carry the ticket, exactly as before.
  const [locs, thgs] = await Promise.allSettled([
    pb.collection('locations').getFullList<Location>({ sort: 'name' }),
    pb.collection('things').getFullList<Thing>({ filter: 'retired = false', sort: 'name' }),
  ])
  if (locs.status === 'fulfilled') locations.value = locs.value
  if (thgs.status === 'fulfilled') things.value = thgs.value
})

async function submit() {
  if (!canSubmit.value) return
  loading.value = true
  error.value = ''
  try {
    const rec = await pb.collection('tickets').create({
      customer: auth.record?.customer,
      requester: auth.record?.id,
      title: title.value.trim(),
      body: body.value.trim(),
      priority: priority.value,
      // Empty string is the correct "none" for both relations: the create rule
      // accepts `= ''` and would reject an id belonging to another customer.
      location: locationId.value,
      location_note: locationNote.value.trim(),
      thing: thingId.value,
      thing_note: thingNote.value.trim(),
      source: 'portal',
      attachments: files.value,
    })
    toast.success(rec.number ? `Ticket #${rec.number} created` : 'Ticket created')
    router.push(`/portal/tickets/${rec.id}`)
  } catch (err: any) {
    error.value = err?.message || 'Failed to create ticket'
    toast.error('Could not submit your ticket')
  } finally {
    loading.value = false
  }
}
</script>

<template>
  <div class="space-y-4 max-w-2xl mx-auto">
    <div>
      <h1 class="text-2xl font-bold">New Ticket</h1>
      <p class="text-sm text-base-content/60 mt-1">Tell us what's going on — our team will follow up by email.</p>
    </div>

    <form class="card bg-base-100 shadow-sm" @submit.prevent="submit">
      <div class="card-body space-y-4">
        <div v-if="error" class="alert alert-error py-2 text-sm">{{ error }}</div>

        <div class="form-control">
          <label class="label" for="nt-title">
            <span class="label-text">What do you need help with? <span class="text-error">*</span></span>
            <span class="label-text-alt text-base-content/50">{{ title.length }}/{{ TITLE_MAX }}</span>
          </label>
          <input
            id="nt-title"
            v-model="title"
            type="text"
            class="input input-bordered"
            required
            :maxlength="TITLE_MAX"
            :disabled="loading"
            placeholder="Short summary, e.g. “Badge reader offline at the north door”"
          />
        </div>

        <div class="form-control">
          <label class="label" for="nt-body"><span class="label-text">Details</span></label>
          <textarea
            id="nt-body"
            v-model="body"
            rows="6"
            class="textarea textarea-bordered"
            placeholder="What happened? What did you expect? When did it start?"
            :disabled="loading"
          ></textarea>
        </div>

        <div class="form-control">
          <label class="label"><span class="label-text">How urgent is this?</span></label>
          <div class="join" role="group" aria-label="Priority">
            <button
              v-for="p in TICKET_PRIORITIES"
              :key="p"
              type="button"
              class="btn btn-sm join-item capitalize"
              :class="priority === p ? 'btn-primary' : 'btn-outline'"
              :aria-pressed="priority === p"
              :disabled="loading"
              @click="priority = p"
            >
              {{ p }}
            </button>
          </div>
          <span class="label"><span class="label-text-alt text-base-content/50">{{ priorityHint[priority] }}</span></span>
        </div>

        <div v-if="hasLocations" class="form-control">
          <label class="label"><span class="label-text">Which location? <span class="text-base-content/40">(optional)</span></span></label>
          <SearchSelect
            v-model="locationId"
            :options="locationOptions"
            :disabled="loading"
            empty-label="Not listed / not sure"
            placeholder="Search your locations…"
          />
        </div>

        <div class="form-control">
          <label class="label" for="nt-location">
            <span class="label-text">
              {{ hasLocations ? 'Room or area' : 'Location' }}
              <span class="text-base-content/40">(optional)</span>
            </span>
          </label>
          <input
            id="nt-location"
            v-model="locationNote"
            type="text"
            class="input input-bordered"
            maxlength="200"
            :disabled="loading"
            :placeholder="
              hasLocations
                ? 'Room, floor, or door — narrows it down for the technician'
                : 'Building, room, or area — helps us send someone to the right place'
            "
          />
        </div>

        <div v-if="hasThings" class="form-control">
          <label class="label">
            <span class="label-text">Which thing? <span class="text-base-content/40">(optional)</span></span>
            <span v-if="locationId" class="label-text-alt text-base-content/50">Things at that location first</span>
          </label>
          <SearchSelect
            v-model="thingId"
            :options="thingOptions"
            :disabled="loading"
            empty-label="Not listed / not sure"
            placeholder="Search your equipment…"
          />
        </div>

        <div v-if="showThingNote" class="form-control">
          <label class="label" for="nt-thing">
            <span class="label-text">
              {{ hasThings ? 'Not listed? Describe it' : 'Equipment' }}
              <span class="text-base-content/40">(optional)</span>
            </span>
          </label>
          <input
            id="nt-thing"
            v-model="thingNote"
            type="text"
            class="input input-bordered"
            maxlength="200"
            :disabled="loading"
            placeholder="e.g. “the beige card reader by reception”"
          />
        </div>

        <div class="form-control">
          <label class="label"><span class="label-text">Attachments <span class="text-base-content/40">(optional)</span></span></label>
          <FileInput v-model:files="files" :disabled="loading" />
        </div>

        <div class="flex justify-end gap-2">
          <button type="button" class="btn btn-ghost" :disabled="loading" @click="router.back()">Cancel</button>
          <button type="submit" class="btn btn-primary" :disabled="!canSubmit">
            <span v-if="loading" class="loading loading-spinner loading-sm"></span>
            Submit
          </button>
        </div>
      </div>
    </form>
  </div>
</template>
