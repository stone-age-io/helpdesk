<script setup lang="ts">
// Field agent's home screen: today's visits assigned to me, each a big tap
// target straight into the work flow (Arrive → timer → Complete). The one
// screen a tech lives on during a shift.
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { pb } from '@/pb'
import { useAuthStore } from '@/stores/auth'
import { useTimerStore } from '@/stores/timer'
import type { Visit } from '@/types'
import FieldVisitCard from '@/components/FieldVisitCard.vue'
import { format } from 'date-fns'

const router = useRouter()
const auth = useAuthStore()
const timer = useTimerStore()

const visits = ref<Visit[]>([])
const loading = ref(true)
const error = ref('')

// Local-day window, in PocketBase's "YYYY-MM-DD HH:MM:SS.sssZ" string form.
function dayBounds(): { start: string; end: string } {
  const now = new Date()
  const s = new Date(now.getFullYear(), now.getMonth(), now.getDate())
  const e = new Date(s.getTime() + 24 * 60 * 60 * 1000)
  const toPb = (d: Date) => d.toISOString().replace('T', ' ')
  return { start: toPb(s), end: toPb(e) }
}

async function load() {
  const me = auth.record?.id
  if (!me) return
  error.value = ''
  const { start, end } = dayBounds()
  try {
    visits.value = await pb.collection('visits').getFullList<Visit>({
      filter: `assignee = '${me}' && status != 'canceled' && scheduled_at >= '${start}' && scheduled_at < '${end}'`,
      sort: 'scheduled_at',
      expand: 'ticket,ticket.customer,ticket.location',
    })
  } catch (e: any) {
    error.value = e?.message || 'Failed to load today'
  } finally {
    loading.value = false
  }
}

const liveCount = computed(() => visits.value.filter((v) => v.status !== 'completed').length)
const doneCount = computed(() => visits.value.filter((v) => v.status === 'completed').length)
// visits are sorted ascending by time, so the first still-open one is up next.
const nextVisit = computed(() => visits.value.find((v) => v.status !== 'completed' && v.status !== 'canceled'))

const timingThis = (v: Visit) => timer.isTimingVisit(v.id)

// Only the summary line needs a formatter now; the card formats its own fields.
const fmtTime = (v: Visit) => (v.scheduled_at ? format(new Date(v.scheduled_at), 'HH:mm') : '—')
function open(id: string) {
  router.push(`/staff/visits/${id}/work`)
}

// Realtime: a visit completed here or from the work view reflects without a
// manual refresh. Progressive enhancement.
let reloadTimer: ReturnType<typeof setTimeout> | undefined
let unsub: (() => void) | null = null
function scheduleReload() {
  clearTimeout(reloadTimer)
  reloadTimer = setTimeout(load, 400)
}
onMounted(async () => {
  await load()
  try {
    unsub = await pb.collection('visits').subscribe('*', scheduleReload)
  } catch {
    // no realtime; fine.
  }
})
onUnmounted(() => {
  clearTimeout(reloadTimer)
  unsub?.()
})
</script>

<template>
  <div class="space-y-4 max-w-2xl mx-auto">
    <div>
      <div class="text-xs text-base-content/50">{{ format(new Date(), 'EEEE, MMM d') }}</div>
      <h1 class="text-2xl font-bold">Today</h1>
    </div>

    <div v-if="loading" class="flex justify-center p-12"><span class="loading loading-spinner loading-lg"></span></div>
    <div v-else-if="error" class="alert alert-error text-sm">{{ error }}</div>

    <template v-else>
      <p class="text-sm text-base-content/60">
        <template v-if="visits.length">{{ liveCount }} to do<span v-if="doneCount"> · {{ doneCount }} done</span><span v-if="nextVisit"> · next {{ fmtTime(nextVisit) }}</span></template>
        <template v-else>Nothing scheduled for you today.</template>
      </p>

      <ul class="space-y-2">
        <li v-for="v in visits" :key="v.id">
          <FieldVisitCard :visit="v" :timing="timingThis(v)" @select="open" />
        </li>
      </ul>
    </template>
  </div>
</template>
