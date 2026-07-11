<script setup lang="ts">
import { ref } from 'vue'
import { useRouter } from 'vue-router'
import { useAuthStore } from '@/stores/auth'
import ChangePasswordModal from '@/components/ChangePasswordModal.vue'
import ThemeToggle from '@/components/ThemeToggle.vue'

const auth = useAuthStore()
const router = useRouter()
const showPassword = ref(false)

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
        <ThemeToggle />
        <div class="dropdown dropdown-end">
          <div tabindex="0" role="button" class="btn btn-ghost btn-sm">{{ auth.record?.name || auth.record?.email }}</div>
          <ul tabindex="0" class="dropdown-content menu menu-sm bg-base-100 rounded-box shadow-lg border border-base-300 w-48 p-1 z-30">
            <li><a @click="showPassword = true">Change password</a></li>
            <li><a @click="logout">Sign out</a></li>
          </ul>
        </div>
      </div>
    </div>
    <main class="p-4 md:p-6 max-w-4xl mx-auto">
      <router-view />
    </main>
    <ChangePasswordModal v-if="showPassword" @close="showPassword = false" />
  </div>
</template>
