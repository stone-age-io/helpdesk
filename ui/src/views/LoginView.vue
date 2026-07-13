<script setup lang="ts">
import { ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useAuthStore } from '@/stores/auth'
import ThemeToggle from '@/components/ThemeToggle.vue'

const auth = useAuthStore()
const route = useRoute()
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
    // Honor an email deep link (/t/{id}) that bounced through login.
    const redirect = route.query.redirect as string | undefined
    if (redirect && redirect.startsWith('/')) {
      router.push(redirect)
    } else {
      router.push(auth.isStaff ? '/staff/dashboard' : '/portal/dashboard')
    }
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
        <div class="flex justify-between items-start">
          <h1 class="card-title text-2xl mb-2">Service Desk</h1>
          <ThemeToggle />
        </div>
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
          <router-link to="/forgot-password" class="link link-hover text-sm block text-center">Forgot password?</router-link>
        </form>
      </div>
    </div>
  </div>
</template>
