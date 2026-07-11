<script setup lang="ts">
// Edit the logged-in account's display name (staff or requester). The users
// self-update rule allows name (not customer/active); staff may edit their
// own too. authRefresh re-pulls the record so the shells update immediately.
import { ref } from 'vue'
import { pb } from '@/pb'
import { useAuthStore } from '@/stores/auth'

const emit = defineEmits<{ (e: 'close'): void }>()
const auth = useAuthStore()

const name = ref(auth.record?.name || '')
const saving = ref(false)
const error = ref('')
const done = ref(false)

async function submit() {
  saving.value = true
  error.value = ''
  const collection = auth.record?.collectionName
  try {
    await pb.collection(collection).update(auth.record.id, { name: name.value.trim() })
    await pb.collection(collection).authRefresh()
    done.value = true
    setTimeout(() => emit('close'), 900)
  } catch (err: any) {
    error.value = err?.data?.message || err?.message || 'Failed to save profile'
  } finally {
    saving.value = false
  }
}
</script>

<template>
  <dialog class="modal modal-open">
    <div class="modal-box max-w-sm">
      <h3 class="font-bold text-lg mb-3">Edit profile</h3>
      <div v-if="done" class="alert alert-success py-2 text-sm">Saved.</div>
      <form v-else class="space-y-3" @submit.prevent="submit">
        <div v-if="error" class="alert alert-error py-2 text-sm">{{ error }}</div>
        <div class="form-control">
          <label class="label py-1"><span class="label-text">Name</span></label>
          <input v-model="name" type="text" class="input input-bordered input-sm" maxlength="150" :disabled="saving" />
        </div>
        <div class="text-xs text-base-content/50">{{ auth.record?.email }}</div>
        <div class="modal-action mt-2">
          <button type="button" class="btn btn-ghost btn-sm" :disabled="saving" @click="emit('close')">Cancel</button>
          <button type="submit" class="btn btn-primary btn-sm" :disabled="saving || !name.trim()">
            <span v-if="saving" class="loading loading-spinner loading-xs"></span>
            Save
          </button>
        </div>
      </form>
    </div>
    <form method="dialog" class="modal-backdrop"><button @click.prevent="emit('close')">close</button></form>
  </dialog>
</template>
