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
  <div class="min-h-dvh bg-base-200">
    <!-- Two destinations don't justify a drawer; the requester portal keeps a
         slim top bar. The account name collapses to an avatar on phones so
         the row never overflows. -->
    <div class="navbar bg-base-100 shadow-sm px-2 sm:px-4 sticky top-0 z-30 pad-safe-top">
      <div class="flex-1 gap-1 min-w-0">
        <span class="text-xl font-bold mr-2 sm:mr-4">Support</span>
        <router-link to="/portal/tickets" class="btn btn-ghost btn-sm" active-class="btn-active">My Tickets</router-link>
        <router-link to="/portal/tickets/new" class="btn btn-ghost btn-sm" active-class="btn-active">New Ticket</router-link>
      </div>
      <div class="flex-none gap-1 sm:gap-2">
        <ThemeToggle />
        <div class="dropdown dropdown-end">
          <div tabindex="0" role="button" class="btn btn-ghost btn-sm" :title="auth.record?.email">
            <span class="hidden sm:inline max-w-[12rem] truncate">{{ auth.record?.name || auth.record?.email }}</span>
            <span class="sm:hidden avatar placeholder">
              <span class="bg-neutral text-neutral-content rounded-full w-6 inline-flex items-center justify-center text-xs font-bold">
                {{ auth.initial }}
              </span>
            </span>
          </div>
          <ul tabindex="0" class="dropdown-content menu menu-sm bg-base-100 rounded-box shadow-lg border border-base-300 w-48 p-1 z-30">
            <li><a @click="showPassword = true">Change password</a></li>
            <li><a @click="logout">Sign out</a></li>
          </ul>
        </div>
      </div>
    </div>
    <main class="p-4 md:p-6 max-w-4xl mx-auto pad-safe-bottom">
      <router-view />
    </main>
    <ChangePasswordModal v-if="showPassword" @close="showPassword = false" />
  </div>
</template>
