<script setup lang="ts">
// Self-serve password change for the logged-in account (staff or
// requester). PocketBase requires oldPassword for a self change and
// invalidates the auth token afterwards, so we silently re-authenticate
// with the new password on success.
import { ref } from 'vue'
import { pb } from '@/pb'
import { useAuthStore } from '@/stores/auth'

const emit = defineEmits<{ (e: 'close'): void }>()
const auth = useAuthStore()

const oldPassword = ref('')
const password = ref('')
const passwordConfirm = ref('')
const saving = ref(false)
const error = ref('')
const done = ref(false)

async function submit() {
  if (password.value !== passwordConfirm.value) {
    error.value = 'New passwords do not match.'
    return
  }
  saving.value = true
  error.value = ''
  const collection = auth.record?.collectionName
  const email = auth.record?.email
  try {
    await pb.collection(collection).update(auth.record.id, {
      oldPassword: oldPassword.value,
      password: password.value,
      passwordConfirm: passwordConfirm.value,
    })
    await pb.collection(collection).authWithPassword(email, password.value)
    done.value = true
    setTimeout(() => emit('close'), 1200)
  } catch (err: any) {
    error.value = err?.data?.data?.oldPassword?.message || err?.data?.message || err?.message || 'Failed to change password'
  } finally {
    saving.value = false
  }
}
</script>

<template>
  <dialog class="modal modal-open">
    <div class="modal-box max-w-sm">
      <h3 class="font-bold text-lg mb-3">Change password</h3>
      <div v-if="done" class="alert alert-success py-2 text-sm">Password changed.</div>
      <form v-else class="space-y-3" @submit.prevent="submit">
        <div v-if="error" class="alert alert-error py-2 text-sm">{{ error }}</div>
        <div class="form-control">
          <label class="label py-1"><span class="label-text">Current password</span></label>
          <input v-model="oldPassword" type="password" class="input input-bordered input-sm" required autocomplete="current-password" :disabled="saving" />
        </div>
        <div class="form-control">
          <label class="label py-1"><span class="label-text">New password</span></label>
          <input v-model="password" type="password" class="input input-bordered input-sm" required minlength="8" autocomplete="new-password" :disabled="saving" />
        </div>
        <div class="form-control">
          <label class="label py-1"><span class="label-text">Confirm new password</span></label>
          <input v-model="passwordConfirm" type="password" class="input input-bordered input-sm" required minlength="8" autocomplete="new-password" :disabled="saving" />
        </div>
        <div class="modal-action mt-2">
          <button type="button" class="btn btn-ghost btn-sm" :disabled="saving" @click="emit('close')">Cancel</button>
          <button type="submit" class="btn btn-primary btn-sm" :disabled="saving || !oldPassword || !password">
            <span v-if="saving" class="loading loading-spinner loading-xs"></span>
            Change
          </button>
        </div>
      </form>
    </div>
    <form method="dialog" class="modal-backdrop"><button @click.prevent="emit('close')">close</button></form>
  </dialog>
</template>
