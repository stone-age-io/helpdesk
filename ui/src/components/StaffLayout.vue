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
        <span class="text-xl font-bold mr-4">Helpdesk</span>
        <router-link to="/staff/dashboard" class="btn btn-ghost btn-sm" active-class="btn-active">Dashboard</router-link>
        <router-link to="/staff/tickets" class="btn btn-ghost btn-sm" active-class="btn-active">Tickets</router-link>
        <router-link to="/staff/customers" class="btn btn-ghost btn-sm" active-class="btn-active">Customers</router-link>
        <router-link to="/staff/requesters" class="btn btn-ghost btn-sm" active-class="btn-active">Requesters</router-link>
      </div>
      <div class="flex-none gap-2">
        <span class="text-sm text-base-content/70">
          {{ auth.record?.name || auth.record?.email }}
          <span class="badge badge-ghost badge-sm ml-1">{{ auth.record?.role }}</span>
        </span>
        <button class="btn btn-ghost btn-sm" @click="logout">Sign out</button>
      </div>
    </div>
    <main class="p-4 md:p-6 max-w-7xl mx-auto">
      <router-view />
    </main>
  </div>
</template>
