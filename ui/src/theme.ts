// Light/dark theme state: daisyUI switches on the html data-theme
// attribute. Preference persists in localStorage; first visit follows the
// OS setting.
import { ref } from 'vue'

const STORAGE_KEY = 'helpdesk-theme'

export const theme = ref<'light' | 'dark'>('light')

export function initTheme() {
  const stored = localStorage.getItem(STORAGE_KEY)
  if (stored === 'light' || stored === 'dark') {
    theme.value = stored
  } else if (window.matchMedia?.('(prefers-color-scheme: dark)').matches) {
    theme.value = 'dark'
  }
  apply()
}

export function toggleTheme() {
  theme.value = theme.value === 'dark' ? 'light' : 'dark'
  localStorage.setItem(STORAGE_KEY, theme.value)
  apply()
}

function apply() {
  document.documentElement.setAttribute('data-theme', theme.value)
}
