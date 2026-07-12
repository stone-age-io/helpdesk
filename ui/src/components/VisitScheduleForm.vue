<script setup lang="ts">
// Shared schedule/reschedule form for a visit: date+time, block duration,
// technician, location, notes. Purely presentational — it initializes from an
// optional `visit` and emits the field patch; the parent owns the save. Reused
// by VisitsCard (create), the Dispatch board, and VisitDetailDrawer
// (reschedule) so the three surfaces can't drift.
import { computed, ref, watch } from 'vue'
import { useAuthStore } from '@/stores/auth'
import type { Staff, Visit } from '@/types'
import SearchSelect from '@/components/SearchSelect.vue'
import { format } from 'date-fns'

const props = defineProps<{ staff: Staff[]; visit?: Visit | null; saving?: boolean }>()
const emit = defineEmits<{ submit: [fields: Record<string, any>]; cancel: [] }>()
const auth = useAuthStore()

const scheduledAt = ref('')
const duration = ref<number | null>(null)
const assignee = ref('')
const location = ref('')
const notes = ref('')

// Seed from the visit being (re)scheduled; default the tech to the current
// staffer when creating fresh, matching the previous inline-form behavior.
function reset() {
  const v = props.visit
  scheduledAt.value = v?.scheduled_at ? format(new Date(v.scheduled_at), "yyyy-MM-dd'T'HH:mm") : ''
  duration.value = v?.duration_minutes ?? null
  assignee.value = v?.assignee || auth.record?.id || ''
  location.value = v?.location || ''
  notes.value = v?.notes || ''
}
watch(() => props.visit, reset, { immediate: true })

const staffOptions = computed(() => props.staff.map((s) => ({ id: s.id, label: s.name, sublabel: s.email })))
const valid = computed(() => !!scheduledAt.value && !!assignee.value)
const isReschedule = computed(() => !!props.visit?.id)

function submit() {
  if (!valid.value) return
  emit('submit', {
    scheduled_at: new Date(scheduledAt.value).toISOString(),
    assignee: assignee.value,
    location: location.value.trim(),
    notes: notes.value.trim(),
    duration_minutes: duration.value || null,
    status: 'scheduled',
  })
}
</script>

<template>
  <div class="space-y-1">
    <input v-model="scheduledAt" type="datetime-local" class="input input-bordered input-sm w-full min-w-0" :disabled="saving" />
    <input v-model.number="duration" type="number" min="15" step="15" placeholder="duration (min, optional)" class="input input-bordered input-sm w-full" :disabled="saving" />
    <SearchSelect v-model="assignee" :options="staffOptions" size="sm" placeholder="Assign technician…" :disabled="saving" />
    <input v-model="location" type="text" placeholder="location" class="input input-bordered input-sm w-full" :disabled="saving" />
    <textarea v-model="notes" rows="2" placeholder="notes" class="textarea textarea-bordered textarea-sm w-full" :disabled="saving"></textarea>
    <div class="flex gap-1 justify-end pt-1">
      <button class="btn btn-ghost btn-sm" :disabled="saving" @click="emit('cancel')">Cancel</button>
      <button class="btn btn-primary btn-sm" :disabled="saving || !valid" @click="submit">
        <span v-if="saving" class="loading loading-spinner loading-xs"></span>
        {{ isReschedule ? 'Reschedule' : 'Schedule' }}
      </button>
    </div>
  </div>
</template>
