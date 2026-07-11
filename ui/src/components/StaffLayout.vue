<script setup lang="ts">
import { onMounted, onUnmounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useAuthStore } from '@/stores/auth'
import ChangePasswordModal from '@/components/ChangePasswordModal.vue'
import ThemeToggle from '@/components/ThemeToggle.vue'

const auth = useAuthStore()
const route = useRoute()
const router = useRouter()
const showPassword = ref(false)

function logout() {
  auth.logout()
  router.push('/login')
}

// Global shortcuts: '/' focuses the queue search (navigating there first
// if needed), 'n' opens the new-ticket form. Suppressed while typing in
// any field or while a modal is open.
function onKeydown(e: KeyboardEvent) {
  if (e.ctrlKey || e.metaKey || e.altKey) return
  const target = e.target as HTMLElement
  if (target.closest('input, textarea, select, [contenteditable]')) return
  if (document.querySelector('.modal-open')) return

  if (e.key === '/') {
    e.preventDefault()
    if (route.name === 'tickets') {
      window.dispatchEvent(new Event('helpdesk:focus-search'))
    } else {
      router.push('/staff/tickets').then(() =>
        // Let the view mount before asking it to focus.
        setTimeout(() => window.dispatchEvent(new Event('helpdesk:focus-search')), 50),
      )
    }
  } else if (e.key === 'n') {
    e.preventDefault()
    router.push('/staff/tickets/new')
  }
}

onMounted(() => window.addEventListener('keydown', onKeydown))
onUnmounted(() => window.removeEventListener('keydown', onKeydown))
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
        <ThemeToggle />
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
