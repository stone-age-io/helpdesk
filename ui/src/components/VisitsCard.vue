<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { pb } from '@/pb'
import { useAuthStore } from '@/stores/auth'
import type { Staff, Visit } from '@/types'
import SearchSelect from '@/components/SearchSelect.vue'
import { format } from 'date-fns'

const props = defineProps<{ ticketId: string; staff: Staff[] }>()
const auth = useAuthStore()

const visits = ref<Visit[]>([])
// '' = closed, 'request' = promote-to-on-site form, 'schedule' = time+tech
// form (creating fresh, or scheduling the `editing` requested visit).
const mode = ref<'' | 'request' | 'schedule'>('')
const editing = ref<Visit | null>(null)
const scheduledAt = ref('')
const assignee = ref('')
const location = ref('')
const notes = ref('')
const saving = ref(false)
const error = ref('')

async function load() {
  try {
    visits.value = await pb.collection('visits').getFullList<Visit>({
      filter: `ticket = '${props.ticketId}'`,
      sort: 'scheduled_at',
      expand: 'assignee',
    })
  } catch {
    // Optional context card.
  }
}

function closeForm() {
  mode.value = ''
  editing.value = null
  scheduledAt.value = ''
  location.value = ''
  notes.value = ''
}

function openRequest() {
  closeForm()
  mode.value = 'request'
}

function openSchedule(v?: Visit) {
  closeForm()
  mode.value = 'schedule'
  if (v) {
    editing.value = v
    location.value = v.location || ''
    notes.value = v.notes || ''
  }
  assignee.value = auth.record?.id || ''
}

async function submitRequest() {
  saving.value = true
  error.value = ''
  try {
    await pb.collection('visits').create({
      ticket: props.ticketId,
      status: 'requested',
      location: location.value.trim(),
      notes: notes.value.trim(),
    })
    closeForm()
    await load()
  } catch (err: any) {
    error.value = err?.message || 'Failed to request visit'
  } finally {
    saving.value = false
  }
}

async function submitSchedule() {
  if (!scheduledAt.value || !assignee.value) return
  saving.value = true
  error.value = ''
  const fields = {
    assignee: assignee.value,
    scheduled_at: new Date(scheduledAt.value).toISOString(),
    status: 'scheduled',
    location: location.value.trim(),
    notes: notes.value.trim(),
  }
  try {
    if (editing.value) {
      await pb.collection('visits').update(editing.value.id, fields)
    } else {
      await pb.collection('visits').create({ ticket: props.ticketId, ...fields })
    }
    closeForm()
    await load()
  } catch (err: any) {
    error.value = err?.message || 'Failed to schedule visit'
  } finally {
    saving.value = false
  }
}

async function setStatus(v: Visit, status: string) {
  try {
    await pb.collection('visits').update(v.id, { status })
    await load()
  } catch (err: any) {
    error.value = err?.message || 'Failed to update visit'
  }
}

const staffOptions = computed(() => props.staff.map((s) => ({ id: s.id, label: s.name, sublabel: s.email })))

const statusClass: Record<string, string> = {
  requested: 'badge-warning',
  scheduled: 'badge-info',
  completed: 'badge-success',
  canceled: 'badge-ghost',
}

onMounted(() => {
  assignee.value = auth.record?.id || ''
  load()
})
</script>

<template>
  <div class="card bg-base-100 shadow-sm">
    <div class="card-body py-4 px-4 space-y-2">
      <div class="flex justify-between items-center">
        <h2 class="font-semibold text-sm">Site Visits</h2>
        <div class="flex gap-1">
          <template v-if="mode === ''">
            <button class="btn btn-ghost btn-xs" @click="openRequest">+ Request</button>
            <button class="btn btn-ghost btn-xs" @click="openSchedule()">+ Schedule</button>
          </template>
          <button v-else class="btn btn-ghost btn-xs" @click="closeForm">Cancel</button>
        </div>
      </div>

      <div v-if="error" class="text-error text-xs">{{ error }}</div>

      <div v-if="mode === 'request'" class="space-y-1">
        <p class="text-xs text-base-content/60">Flag this ticket for on-site work — a dispatcher assigns the tech and time later.</p>
        <input v-model="location" type="text" placeholder="location (optional)" class="input input-bordered input-sm w-full" :disabled="saving" />
        <input v-model="notes" type="text" placeholder="notes (optional)" class="input input-bordered input-sm w-full" :disabled="saving" />
        <button class="btn btn-primary btn-sm w-full" :disabled="saving" @click="submitRequest">Request visit</button>
      </div>

      <div v-if="mode === 'schedule'" class="space-y-1">
        <p v-if="editing" class="text-xs text-base-content/60">Scheduling the requested visit.</p>
        <input v-model="scheduledAt" type="datetime-local" class="input input-bordered input-sm w-full min-w-0" :disabled="saving" />
        <SearchSelect v-model="assignee" :options="staffOptions" size="sm" placeholder="Assign technician…" :disabled="saving" />
        <input v-model="location" type="text" placeholder="location" class="input input-bordered input-sm w-full" :disabled="saving" />
        <input v-model="notes" type="text" placeholder="notes" class="input input-bordered input-sm w-full" :disabled="saving" />
        <button class="btn btn-primary btn-sm w-full" :disabled="saving || !scheduledAt || !assignee" @click="submitSchedule">Schedule</button>
      </div>

      <ul class="space-y-2">
        <li v-for="v in visits" :key="v.id" class="text-sm space-y-0.5">
          <div class="flex items-center gap-2">
            <span v-if="v.scheduled_at" class="font-medium whitespace-nowrap">{{ format(new Date(v.scheduled_at), 'MMM d, HH:mm') }}</span>
            <span v-else class="italic text-base-content/60">needs scheduling</span>
            <span class="badge badge-xs" :class="statusClass[v.status]">{{ v.status }}</span>
          </div>
          <div v-if="v.location" class="text-xs text-base-content/60 truncate">📍 {{ v.location }}</div>
          <div class="flex items-center gap-2 text-base-content/70">
            <span class="flex-1 truncate">{{ v.expand?.assignee?.name }}<template v-if="v.notes"><template v-if="v.expand?.assignee?.name"> — </template>{{ v.notes }}</template></span>
            <template v-if="v.status === 'requested'">
              <button class="btn btn-ghost btn-xs" @click="openSchedule(v)">Schedule</button>
              <button class="btn btn-ghost btn-xs text-error" @click="setStatus(v, 'canceled')">Cancel</button>
            </template>
            <template v-else-if="v.status === 'scheduled'">
              <button class="btn btn-ghost btn-xs" @click="setStatus(v, 'completed')">Done</button>
              <button class="btn btn-ghost btn-xs text-error" @click="setStatus(v, 'canceled')">Cancel</button>
            </template>
          </div>
        </li>
      </ul>
      <p v-if="visits.length === 0 && mode === ''" class="text-xs text-base-content/50">No visits scheduled.</p>
    </div>
  </div>
</template>
