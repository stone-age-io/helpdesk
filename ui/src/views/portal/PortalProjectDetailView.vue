<script setup lang="ts">
// Read-only project detail for requesters: the header, the tickets that make
// up the work, and the scheduled/completed visits. Never shows the technician
// or crew (the staff ViewRule drops the assignee expand, and we don't request
// it) — the MSP roster stays hidden, matching the ticket detail portal view.
//
// Both lists are bounded and say so. A rollout is the one thing in this app
// that legitimately carries fifty tickets and a hundred visits, and rendering
// all of them turned the page into a scroll with no summary at the top of it.
import { computed, onMounted, ref } from 'vue'
import { useRoute } from 'vue-router'
import { pb } from '@/pb'
import type { Project, ProjectStatus, Ticket, Visit, VisitStatus } from '@/types'
import TicketListRow from '@/components/TicketListRow.vue'
import { format } from 'date-fns'

const route = useRoute()
const id = route.params.id as string

const PAGE = 15

const project = ref<Project | null>(null)
const tickets = ref<Ticket[]>([])
const ticketTotal = ref(0)
const openTotal = ref(0)
const visits = ref<Visit[]>([])
const visitTotal = ref(0)
const loading = ref(true)
const error = ref('')
// Both lists start capped and expand in place. A "show all" that re-fetches is
// the honest control here: there is no other page holding a project's tickets,
// so a "View all →" link would have nowhere to point.
const allTickets = ref(false)
const allVisits = ref(false)

const statusClass: Record<ProjectStatus, string> = {
  pending: 'badge-soft-neutral',
  active: 'badge-soft-info',
  completed: 'badge-soft-success',
  canceled: 'badge-soft-neutral opacity-60',
}
const visitBadge: Record<VisitStatus, string> = {
  requested: 'badge-soft-neutral',
  scheduled: 'badge-soft-info',
  completed: 'badge-soft-success',
  canceled: 'badge-soft-neutral opacity-60',
}

function fmtDate(s?: string): string {
  return s ? format(new Date(s), 'MMM d, yyyy') : '—'
}
function fmtDateTime(s?: string): string {
  return s ? format(new Date(s), 'MMM d, yyyy HH:mm') : ''
}

async function loadTickets() {
  const res = await pb
    .collection('tickets')
    .getList<Ticket>(1, allTickets.value ? 500 : PAGE, { filter: `project = '${id}'`, sort: '-created' })
  tickets.value = res.items
  ticketTotal.value = res.totalItems
}

async function loadVisits() {
  // Visits on this project's tickets — no assignee expand (roster stays hidden).
  const res = await pb
    .collection('visits')
    .getList<Visit>(1, allVisits.value ? 500 : PAGE, { filter: `ticket.project = '${id}'`, sort: 'scheduled_at' })
  visits.value = res.items
  visitTotal.value = res.totalItems
}

async function load() {
  loading.value = true
  error.value = ''
  try {
    const [p, , , open] = await Promise.all([
      pb.collection('projects').getOne<Project>(id, { expand: 'location' }),
      loadTickets(),
      loadVisits(),
      pb.collection('tickets').getList<Ticket>(1, 1, {
        filter: `project = '${id}' && status != 'resolved' && status != 'closed'`,
      }),
    ])
    project.value = p
    openTotal.value = open.totalItems
  } catch (err: any) {
    error.value = err?.message || 'Failed to load project'
  } finally {
    loading.value = false
  }
}

const doneTotal = computed(() => ticketTotal.value - openTotal.value)

function expandTickets() {
  allTickets.value = true
  loadTickets()
}
function expandVisits() {
  allVisits.value = true
  loadVisits()
}

onMounted(load)
</script>

<template>
  <div class="space-y-4">
    <div class="breadcrumbs text-sm">
      <ul>
        <li><router-link to="/portal/projects">Projects</router-link></li>
        <li>{{ project ? `#${project.number}` : '…' }}</li>
      </ul>
    </div>

    <div v-if="error" class="alert alert-error py-2 text-sm">{{ error }}</div>
    <div v-if="loading" class="flex justify-center p-12"><span class="loading loading-spinner loading-lg"></span></div>

    <template v-else-if="project">
      <div class="card bg-base-100 shadow-sm">
        <div class="card-body gap-2">
          <div class="flex items-center gap-2 flex-wrap">
            <span class="badge-soft" :class="statusClass[project.status]">{{ project.status }}</span>
            <h1 class="text-xl font-bold">{{ project.title }}</h1>
          </div>
          <div class="text-sm text-base-content/60 space-y-0.5">
            <div v-if="project.expand?.location">
              📍
              <router-link :to="`/portal/locations/${project.location}`" class="link link-hover">
                {{ project.expand.location.name }}
              </router-link>
              <span v-if="project.expand.location.address" class="text-base-content/50"> — {{ project.expand.location.address }}</span>
            </div>
            <div v-if="project.start_date || project.target_date">
              🗓️
              <span v-if="project.start_date">{{ fmtDate(project.start_date) }} → </span>
              target {{ fmtDate(project.target_date) }}
            </div>
          </div>
          <p v-if="project.description" class="text-sm whitespace-pre-wrap mt-1">{{ project.description }}</p>

          <!-- Progress, which is what the page is opened for and what a wall of
               ticket rows was burying. -->
          <div v-if="ticketTotal" class="pt-1">
            <div class="flex items-center justify-between text-xs text-base-content/60 mb-1">
              <span>{{ doneTotal }} of {{ ticketTotal }} ticket{{ ticketTotal === 1 ? '' : 's' }} closed out</span>
              <span v-if="openTotal">{{ openTotal }} still open</span>
            </div>
            <progress class="progress progress-primary w-full" :value="doneTotal" :max="ticketTotal"></progress>
          </div>
        </div>
      </div>

      <!-- Tickets -->
      <div class="card bg-base-100 shadow-sm">
        <div class="card-body">
          <h2 class="font-semibold">
            Tickets <span class="text-base-content/50 font-normal">({{ ticketTotal }})</span>
          </h2>
          <div class="divide-y divide-base-200">
            <TicketListRow v-for="t in tickets" :key="t.id" :ticket="t" :to="`/portal/tickets/${t.id}`" />
            <p v-if="ticketTotal === 0" class="py-3 text-sm text-base-content/50">No tickets on this project yet.</p>
          </div>
          <button v-if="ticketTotal > tickets.length" class="btn btn-ghost btn-xs self-start" @click="expandTickets">
            Show all {{ ticketTotal }} →
          </button>
        </div>
      </div>

      <!-- Visits (no technician shown) -->
      <div v-if="visitTotal" class="card bg-base-100 shadow-sm">
        <div class="card-body">
          <h2 class="font-semibold">
            Visits <span class="text-base-content/50 font-normal">({{ visitTotal }})</span>
          </h2>
          <div class="divide-y divide-base-200">
            <div v-for="v in visits" :key="v.id" class="flex items-center gap-3 py-2">
              <span class="badge-soft" :class="visitBadge[v.status]">{{ v.status }}</span>
              <span class="flex-1 text-sm">
                <span v-if="v.scheduled_at">{{ fmtDateTime(v.scheduled_at) }}</span>
                <span v-else class="text-base-content/50">Not yet scheduled</span>
              </span>
              <span v-if="v.location" class="text-xs text-base-content/60">📍 {{ v.location }}</span>
            </div>
          </div>
          <button v-if="visitTotal > visits.length" class="btn btn-ghost btn-xs self-start" @click="expandVisits">
            Show all {{ visitTotal }} →
          </button>
        </div>
      </div>
    </template>
  </div>
</template>
