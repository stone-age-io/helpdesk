<script setup lang="ts">
// Navigation sidebar (drawer-side content) shared by the staff, field, and
// portal shells. Overlay drawer below lg, a permanent column on lg+ with an
// icons-only compact rail — see the layouts. Nav content arrives as `sections`
// so this stays a pure renderer; the brand/logo come from the branding overlay.
import { computed } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useAuthStore } from '@/stores/auth'
import { useBrandingStore } from '@/stores/branding'
import { sidebarCompact, toggleCompact, useMediaQuery } from '@/sidebar'
import ThemeToggle from '@/components/ThemeToggle.vue'
import Avatar from '@/components/Avatar.vue'
import BrandLogo from '@/components/BrandLogo.vue'

export interface NavItem {
  label: string
  icon: string
  path: string
  adminOnly?: boolean
}
export interface NavSection {
  title?: string
  items: NavItem[]
}

const props = withDefaults(
  defineProps<{ sections: NavSection[]; brand?: string; home?: string }>(),
  { home: '/' },
)

const route = useRoute()
const router = useRouter()
const auth = useAuthStore()
const brandingStore = useBrandingStore()

const emit = defineEmits<{ (e: 'change-password'): void; (e: 'edit-profile'): void }>()

const isLargeScreen = useMediaQuery('(min-width: 1024px)')
const effectiveCompact = computed(() => sidebarCompact.value && isLargeScreen.value)

// Brand text: an explicit prop wins (the shells differ — "Service Desk" vs
// "Support"), else the operator branding overlay's app name.
const brandText = computed(() => props.brand ?? brandingStore.appName)

// Hide admin-only items from non-admins; drop sections left empty.
const visibleSections = computed<NavSection[]>(() =>
  props.sections
    .map((s) => ({ ...s, items: s.items.filter((i) => !i.adminOnly || auth.isAdmin) }))
    .filter((s) => s.items.length > 0),
)

// Longest matching prefix wins, so /portal/tickets/new highlights "New Ticket"
// and not also "Tickets".
const activePath = computed(() => {
  let best = ''
  for (const s of visibleSections.value)
    for (const i of s.items) {
      if ((route.path === i.path || route.path.startsWith(i.path + '/')) && i.path.length > best.length) best = i.path
    }
  return best
})

// Staff have a role to show; requesters get their email as the second line.
const subtitle = computed(() => auth.record?.role || auth.record?.email || '')

// On mobile the drawer overlays the page; navigating should dismiss it. The
// checkbox is the drawer's single source of open/closed state.
function closeDrawer() {
  const drawer = document.getElementById('sidebar-drawer') as HTMLInputElement | null
  if (drawer) drawer.checked = false
}

// daisyUI dropdowns close on blur; a menu click keeps focus, so drop it.
function closeDropdown() {
  ;(document.activeElement as HTMLElement | null)?.blur()
}

function changePassword() {
  closeDropdown()
  closeDrawer()
  emit('change-password')
}

function editProfile() {
  closeDropdown()
  closeDrawer()
  emit('edit-profile')
}

function logout() {
  closeDropdown()
  auth.logout()
  closeDrawer()
  router.push('/login')
}
</script>

<template>
  <!-- min-h-full, not h-dvh: daisyUI's .drawer-side is already a 100dvh
       container and the only scroller — a second full-height box inside it
       overflows by a few pixels and grows a phantom scrollbar. -->
  <aside
    class="bg-base-100 min-h-full flex flex-col border-r border-base-300 transition-all duration-300 ease-in-out z-20 pad-safe-top"
    :class="effectiveCompact ? 'w-20 min-w-[5rem]' : 'w-72 min-w-[18rem]'"
  >
    <!-- TOP: brand + collapse toggle -->
    <div class="flex-none p-3 pb-0">
      <div
        class="flex transition-all duration-300"
        :class="effectiveCompact ? 'flex-col items-center gap-3 py-2' : 'flex-row items-center justify-between px-2 py-2'"
      >
        <router-link :to="home" class="flex items-center gap-3 hover:opacity-80 transition-opacity overflow-hidden" @click="closeDrawer">
          <div class="w-10 h-10 flex items-center justify-center flex-shrink-0 text-primary">
            <BrandLogo :size="36" />
          </div>
          <span v-show="!effectiveCompact" class="font-bold text-lg tracking-tight whitespace-nowrap overflow-hidden">
            {{ brandText }}
          </span>
        </router-link>

        <button
          v-if="isLargeScreen"
          @click="toggleCompact"
          class="btn btn-ghost btn-sm btn-square opacity-60 hover:opacity-100 transition-opacity"
          :title="sidebarCompact ? 'Expand sidebar' : 'Collapse sidebar'"
          :aria-label="sidebarCompact ? 'Expand sidebar' : 'Collapse sidebar'"
        >
          <span v-if="sidebarCompact">»</span>
          <span v-else>«</span>
        </button>
      </div>
      <div class="divider my-0"></div>
    </div>

    <!-- NAVIGATION -->
    <nav class="flex-1 overflow-y-auto overflow-x-hidden px-3 pb-2">
      <ul class="menu p-0 gap-1 w-full">
        <template v-for="(section, si) in visibleSections" :key="si">
          <li v-if="section.title && !effectiveCompact" class="menu-title px-2 pt-3 pb-1 text-[10px] uppercase tracking-widest opacity-50">
            {{ section.title }}
          </li>
          <li v-else-if="section.title && effectiveCompact" class="py-1">
            <div class="divider my-0"></div>
          </li>
          <li v-for="item in section.items" :key="item.path">
            <router-link
              :to="item.path"
              :class="{ active: item.path === activePath }"
              class="group relative"
              @click="closeDrawer"
            >
              <div
                v-if="effectiveCompact"
                class="tooltip tooltip-right absolute left-0 w-full h-full"
                :data-tip="item.label"
              ></div>
              <span class="text-lg opacity-80 w-6 text-center">{{ item.icon }}</span>
              <span v-show="!effectiveCompact" class="font-medium truncate">{{ item.label }}</span>
            </router-link>
          </li>
        </template>
      </ul>
    </nav>

    <!-- BOTTOM: theme + account (dropdown opens upward over the nav) -->
    <div class="flex-none p-3 pt-0 flex flex-col gap-1 pad-safe-bottom">
      <div class="divider my-0"></div>

      <ThemeToggle v-if="!effectiveCompact" row />
      <div v-else class="flex justify-center"><ThemeToggle /></div>

      <div class="dropdown dropdown-top w-full">
        <div
          tabindex="0"
          role="button"
          class="flex items-center gap-3 w-full p-2 rounded-lg bg-base-200/50 hover:bg-base-200 cursor-pointer transition-colors"
          :class="{ 'justify-center': effectiveCompact }"
        >
          <Avatar :record="auth.record" :name="auth.record?.name || auth.record?.email" size="sm" />
          <template v-if="!effectiveCompact">
            <div class="flex flex-col truncate flex-1 text-left min-w-0">
              <span class="font-semibold text-sm truncate leading-tight">{{ auth.record?.name || auth.record?.email }}</span>
              <span class="text-xs text-base-content/60 truncate leading-tight">{{ subtitle }}</span>
            </div>
            <span class="text-base-content/40 text-lg leading-none pr-1">⋮</span>
          </template>
        </div>
        <ul tabindex="0" class="dropdown-content menu menu-sm bg-base-100 rounded-box shadow-lg border border-base-300 w-52 p-1 mb-1 z-50">
          <li><a @click="editProfile">Edit profile</a></li>
          <li><a @click="changePassword">Change password</a></li>
          <li><a class="text-error" @click="logout">Sign out</a></li>
        </ul>
      </div>
    </div>
  </aside>
</template>
