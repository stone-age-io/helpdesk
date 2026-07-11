<script setup lang="ts">
import { ref } from 'vue'
import { useRouter } from 'vue-router'
import { useAuthStore } from '@/stores/auth'

const auth = useAuthStore()
const router = useRouter()

const email = ref('')
const password = ref('')
const loading = ref(false)
const error = ref('')

async function submit() {
  loading.value = true
  error.value = ''
  try {
    await auth.login(email.value, password.value)
    router.push(auth.isStaff ? '/staff/tickets' : '/portal/tickets')
  } catch {
    error.value = 'Invalid email or password.'
  } finally {
    loading.value = false
  }
}
</script>

<template>
  <div class="min-h-screen bg-base-200 flex items-center justify-center p-4">
    <div class="card bg-base-100 shadow-xl w-full max-w-sm">
      <div class="card-body">
        <h1 class="card-title text-2xl mb-2">Helpdesk</h1>
        <form class="space-y-4" @submit.prevent="submit">
          <div class="form-control">
            <label class="label"><span class="label-text">Email</span></label>
            <input v-model="email" type="email" class="input input-bordered" required autocomplete="username" :disabled="loading" />
          </div>
          <div class="form-control">
            <label class="label"><span class="label-text">Password</span></label>
            <input v-model="password" type="password" class="input input-bordered" required autocomplete="current-password" :disabled="loading" />
          </div>
          <div v-if="error" class="alert alert-error py-2 text-sm">{{ error }}</div>
          <button type="submit" class="btn btn-primary w-full" :disabled="loading">
            <span v-if="loading" class="loading loading-spinner loading-sm"></span>
            Sign in
          </button>
        </form>
      </div>
    </div>
  </div>
</template>
