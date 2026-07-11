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
const showForm = ref(false)
const scheduledAt = ref('')
const assignee = ref('')
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

async function schedule() {
  if (!scheduledAt.value || !assignee.value) return
  saving.value = true
  error.value = ''
  try {
    await pb.collection('visits').create({
      ticket: props.ticketId,
      assignee: assignee.value,
      scheduled_at: new Date(scheduledAt.value).toISOString(),
      status: 'scheduled',
      notes: notes.value.trim(),
    })
    showForm.value = false
    scheduledAt.value = ''
    notes.value = ''
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
        <button class="btn btn-ghost btn-xs" @click="showForm = !showForm">{{ showForm ? 'Cancel' : '+ Schedule' }}</button>
      </div>

      <div v-if="error" class="text-error text-xs">{{ error }}</div>

      <div v-if="showForm" class="space-y-1">
        <input v-model="scheduledAt" type="datetime-local" class="input input-bordered input-sm w-full min-w-0" :disabled="saving" />
        <SearchSelect v-model="assignee" :options="staffOptions" size="sm" placeholder="Assign technician…" :disabled="saving" />
        <input v-model="notes" type="text" placeholder="notes" class="input input-bordered input-sm w-full" :disabled="saving" />
        <button class="btn btn-primary btn-sm w-full" :disabled="saving || !scheduledAt || !assignee" @click="schedule">Schedule</button>
      </div>

      <ul class="space-y-2">
        <li v-for="v in visits" :key="v.id" class="text-sm space-y-0.5">
          <div class="flex items-center gap-2">
            <span class="font-medium whitespace-nowrap">{{ format(new Date(v.scheduled_at), 'MMM d, HH:mm') }}</span>
            <span class="badge badge-xs" :class="statusClass[v.status]">{{ v.status }}</span>
          </div>
          <div class="flex items-center gap-2 text-base-content/70">
            <span class="flex-1 truncate">{{ v.expand?.assignee?.name }}<template v-if="v.notes"> — {{ v.notes }}</template></span>
            <template v-if="v.status === 'scheduled'">
              <button class="btn btn-ghost btn-xs" @click="setStatus(v, 'completed')">Done</button>
              <button class="btn btn-ghost btn-xs text-error" @click="setStatus(v, 'canceled')">Cancel</button>
            </template>
          </div>
        </li>
      </ul>
      <p v-if="visits.length === 0 && !showForm" class="text-xs text-base-content/50">No visits scheduled.</p>
    </div>
  </div>
</template>
