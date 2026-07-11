<script setup lang="ts">
// One home for the theme icon/label mapping. Default is the compact icon
// button (navbars); `row` renders a full-width labeled row (sidebar).
import { theme, toggleTheme } from '@/theme'

defineProps<{ row?: boolean }>()
</script>

<template>
  <button
    v-if="row"
    class="flex items-center gap-3 w-full p-2 rounded-lg hover:bg-base-200 transition-colors text-sm"
    aria-label="Toggle light/dark theme"
    @click="toggleTheme"
  >
    <span class="w-6 text-center text-lg">{{ theme === 'dark' ? '☀️' : '🌙' }}</span>
    <span class="font-medium">{{ theme === 'dark' ? 'Light mode' : 'Dark mode' }}</span>
  </button>
  <button
    v-else
    class="btn btn-ghost btn-sm btn-circle"
    :title="theme === 'dark' ? 'Switch to light mode' : 'Switch to dark mode'"
    @click="toggleTheme"
  >
    <span v-if="theme === 'dark'">☀️</span>
    <span v-else>🌙</span>
  </button>
</template>
