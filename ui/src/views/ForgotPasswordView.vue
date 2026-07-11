<script setup lang="ts">
// Self-service password reset request for requesters (users collection).
// PocketBase emails a reset link; the confirm step lives in
// ResetPasswordView. No account-existence oracle — the success message is
// the same whether or not the email matched.
import { ref } from 'vue'
import { pb } from '@/pb'

const email = ref('')
const loading = ref(false)
const sent = ref(false)
const error = ref('')

async function submit() {
  loading.value = true
  error.value = ''
  try {
    await pb.collection('users').requestPasswordReset(email.value.trim())
    sent.value = true
  } catch (err: any) {
    error.value = err?.message || 'Could not send the reset email. Try again.'
  } finally {
    loading.value = false
  }
}
</script>

<template>
  <div class="min-h-screen bg-base-200 flex items-center justify-center p-4">
    <div class="card bg-base-100 shadow-xl w-full max-w-sm">
      <div class="card-body">
        <h1 class="card-title text-2xl mb-2">Reset password</h1>
        <div v-if="sent" class="space-y-3">
          <div class="alert alert-success py-2 text-sm">
            If an account exists for that email, a reset link is on its way. Check your inbox.
          </div>
          <router-link to="/login" class="btn btn-ghost btn-sm w-full">Back to sign in</router-link>
        </div>
        <form v-else class="space-y-4" @submit.prevent="submit">
          <p class="text-sm text-base-content/60">Enter your email and we'll send a link to reset your password.</p>
          <div class="form-control">
            <label class="label"><span class="label-text">Email</span></label>
            <input v-model="email" type="email" class="input input-bordered" required autocomplete="username" :disabled="loading" />
          </div>
          <div v-if="error" class="alert alert-error py-2 text-sm">{{ error }}</div>
          <button type="submit" class="btn btn-primary w-full" :disabled="loading || !email.trim()">
            <span v-if="loading" class="loading loading-spinner loading-sm"></span>
            Send reset link
          </button>
          <router-link to="/login" class="link link-hover text-sm block text-center">Back to sign in</router-link>
        </form>
      </div>
    </div>
  </div>
</template>
