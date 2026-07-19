// Sidebar UI state. The helpdesk has no @vueuse/core, so the desktop compact-rail
// toggle and a minimal reactive media query live here, mirroring theme.ts's
// module-ref shape.
import { onMounted, onUnmounted, ref } from 'vue'

const STORAGE_KEY = 'helpdesk-sidebar-compact'

// Desktop icons-only rail preference; persists across sessions. Only takes effect
// on lg+ — the mobile drawer always shows the full-width sidebar.
export const sidebarCompact = ref(localStorage.getItem(STORAGE_KEY) === '1')

export function toggleCompact() {
  sidebarCompact.value = !sidebarCompact.value
  localStorage.setItem(STORAGE_KEY, sidebarCompact.value ? '1' : '0')
}

// Minimal reactive media query — a tiny stand-in for @vueuse/core's useMediaQuery.
export function useMediaQuery(query: string) {
  const mql = window.matchMedia(query)
  const matches = ref(mql.matches)
  const onChange = (e: MediaQueryListEvent) => (matches.value = e.matches)
  onMounted(() => mql.addEventListener('change', onChange))
  onUnmounted(() => mql.removeEventListener('change', onChange))
  return matches
}
