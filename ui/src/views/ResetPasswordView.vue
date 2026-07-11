<script setup lang="ts">
// Confirms a requester password reset. The token arrives as ?token= in the
// link PocketBase emailed. NB: the reset email's link target is set in the
// PocketBase dashboard (Settings → Mail) — point it at
// {APP_URL}/reset-password?token={TOKEN} for this page to receive the token.
import { computed, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { pb } from '@/pb'

const route = useRoute()
const router = useRouter()

const token = computed(() => (route.query.token as string) || '')
const password = ref('')
const passwordConfirm = ref('')
const loading = ref(false)
const error = ref('')
const done = ref(false)

async function submit() {
  if (password.value !== passwordConfirm.value) {
    error.value = 'Passwords do not match.'
    return
  }
  loading.value = true
  error.value = ''
  try {
    await pb.collection('users').confirmPasswordReset(token.value, password.value, passwordConfirm.value)
    done.value = true
    setTimeout(() => router.push('/login'), 1500)
  } catch (err: any) {
    error.value = err?.data?.message || err?.message || 'Reset failed — the link may have expired.'
  } finally {
    loading.value = false
  }
}
</script>

<template>
  <div class="min-h-screen bg-base-200 flex items-center justify-center p-4">
    <div class="card bg-base-100 shadow-xl w-full max-w-sm">
      <div class="card-body">
        <h1 class="card-title text-2xl mb-2">Set a new password</h1>

        <div v-if="!token" class="space-y-3">
          <div class="alert alert-error py-2 text-sm">This reset link is invalid or incomplete.</div>
          <router-link to="/forgot-password" class="btn btn-ghost btn-sm w-full">Request a new link</router-link>
        </div>

        <div v-else-if="done" class="alert alert-success py-2 text-sm">Password updated. Redirecting to sign in…</div>

        <form v-else class="space-y-4" @submit.prevent="submit">
          <div class="form-control">
            <label class="label"><span class="label-text">New password</span></label>
            <input v-model="password" type="password" class="input input-bordered" required minlength="8" autocomplete="new-password" :disabled="loading" />
          </div>
          <div class="form-control">
            <label class="label"><span class="label-text">Confirm new password</span></label>
            <input v-model="passwordConfirm" type="password" class="input input-bordered" required minlength="8" autocomplete="new-password" :disabled="loading" />
          </div>
          <div v-if="error" class="alert alert-error py-2 text-sm">{{ error }}</div>
          <button type="submit" class="btn btn-primary w-full" :disabled="loading || !password || !passwordConfirm">
            <span v-if="loading" class="loading loading-spinner loading-sm"></span>
            Update password
          </button>
        </form>
      </div>
    </div>
  </div>
</template>
