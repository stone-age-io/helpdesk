<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { pb } from '@/pb'
import { useAuthStore } from '@/stores/auth'
import type { TimeEntry } from '@/types'
import { format } from 'date-fns'

const props = defineProps<{ ticketId: string }>()
const auth = useAuthStore()

const entries = ref<TimeEntry[]>([])
const minutes = ref<number | null>(null)
const note = ref('')
const saving = ref(false)
const error = ref('')

const totalMinutes = computed(() => entries.value.reduce((sum, e) => sum + e.minutes, 0))

function fmtTotal(m: number): string {
  const h = Math.floor(m / 60)
  return h > 0 ? `${h}h ${m % 60}m` : `${m}m`
}

async function load() {
  try {
    entries.value = await pb.collection('time_entries').getFullList<TimeEntry>({
      filter: `ticket = '${props.ticketId}'`,
      sort: '-work_date',
      expand: 'staff',
    })
  } catch {
    // Card is optional context; the ticket view stays usable.
  }
}

async function add() {
  if (!minutes.value || minutes.value < 1) return
  saving.value = true
  error.value = ''
  try {
    await pb.collection('time_entries').create({
      ticket: props.ticketId,
      staff: auth.record?.id,
      minutes: minutes.value,
      work_date: new Date().toISOString(),
      note: note.value.trim(),
    })
    minutes.value = null
    note.value = ''
    await load()
  } catch (err: any) {
    error.value = err?.message || 'Failed to log time'
  } finally {
    saving.value = false
  }
}

async function remove(entry: TimeEntry) {
  try {
    await pb.collection('time_entries').delete(entry.id)
    await load()
  } catch (err: any) {
    error.value = err?.message || 'Failed to delete entry'
  }
}

onMounted(load)
</script>

<template>
  <div class="card bg-base-100 shadow-sm">
    <div class="card-body py-4 px-4 space-y-2">
      <div class="flex justify-between items-center">
        <h2 class="font-semibold text-sm">Time</h2>
        <span class="badge badge-ghost badge-sm">{{ fmtTotal(totalMinutes) }}</span>
      </div>

      <div v-if="error" class="text-error text-xs">{{ error }}</div>

      <ul class="space-y-1">
        <li v-for="e in entries" :key="e.id" class="flex items-center justify-between text-sm gap-2">
          <span class="text-base-content/70 whitespace-nowrap">{{ format(new Date(e.work_date), 'MMM d') }}</span>
          <span class="flex-1 truncate" :title="e.note">{{ e.note || e.expand?.staff?.name || '' }}</span>
          <span class="font-mono whitespace-nowrap">{{ fmtTotal(e.minutes) }}</span>
          <button
            v-if="e.staff === auth.record?.id || auth.isAdmin"
            class="btn btn-ghost btn-xs text-error"
            @click="remove(e)"
          >✕</button>
        </li>
      </ul>

      <div class="flex gap-1 min-w-0">
        <input v-model.number="minutes" type="number" min="1" placeholder="min" class="input input-bordered input-sm w-16 shrink-0" :disabled="saving" />
        <input v-model="note" type="text" placeholder="note" class="input input-bordered input-sm flex-1 min-w-0" :disabled="saving" />
        <button class="btn btn-sm btn-primary shrink-0" :disabled="saving || !minutes" @click="add">Log</button>
      </div>
    </div>
  </div>
</template>
