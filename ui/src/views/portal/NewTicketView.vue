<script setup lang="ts">
import { ref } from 'vue'
import { useRouter } from 'vue-router'
import { pb } from '@/pb'
import { useAuthStore } from '@/stores/auth'
import FileInput from '@/components/FileInput.vue'

const router = useRouter()
const auth = useAuthStore()

const title = ref('')
const body = ref('')
const files = ref<File[]>([])
const loading = ref(false)
const error = ref('')

async function submit() {
  loading.value = true
  error.value = ''
  try {
    const rec = await pb.collection('tickets').create({
      customer: auth.record?.customer,
      requester: auth.record?.id,
      title: title.value.trim(),
      body: body.value.trim(),
      source: 'portal',
      attachments: files.value,
    })
    router.push(`/portal/tickets/${rec.id}`)
  } catch (err: any) {
    error.value = err?.message || 'Failed to create ticket'
  } finally {
    loading.value = false
  }
}
</script>

<template>
  <div class="space-y-4 max-w-2xl">
    <h1 class="text-2xl font-bold">New Ticket</h1>

    <form class="card bg-base-100 shadow-sm" @submit.prevent="submit">
      <div class="card-body space-y-3">
        <div v-if="error" class="alert alert-error py-2 text-sm">{{ error }}</div>
        <div class="form-control">
          <label class="label"><span class="label-text">What do you need help with? *</span></label>
          <input v-model="title" type="text" class="input input-bordered" required maxlength="300" :disabled="loading" />
        </div>
        <div class="form-control">
          <label class="label"><span class="label-text">Details</span></label>
          <textarea v-model="body" rows="6" class="textarea textarea-bordered" placeholder="What happened? What did you expect?" :disabled="loading"></textarea>
        </div>
        <div class="form-control">
          <label class="label"><span class="label-text">Attachments</span></label>
          <FileInput v-model:files="files" :disabled="loading" />
        </div>
        <div class="flex justify-end gap-2">
          <button type="button" class="btn btn-ghost" :disabled="loading" @click="router.back()">Cancel</button>
          <button type="submit" class="btn btn-primary" :disabled="loading || !title.trim()">
            <span v-if="loading" class="loading loading-spinner loading-sm"></span>
            Submit
          </button>
        </div>
      </div>
    </form>
  </div>
</template>
