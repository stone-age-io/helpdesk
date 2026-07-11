<script setup lang="ts">
// Staff navigation sidebar (drawer-side content). Overlay drawer below lg,
// permanent column on lg+ — see StaffLayout. Pattern lifted from the
// access-control sibling.
import { computed } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useAuthStore } from '@/stores/auth'
import ThemeToggle from '@/components/ThemeToggle.vue'

const route = useRoute()
const router = useRouter()
const auth = useAuthStore()

const emit = defineEmits<{ (e: 'change-password'): void }>()

interface NavItem { label: string; icon: string; path: string; adminOnly?: boolean }
interface NavSection { title?: string; items: NavItem[] }

const sections: NavSection[] = [
  {
    items: [
      { label: 'Dashboard', icon: '📊', path: '/staff/dashboard' },
      { label: 'Tickets', icon: '🎫', path: '/staff/tickets' },
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

// Hide admin-only items from agents; drop sections left empty.
const visibleSections = computed<NavSection[]>(() =>
  sections
    .map((s) => ({ ...s, items: s.items.filter((i) => !i.adminOnly || auth.isAdmin) }))
    .filter((s) => s.items.length > 0),
)

function isActive(path: string): boolean {
  return route.path === path || route.path.startsWith(path + '/')
}

// On mobile the drawer overlays the page; navigating should dismiss it. The
// checkbox is the drawer's single source of open/closed state.
function closeDrawer() {
  const drawer = document.getElementById('sidebar-drawer') as HTMLInputElement | null
  if (drawer) drawer.checked = false
}

function changePassword() {
  closeDrawer()
  emit('change-password')
}

function logout() {
  auth.logout()
  closeDrawer()
  router.push('/login')
}
</script>

<template>
  <aside class="bg-base-100 h-dvh w-64 min-w-[16rem] flex flex-col border-r border-base-300 pad-safe-top">
    <!-- TOP: brand -->
    <div class="flex-none p-3 pb-0">
      <router-link
        to="/staff/tickets"
        class="flex items-center gap-2 px-2 py-2 hover:opacity-80 transition-opacity"
        @click="closeDrawer"
      >
        <span class="text-2xl">🛟</span>
        <span class="font-bold text-lg tracking-tight">Helpdesk</span>
      </router-link>
      <div class="divider my-0"></div>
    </div>

    <!-- NAVIGATION -->
    <nav class="flex-1 overflow-y-auto overflow-x-hidden px-3 pb-2">
      <ul class="menu p-0 gap-1 w-full">
        <template v-for="(section, si) in visibleSections" :key="si">
          <li v-if="section.title" class="menu-title px-2 pt-3 pb-1 text-[10px] uppercase tracking-widest opacity-50">
            {{ section.title }}
          </li>
          <li v-for="item in section.items" :key="item.path">
            <router-link :to="item.path" :class="{ active: isActive(item.path) }" @click="closeDrawer">
              <span class="text-lg opacity-80 w-6 text-center">{{ item.icon }}</span>
              <span class="font-medium truncate">{{ item.label }}</span>
            </router-link>
          </li>
        </template>
      </ul>
    </nav>

    <!-- BOTTOM: theme + account -->
    <div class="flex-none p-3 pt-0 flex flex-col gap-1 pad-safe-bottom">
      <div class="divider my-0"></div>

      <ThemeToggle row />

      <div class="flex items-center gap-3 w-full p-2 rounded-lg bg-base-200/50">
        <div class="avatar placeholder">
          <div class="bg-neutral text-neutral-content rounded-full w-8">
            <span class="text-xs font-bold">{{ auth.initial }}</span>
          </div>
        </div>
        <div class="flex flex-col truncate flex-1 text-left min-w-0">
          <span class="font-semibold text-sm truncate leading-tight">{{ auth.record?.name || auth.record?.email }}</span>
          <span class="text-xs text-base-content/60 truncate leading-tight">{{ auth.record?.role }}</span>
        </div>
      </div>

      <div class="flex gap-1">
        <button class="btn btn-ghost btn-xs flex-1" @click="changePassword">Change password</button>
        <button class="btn btn-ghost btn-xs flex-1 text-error" @click="logout">Sign out</button>
      </div>
    </div>
  </aside>
</template>
