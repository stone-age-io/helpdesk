<script setup lang="ts">
import { ref } from 'vue'
import { useRouter } from 'vue-router'
import { useAuthStore } from '@/stores/auth'
import ChangePasswordModal from '@/components/ChangePasswordModal.vue'

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
        <span class="text-xl font-bold mr-4">Helpdesk</span>
        <router-link to="/staff/dashboard" class="btn btn-ghost btn-sm" active-class="btn-active">Dashboard</router-link>
        <router-link to="/staff/tickets" class="btn btn-ghost btn-sm" active-class="btn-active">Tickets</router-link>
        <router-link to="/staff/customers" class="btn btn-ghost btn-sm" active-class="btn-active">Customers</router-link>
        <router-link to="/staff/requesters" class="btn btn-ghost btn-sm" active-class="btn-active">Requesters</router-link>
        <router-link v-if="auth.isAdmin" to="/staff/staff" class="btn btn-ghost btn-sm" active-class="btn-active">Staff</router-link>
        <router-link v-if="auth.isAdmin" to="/staff/notifications" class="btn btn-ghost btn-sm" active-class="btn-active">Notifications</router-link>
      </div>
      <div class="flex-none gap-2">
        <div class="dropdown dropdown-end">
          <div tabindex="0" role="button" class="btn btn-ghost btn-sm">
            {{ auth.record?.name || auth.record?.email }}
            <span class="badge badge-ghost badge-sm">{{ auth.record?.role }}</span>
          </div>
          <ul tabindex="0" class="dropdown-content menu menu-sm bg-base-100 rounded-box shadow-lg border border-base-300 w-48 p-1 z-30">
            <li><a @click="showPassword = true">Change password</a></li>
            <li><a @click="logout">Sign out</a></li>
          </ul>
        </div>
      </div>
    </div>
    <main class="p-4 md:p-6 max-w-7xl mx-auto">
      <router-view />
    </main>
    <ChangePasswordModal v-if="showPassword" @close="showPassword = false" />
  </div>
</template>
