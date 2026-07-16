<script setup lang="ts">
import { ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import AppSidebar, { type NavSection } from '@/components/AppSidebar.vue'
import ChangePasswordModal from '@/components/ChangePasswordModal.vue'
import ProfileModal from '@/components/ProfileModal.vue'
import ThemeToggle from '@/components/ThemeToggle.vue'

const route = useRoute()
const showPassword = ref(false)
const showProfile = ref(false)

const sections: NavSection[] = [
  {
    items: [
      { label: 'Dashboard', icon: '📊', path: '/portal/dashboard' },
      { label: 'Tickets', icon: '🎫', path: '/portal/tickets' },
      { label: 'Visits', icon: '📅', path: '/portal/visits' },
      { label: 'Projects', icon: '📁', path: '/portal/projects' },
      { label: 'New Ticket', icon: '➕', path: '/portal/tickets/new' },
    ],
  },
]

// Any navigation dismisses the mobile drawer — sidebar links close it
// themselves, but programmatic pushes (dashboard tiles, ticket rows) would
// otherwise change the page behind the still-open overlay.
watch(
  () => route.fullPath,
  () => {
    const drawer = document.getElementById('sidebar-drawer') as HTMLInputElement | null
    if (drawer) drawer.checked = false
  },
)
</script>

<template>
  <!-- Same drawer shell as StaffLayout: overlay sidebar below lg (checkbox-
       driven, no JS), permanent sidebar column on lg+. <main> is the only
       scroller. Duplicated on purpose — it's ~30 lines of stable markup. -->
  <div class="drawer lg:drawer-open h-dvh">
    <input id="sidebar-drawer" type="checkbox" class="drawer-toggle" />

    <div class="drawer-content flex flex-col min-h-0">
      <header class="navbar bg-base-100 border-b border-base-300 min-h-[4rem] lg:hidden sticky top-0 z-30 pad-safe-top">
        <div class="grid grid-cols-[1fr_auto_1fr] items-center w-full">
          <div class="justify-self-start">
            <label for="sidebar-drawer" class="btn btn-square btn-ghost" aria-label="Open navigation menu">
              <svg xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24" class="inline-block w-6 h-6 stroke-current">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 6h16M4 12h16M4 18h16"></path>
              </svg>
            </label>
          </div>
          <span class="justify-self-center font-bold text-lg">Support</span>
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
      <AppSidebar :sections="sections" brand="Support" home="/portal/dashboard" @change-password="showPassword = true" @edit-profile="showProfile = true" />
    </div>

    <ChangePasswordModal v-if="showPassword" @close="showPassword = false" />
    <ProfileModal v-if="showProfile" @close="showProfile = false" />
  </div>
</template>
