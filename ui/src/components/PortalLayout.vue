<script setup lang="ts">
import { useRouter } from 'vue-router'
import { useAuthStore } from '@/stores/auth'

const auth = useAuthStore()
const router = useRouter()

function logout() {
  auth.logout()
  router.push('/login')
}
</script>

<template>
  <div class="min-h-screen bg-base-200">
    <div class="navbar bg-base-100 shadow-sm px-4">
      <div class="flex-1 gap-1">
        <span class="text-xl font-bold mr-4">Support</span>
        <router-link to="/portal/tickets" class="btn btn-ghost btn-sm" active-class="btn-active">My Tickets</router-link>
        <router-link to="/portal/tickets/new" class="btn btn-ghost btn-sm" active-class="btn-active">New Ticket</router-link>
      </div>
      <div class="flex-none gap-2">
        <span class="text-sm text-base-content/70">{{ auth.record?.name || auth.record?.email }}</span>
        <button class="btn btn-ghost btn-sm" @click="logout">Sign out</button>
      </div>
    </div>
    <main class="p-4 md:p-6 max-w-4xl mx-auto">
      <router-view />
    </main>
  </div>
</template>
