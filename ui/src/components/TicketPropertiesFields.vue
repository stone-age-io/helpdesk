<script setup lang="ts">
// The staff ticket controls, in three tiers: Identity (who it's for — always
// visible), Workflow (how we're handling it — always visible), and a collapsed
// Classification/source section (changed rarely). Extracted so it can render in
// both the desktop rail and the mobile panel without duplicating markup. Purely
// presentational: it emits intent, the parent owns the saves.
import { computed } from 'vue'
import type { Ticket } from '@/types'
import { TICKET_PRIORITIES, TICKET_STATUSES, TICKET_TYPES } from '@/types'
import SearchSelect from '@/components/SearchSelect.vue'
import Avatar from '@/components/Avatar.vue'

interface Option {
  id: string
  label: string
  sublabel?: string
}

const props = defineProps<{
  ticket: Ticket
  staffOptions: Option[]
  customerOptions: Option[]
  categoryOptions: Option[]
  requesterOptions: Option[]
  locationOptions: Option[]
  projectOptions: Option[]
  notify: boolean
}>()

const emit = defineEmits<{
  'update-field': [field: 'status' | 'priority' | 'assignee', value: string]
  patch: [fields: Record<string, string>]
  'change-customer': [value: string]
  'create-location': [label: string]
  'update:notify': [value: boolean]
}>()

// Requesters (users collection) may carry a direct line; staff don't. Shown
// read-only in the identity tier when present — primary for a callback.
const requesterPhone = computed(() => (props.ticket.expand?.requester as any)?.phone || '')

// Maps deep link for the ticket's site: coordinates preferred, the location's
// free-text address as fallback. Empty (link hidden) when the site has neither.
const navigateUrl = computed(() => {
  const loc = props.ticket.expand?.location as any
  if (!loc) return ''
  if (loc.lat || loc.lng) return `https://www.google.com/maps/search/?api=1&query=${loc.lat},${loc.lng}`
  if (loc.address) return `https://www.google.com/maps/search/?api=1&query=${encodeURIComponent(loc.address)}`
  return ''
})
</script>

<template>
  <!-- Identity: who the ticket is for. Always visible — primary context. -->
  <div class="form-control">
    <label class="label py-1">
      <span class="label-text text-xs">Customer</span>
      <router-link v-if="ticket.expand?.customer" :to="`/staff/customers/${ticket.customer}`" class="label-text-alt link link-hover">view →</router-link>
    </label>
    <SearchSelect
      :model-value="ticket.customer || ''"
      :options="customerOptions"
      size="sm"
      placeholder="Type a customer…"
      @update:model-value="emit('change-customer', $event)"
    />
  </div>
  <div class="form-control">
    <label class="label py-1">
      <span class="label-text text-xs">Requester</span>
      <a v-if="requesterPhone" :href="`tel:${requesterPhone}`" class="label-text-alt link link-hover">{{ requesterPhone }}</a>
    </label>
    <SearchSelect
      :model-value="ticket.requester || ''"
      :options="requesterOptions"
      size="sm"
      empty-label="None"
      placeholder="Type a name or email…"
      @update:model-value="emit('patch', { requester: $event })"
    />
  </div>
  <!-- Location + project surface here as read chips only when set (the pickers
       to change them live in Classification below) — glanceable for field work
       without cluttering a plain desk ticket. -->
  <div v-if="ticket.expand?.location" class="flex items-center justify-between gap-2 px-1">
    <span class="text-xs text-base-content/60 shrink-0">Location</span>
    <span class="text-sm text-right flex items-center gap-2 min-w-0">
      <span class="truncate">📍 {{ ticket.expand.location.name }}</span>
      <a v-if="navigateUrl" :href="navigateUrl" target="_blank" rel="noopener" class="link link-hover shrink-0 text-xs">Navigate</a>
    </span>
  </div>
  <router-link
    v-if="ticket.expand?.project"
    :to="`/staff/projects/${ticket.project}`"
    class="flex items-center justify-between gap-2 px-1 link link-hover"
  >
    <span class="text-xs text-base-content/60 shrink-0">Project</span>
    <span class="text-sm text-right truncate">#{{ ticket.expand.project.number }} {{ ticket.expand.project.title }}</span>
  </router-link>

  <div class="divider my-0"></div>

  <!-- Workflow: how we're handling it. Always visible — the hot controls. -->
  <div class="form-control">
    <label class="label py-1"><span class="label-text text-xs">Status</span></label>
    <select class="select select-bordered select-sm" :value="ticket.status" @change="emit('update-field', 'status', ($event.target as HTMLSelectElement).value)">
      <option v-for="s in TICKET_STATUSES" :key="s" :value="s">{{ s.replace('_', ' ') }}</option>
    </select>
  </div>
  <div class="form-control">
    <label class="label py-1"><span class="label-text text-xs">Priority</span></label>
    <select class="select select-bordered select-sm" :value="ticket.priority" @change="emit('update-field', 'priority', ($event.target as HTMLSelectElement).value)">
      <option v-for="p in TICKET_PRIORITIES" :key="p" :value="p">{{ p }}</option>
    </select>
  </div>
  <div class="form-control">
    <label class="label py-1 gap-2">
      <span class="label-text text-xs">Assignee</span>
      <span v-if="ticket.expand?.assignee" class="flex items-center gap-1 label-text-alt">
        <Avatar :record="ticket.expand.assignee" :name="ticket.expand.assignee.name" size="xs" />
      </span>
    </label>
    <SearchSelect
      :model-value="ticket.assignee || ''"
      :options="staffOptions"
      size="sm"
      empty-label="Unassigned"
      placeholder="Type a name…"
      @update:model-value="emit('update-field', 'assignee', $event)"
    />
  </div>
  <label class="label cursor-pointer justify-start gap-2 py-1">
    <input
      type="checkbox"
      class="toggle toggle-sm toggle-primary"
      :checked="notify"
      @change="emit('update:notify', ($event.target as HTMLInputElement).checked)"
    />
    <span class="label-text text-xs">Email requester on changes</span>
  </label>

  <div class="divider my-0"></div>

  <!-- Classification + provenance: changed rarely, collapsed by default so the
       rail stays short and the identity/workflow tiers above stay in reach. -->
  <details class="group">
    <summary class="list-none cursor-pointer select-none flex items-center gap-2 py-1 text-xs font-semibold text-base-content/70 [&::-webkit-details-marker]:hidden">
      Classification &amp; source
      <span class="ml-auto transition-transform group-open:rotate-90">▸</span>
    </summary>
    <div class="space-y-2 pt-2">
  <div class="form-control">
    <label class="label py-1"><span class="label-text text-xs">Category</span></label>
    <SearchSelect
      :model-value="ticket.category || ''"
      :options="categoryOptions"
      size="sm"
      empty-label="None"
      placeholder="Classify…"
      @update:model-value="emit('patch', { category: $event })"
    />
  </div>
  <div class="form-control">
    <label class="label py-1"><span class="label-text text-xs">Type</span></label>
    <select class="select select-bordered select-sm" :value="ticket.type || 'issue'" @change="emit('patch', { type: ($event.target as HTMLSelectElement).value })">
      <option v-for="t in TICKET_TYPES" :key="t" :value="t">{{ t }}</option>
    </select>
  </div>
  <div class="form-control">
    <label class="label py-1"><span class="label-text text-xs">Project</span></label>
    <SearchSelect
      :model-value="ticket.project || ''"
      :options="projectOptions"
      size="sm"
      empty-label="None"
      placeholder="Attach to a project…"
      @update:model-value="emit('patch', { project: $event })"
    />
  </div>
  <div class="form-control">
    <label class="label py-1"><span class="label-text text-xs">Asset</span></label>
    <input
      :value="ticket.asset || ''"
      type="text"
      maxlength="200"
      class="input input-bordered input-sm"
      placeholder="Device / system"
      @change="emit('patch', { asset: ($event.target as HTMLInputElement).value })"
    />
  </div>
  <div class="form-control">
    <label class="label py-1"><span class="label-text text-xs">Location</span></label>
    <SearchSelect
      :model-value="ticket.location || ''"
      :options="locationOptions"
      size="sm"
      empty-label="None"
      placeholder="Pick a site…"
      create-label="New location"
      @update:model-value="emit('patch', { location: $event })"
      @create="emit('create-location', $event)"
    />
  </div>
  <div class="form-control">
    <label class="label py-1"><span class="label-text text-xs">Location note</span></label>
    <input
      :value="ticket.location_note || ''"
      type="text"
      maxlength="200"
      class="input input-bordered input-sm"
      placeholder="Access hints / where"
      @change="emit('patch', { location_note: ($event.target as HTMLInputElement).value })"
    />
  </div>
  <div class="flex items-center justify-between gap-2">
    <span class="text-xs text-base-content/60">Source</span>
    <span class="text-sm">{{ ticket.source }}</span>
  </div>
    </div>
  </details>
</template>
