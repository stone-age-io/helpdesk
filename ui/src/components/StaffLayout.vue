<script setup lang="ts">
import { onMounted, onUnmounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import AppSidebar, { type NavSection } from '@/components/AppSidebar.vue'
import ChangePasswordModal from '@/components/ChangePasswordModal.vue'
import ThemeToggle from '@/components/ThemeToggle.vue'

const route = useRoute()
const router = useRouter()
const showPassword = ref(false)

const sections: NavSection[] = [
  {
    items: [
      { label: 'Dashboard', icon: '📊', path: '/staff/dashboard' },
      { label: 'Tickets', icon: '🎫', path: '/staff/tickets' },
      { label: 'Dispatch', icon: '🚚', path: '/staff/dispatch' },
    ],
  },
  {
    title: 'Directory',
    items: [
      { label: 'Customers', icon: '🏢', path: '/staff/customers' },
      { label: 'Requesters', icon: '🙋', path: '/staff/requesters' },
    ],
  },
  {
    title: 'Administration',
    items: [
      { label: 'Staff', icon: '👥', path: '/staff/staff', adminOnly: true },
      { label: 'Notifications', icon: '✉️', path: '/staff/notifications', adminOnly: true },
    ],
  },
]

// Any navigation dismisses the mobile drawer — sidebar links close it
// themselves, but keyboard shortcuts and other programmatic pushes would
// otherwise change the page behind the still-open overlay.
watch(
  () => route.fullPath,
  () => {
    const drawer = document.getElementById('sidebar-drawer') as HTMLInputElement | null
    if (drawer) drawer.checked = false
  },
)

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
  <!-- daisyUI drawer shell: overlay sidebar below lg (checkbox-driven, no
       JS), permanent sidebar column on lg+. <main> is the only scroller. -->
  <div class="drawer lg:drawer-open h-dvh">
    <input id="sidebar-drawer" type="checkbox" class="drawer-toggle" />

    <div class="drawer-content flex flex-col min-h-0">
      <!-- Sticky header, mobile only (sidebar is permanent on lg+). A 3-column
           grid (1fr · auto · 1fr) keeps the brand dead-center regardless of
           how many buttons flank it. -->
      <header class="navbar bg-base-100 border-b border-base-300 min-h-[4rem] lg:hidden sticky top-0 z-30 pad-safe-top">
        <div class="grid grid-cols-[1fr_auto_1fr] items-center w-full">
          <div class="justify-self-start">
            <label for="sidebar-drawer" class="btn btn-square btn-ghost" aria-label="Open navigation menu">
              <svg xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24" class="inline-block w-6 h-6 stroke-current">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 6h16M4 12h16M4 18h16"></path>
              </svg>
            </label>
          </div>
          <span class="justify-self-center font-bold text-lg">Helpdesk</span>
          <div class="justify-self-end">
            <ThemeToggle />
          </div>
        </div>
      </header>

      <main class="flex-1 min-h-0 overflow-y-auto overscroll-y-contain bg-base-200">
        <div class="mx-auto w-full max-w-7xl p-4 lg:p-6 pad-safe-bottom">
          <router-view />
        </div>
      </main>
    </div>

    <div class="drawer-side z-40">
      <label for="sidebar-drawer" class="drawer-overlay" aria-label="Close navigation menu"></label>
      <AppSidebar :sections="sections" home="/staff/tickets" @change-password="showPassword = true" />
    </div>

    <ChangePasswordModal v-if="showPassword" @close="showPassword = false" />
  </div>
</template>
