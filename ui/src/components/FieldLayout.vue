<script setup lang="ts">
// Mobile-first shell for field agents (staff role `field`, migration 1816000000),
// responsive both ways:
//   - phones: a thumb-reachable bottom tab bar + a compact top header.
//   - desktop (lg+): the shared AppSidebar on the left + a wider content column,
//     so it reads as a real app instead of a phone stranded mid-screen.
// Same /staff/* child routes as StaffLayout — only the chrome differs. The
// running-timer strip stays pinned since the timer is the field agent's main
// tool.
import { onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useAuthStore } from '@/stores/auth'
import { useTimerStore } from '@/stores/timer'
import AppSidebar, { type NavSection } from '@/components/AppSidebar.vue'
import Avatar from '@/components/Avatar.vue'
import ThemeToggle from '@/components/ThemeToggle.vue'
import TimerBar from '@/components/TimerBar.vue'
import ChangePasswordModal from '@/components/ChangePasswordModal.vue'
import ProfileModal from '@/components/ProfileModal.vue'

const route = useRoute()
const router = useRouter()
const auth = useAuthStore()
const timer = useTimerStore()

const showPassword = ref(false)
const showProfile = ref(false)

const tabs = [
  { label: 'Today', icon: '📋', path: '/staff/today' },
  { label: 'Schedule', icon: '📅', path: '/staff/schedule' },
  { label: 'Tickets', icon: '🎫', path: '/staff/tickets' },
  { label: 'Projects', icon: '📁', path: '/staff/projects' },
  { label: 'Time', icon: '⏱️', path: '/staff/my-time' },
]
// The desktop sidebar shows the same destinations.
const fieldSections: NavSection[] = [{ items: tabs }]

// None of the tab paths prefix one another, so a prefix match keeps the tab lit
// on its detail routes (e.g. /staff/tickets/30 → Tickets).
function isActive(path: string): boolean {
  return route.path === path || route.path.startsWith(path + '/')
}

function closeDropdown() {
  ;(document.activeElement as HTMLElement | null)?.blur()
}
function editProfile() {
  closeDropdown()
  showProfile.value = true
}
function changePassword() {
  closeDropdown()
  showPassword.value = true
}
function logout() {
  closeDropdown()
  auth.logout()
  router.push('/login')
}

onMounted(() => timer.load())
</script>

<template>
  <div class="flex h-dvh bg-base-200">
    <!-- Desktop: permanent sidebar (reused). Hidden on phones. -->
    <div class="hidden lg:block shrink-0">
      <AppSidebar
        :sections="fieldSections"
        brand="Service Desk"
        icon="🛠️"
        home="/staff/today"
        @change-password="showPassword = true"
        @edit-profile="showProfile = true"
      />
    </div>

    <div class="flex flex-col flex-1 min-w-0">
      <!-- Phone header: account · brand · theme. Hidden on desktop (sidebar owns
           the account menu there). -->
      <header class="lg:hidden navbar bg-base-100 border-b border-base-300 min-h-[3.5rem] sticky top-0 z-30 pad-safe-top">
        <div class="grid grid-cols-[1fr_auto_1fr] items-center w-full px-1">
          <div class="justify-self-start dropdown">
            <div tabindex="0" role="button" class="btn btn-ghost btn-sm px-1" aria-label="Account menu">
              <Avatar :record="auth.record" :name="auth.record?.name || auth.record?.email" size="sm" />
            </div>
            <ul tabindex="0" class="dropdown-content menu menu-sm bg-base-100 rounded-box shadow-lg border border-base-300 w-52 p-1 mt-1 z-50">
              <li class="menu-title px-2 py-1 text-xs">
                <span class="truncate">{{ auth.record?.name || auth.record?.email }}</span>
              </li>
              <li><a @click="editProfile">Edit profile</a></li>
              <li><a @click="changePassword">Change password</a></li>
              <li><a class="text-error" @click="logout">Sign out</a></li>
            </ul>
          </div>
          <span class="justify-self-center font-bold text-lg">Service Desk</span>
          <div class="justify-self-end">
            <ThemeToggle />
          </div>
        </div>
      </header>

      <TimerBar />

      <main class="flex-1 min-h-0 overflow-y-auto overscroll-y-contain">
        <div class="mx-auto w-full max-w-3xl p-4 lg:p-6 pad-safe-bottom">
          <router-view />
        </div>
      </main>

      <!-- Bottom tab bar: 5 thumb targets, phones only. -->
      <nav class="lg:hidden flex-none grid grid-cols-5 bg-base-100 border-t border-base-300 pad-safe-bottom" aria-label="Primary">
        <router-link
          v-for="tab in tabs"
          :key="tab.path"
          :to="tab.path"
          class="flex flex-col items-center justify-center gap-0.5 py-2 text-[10px] font-medium transition-colors"
          :class="isActive(tab.path) ? 'text-primary' : 'text-base-content/60'"
          :aria-current="isActive(tab.path) ? 'page' : undefined"
        >
          <span class="text-xl leading-none" aria-hidden="true">{{ tab.icon }}</span>
          {{ tab.label }}
        </router-link>
      </nav>
    </div>

    <ChangePasswordModal v-if="showPassword" @close="showPassword = false" />
    <ProfileModal v-if="showProfile" @close="showProfile = false" />
  </div>
</template>
