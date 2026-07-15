<script setup lang="ts">
// Read-only project detail for requesters: the header, the tickets that make
// up the work, and the scheduled/completed visits. Never shows the technician
// or crew (the staff ViewRule drops the assignee expand, and we don't request
// it) — the MSP roster stays hidden, matching the ticket detail portal view.
import { onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { pb } from '@/pb'
import type { Project, ProjectStatus, Ticket, Visit } from '@/types'
import TicketBadges from '@/components/TicketBadges.vue'
import { format } from 'date-fns'

const route = useRoute()
const router = useRouter()
const id = route.params.id as string

const project = ref<Project | null>(null)
const tickets = ref<Ticket[]>([])
const visits = ref<Visit[]>([])
const loading = ref(true)
const error = ref('')

const statusClass: Record<ProjectStatus, string> = {
  planned: 'badge-soft-neutral',
  active: 'badge-soft-info',
  completed: 'badge-soft-success',
  canceled: 'badge-soft-neutral opacity-60',
}
const visitBadge: Record<string, string> = {
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

async function load() {
  loading.value = true
  error.value = ''
  try {
    project.value = await pb.collection('projects').getOne<Project>(id, { expand: 'location' })
    tickets.value = await pb.collection('tickets').getFullList<Ticket>({ filter: `project = '${id}'`, sort: '-created' })
    // Visits on this project's tickets — no assignee expand (roster stays hidden).
    visits.value = await pb.collection('visits').getFullList<Visit>({ filter: `ticket.project = '${id}'`, sort: 'scheduled_at' })
  } catch (err: any) {
    error.value = err?.message || 'Failed to load project'
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
        <li><a @click="router.push('/portal/projects')">Projects</a></li>
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
              📍 {{ project.expand.location.name }}
              <span v-if="project.expand.location.address" class="text-base-content/50"> — {{ project.expand.location.address }}</span>
            </div>
            <div v-if="project.start_date || project.target_date">
              🗓️
              <span v-if="project.start_date">{{ fmtDate(project.start_date) }} → </span>
              target {{ fmtDate(project.target_date) }}
            </div>
          </div>
          <p v-if="project.description" class="text-sm whitespace-pre-wrap mt-1">{{ project.description }}</p>
        </div>
      </div>

      <!-- Tickets -->
      <div class="card bg-base-100 shadow-sm">
        <div class="card-body">
          <h2 class="font-semibold">Tickets <span class="text-base-content/50 font-normal">({{ tickets.length }})</span></h2>
          <div class="divide-y divide-base-200">
            <router-link
              v-for="t in tickets"
              :key="t.id"
              :to="`/portal/tickets/${t.id}`"
              class="flex items-center gap-3 py-2 hover:bg-base-200/50 -mx-2 px-2 rounded"
            >
              <span class="font-mono text-xs text-base-content/50 w-10">#{{ t.number }}</span>
              <span class="flex-1 truncate">{{ t.title }}</span>
              <TicketBadges :status="t.status" :priority="t.priority" />
            </router-link>
            <p v-if="tickets.length === 0" class="py-3 text-sm text-base-content/50">No tickets on this project yet.</p>
          </div>
        </div>
      </div>

      <!-- Visits (no technician shown) -->
      <div v-if="visits.length" class="card bg-base-100 shadow-sm">
        <div class="card-body">
          <h2 class="font-semibold">Scheduled visits</h2>
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
        </div>
      </div>
    </template>
  </div>
</template>
