<script setup lang="ts">
import { computed, ref } from 'vue'
import { useRouter } from 'vue-router'
import { pb } from '@/pb'
import { useAuthStore } from '@/stores/auth'
import { useToastStore } from '@/stores/toast'
import { TICKET_PRIORITIES, type TicketPriority } from '@/types'
import FileInput from '@/components/FileInput.vue'

const router = useRouter()
const auth = useAuthStore()
const toast = useToastStore()

const title = ref('')
const body = ref('')
// Priority and a free-text site are the only classification fields the portal
// create rule leaves open to requesters (category/type/project/location are
// staff-only); everything else is triaged by staff after intake.
const priority = ref<TicketPriority>('normal')
const locationNote = ref('')
const files = ref<File[]>([])
const loading = ref(false)
const error = ref('')

const TITLE_MAX = 300
const canSubmit = computed(() => !!title.value.trim() && !loading.value)

// Short, requester-friendly hints so "urgent" isn't the reflex choice.
const priorityHint: Record<TicketPriority, string> = {
  low: 'Minor — no rush.',
  normal: 'Standard — handled in turn.',
  high: 'Important — affecting work.',
  urgent: 'Critical — work is stopped.',
}

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
      location_note: locationNote.value.trim(),
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

        <div class="form-control">
          <label class="label" for="nt-location">
            <span class="label-text">Location <span class="text-base-content/40">(optional)</span></span>
          </label>
          <input
            id="nt-location"
            v-model="locationNote"
            type="text"
            class="input input-bordered"
            maxlength="200"
            :disabled="loading"
            placeholder="Site, building, or room — helps us send someone to the right place"
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
