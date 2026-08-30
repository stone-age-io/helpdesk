<script setup lang="ts">
// Read-only project list for requesters: the installations / field work at
// their company's locations. Collection rules already scope this to the caller's
// customer. Deliberately shows no lead or crew — the MSP roster stays hidden.
import { computed, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { pb } from '@/pb'
import { useQuerySync, useQueryValue } from '@/composables/useQuerySync'
import type { Project, ProjectStatus } from '@/types'
import { PROJECT_STATUSES } from '@/types'
import { format } from 'date-fns'

const router = useRouter()
const q = useQueryValue()

const projects = ref<Project[]>([])
const loading = ref(true)
const error = ref('')

// Defaults to work in flight. A customer three years in has a long tail of
// completed and canceled rollouts, and mixing those into the same grid as the
// two jobs actually running buries the answer to "what's happening now" — the
// only question this page is opened with.
const status = ref<'open' | 'all' | ProjectStatus>((q('status') as any) || 'open')
useQuerySync({ status }, { status: 'open' })

const statusClass: Record<ProjectStatus, string> = {
  pending: 'badge-soft-neutral',
  active: 'badge-soft-info',
  completed: 'badge-soft-success',
  canceled: 'badge-soft-neutral opacity-60',
}

function fmtDate(s?: string): string {
  return s ? format(new Date(s), 'MMM d, yyyy') : '—'
}

async function load() {
  loading.value = true
  error.value = ''
  try {
    projects.value = await pb.collection('projects').getFullList<Project>({ sort: '-created', expand: 'location' })
  } catch (err: any) {
    error.value = err?.message || 'Failed to load projects'
  } finally {
    loading.value = false
  }
}

// Filtered in memory: the fetch is already whole-catalog (a customer holds
// projects in the dozens, not the thousands) and the option counts need every
// row anyway.
const filtered = computed(() => {
  if (status.value === 'all') return projects.value
  if (status.value === 'open') return projects.value.filter((p) => p.status === 'pending' || p.status === 'active')
  return projects.value.filter((p) => p.status === status.value)
})
const countOf = (s: ProjectStatus) => projects.value.filter((p) => p.status === s).length

onMounted(load)
</script>

<template>
  <div class="space-y-4">
    <h1 class="text-2xl font-bold">Projects</h1>
    <p class="text-sm text-base-content/60">Installations and scheduled work at your locations.</p>

    <div v-if="error" class="alert alert-error py-2 text-sm">{{ error }}</div>
    <div v-if="loading" class="flex justify-center p-12"><span class="loading loading-spinner loading-lg"></span></div>

    <div v-else-if="projects.length === 0" class="text-base-content/60">No projects yet.</div>

    <template v-else>
      <select v-model="status" class="select select-bordered select-sm w-full sm:w-auto">
        <option value="open">In flight ({{ countOf('pending') + countOf('active') }})</option>
        <option value="all">All ({{ projects.length }})</option>
        <option v-for="s in PROJECT_STATUSES" :key="s" :value="s">{{ s }} ({{ countOf(s) }})</option>
      </select>

      <div class="grid grid-cols-1 sm:grid-cols-2 gap-3 items-start">
        <button
          v-for="p in filtered"
          :key="p.id"
          class="card bg-base-100 shadow-sm hover:shadow-md transition-shadow text-left"
          @click="router.push(`/portal/projects/${p.id}`)"
        >
          <div class="card-body p-4 gap-1">
            <div class="flex items-center gap-2">
              <span class="badge-soft" :class="statusClass[p.status]">{{ p.status }}</span>
              <span class="font-mono text-xs text-base-content/50">#{{ p.number }}</span>
            </div>
            <div class="font-semibold">{{ p.title }}</div>
            <div class="text-sm text-base-content/60">
              <span v-if="p.expand?.location">📍 {{ p.expand.location.name }}</span>
              <span v-if="p.target_date"> · target {{ fmtDate(p.target_date) }}</span>
            </div>
          </div>
        </button>

        <p v-if="filtered.length === 0" class="text-sm text-base-content/50 py-2">No projects with that status.</p>
      </div>
    </template>
  </div>
</template>
