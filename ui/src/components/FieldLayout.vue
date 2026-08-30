<script setup lang="ts">
// Mobile-first shell for field agents (staff role `field`, migration 1816000000),
// responsive both ways:
//   - phones: a thumb-reachable bottom tab bar + a compact top header.
//   - desktop (lg+): the shared AppSidebar on the left + a wider content column,
//     so it reads as a real app instead of a phone stranded mid-screen.
// Same /staff/* child routes as StaffLayout — only the chrome differs. The
// running-timer strip stays pinned since the timer is the field agent's main
// tool.
import { computed, onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useAuthStore } from '@/stores/auth'
import { useBrandingStore } from '@/stores/branding'
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

// Same label as the staff shell — the field view is that app on a phone, not a
// separate product. Replaced wholesale by an operator overlay's app name.
const branding = useBrandingStore()
const brandText = computed(() => branding.shellName('Service Desk'))

// Five thumb targets is the most a phone bottom bar takes, and the field app has
// eight destinations. The four a tech touches during a shift keep their slots;
// the fifth is a door to the rest (FieldMoreView).
//
// Projects lost its tab to make room. That is the honest trade: it is the one
// destination here that is read between jobs rather than during one, and it is
// one tap away behind More instead of zero.
const tabs = [
  { label: 'Today', icon: '📋', path: '/staff/today' },
  { label: 'Schedule', icon: '📅', path: '/staff/schedule' },
  { label: 'Tickets', icon: '🎫', path: '/staff/tickets' },
  { label: 'Time', icon: '⏱️', path: '/staff/my-time' },
  { label: 'More', icon: '⋯', path: '/staff/more' },
]

// Desktop has a sidebar, so nothing needs to hide behind More there — the hub's
// contents are listed flat. Same sections as the desk shell's Directory, minus
// the surfaces (Customers, Requesters) a tech has no on-site use for.
const fieldSections: NavSection[] = [
  {
    items: [
      { label: 'Today', icon: '📋', path: '/staff/today' },
      { label: 'Schedule', icon: '📅', path: '/staff/schedule' },
      { label: 'Tickets', icon: '🎫', path: '/staff/tickets' },
      { label: 'Projects', icon: '📁', path: '/staff/projects' },
      { label: 'Time', icon: '⏱️', path: '/staff/my-time' },
    ],
  },
  {
    title: 'Directory',
    items: [
      { label: 'Scan', icon: '📷', path: '/staff/scan' },
      { label: 'Sites', icon: '📍', path: '/staff/locations' },
      { label: 'Devices', icon: '🧰', path: '/staff/things' },
    ],
  },
]

// Destinations that live behind More rather than on the bar. They keep the More
// tab lit, so a tech deep in Devices can still see where they are — an
// unlit bar reads as "nowhere", which is exactly the wrong answer three taps
// into a hub.
const behindMore = ['/staff/scan', '/staff/locations', '/staff/things', '/staff/projects']

// None of the tab paths prefix one another, so a prefix match keeps the tab lit
// on its detail routes (e.g. /staff/tickets/30 → Tickets).
function isActive(path: string): boolean {
  const on = (p: string) => route.path === p || route.path.startsWith(p + '/')
  if (path === '/staff/more') return on(path) || behindMore.some(on)
  return on(path)
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
        :brand="brandText"
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
          <span class="justify-self-center font-bold text-lg truncate">{{ brandText }}</span>
          <div class="justify-self-end flex items-center">
            <!-- Header, not a sixth bottom tab: five thumb targets is already the
                 most a phone takes, and scanning is an action rather than a
                 place. -->
            <RouterLink
              to="/staff/scan"
              class="btn btn-ghost btn-sm px-2"
              aria-label="Scan a label"
              title="Scan a label"
            >
              <span class="text-lg" aria-hidden="true">📷</span>
            </RouterLink>
            <ThemeToggle />
          </div>
        </div>
      </header>

      <TimerBar />

      <!-- Same generous width as the desk shell so reused table views (Tickets,
           Projects) get full room; the field-native list views cap themselves. -->
      <main class="flex-1 min-h-0 overflow-y-auto overscroll-y-contain">
        <div class="mx-auto w-full max-w-7xl p-4 lg:p-6 pad-safe-bottom">
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
