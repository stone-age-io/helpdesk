<script setup lang="ts">
import { theme, toggleTheme } from '@/theme'
</script>

<template>
  <button class="btn btn-ghost btn-sm btn-circle" :title="theme === 'dark' ? 'Switch to light mode' : 'Switch to dark mode'" @click="toggleTheme">
    <span v-if="theme === 'dark'">☀️</span>
    <span v-else>🌙</span>
  </button>
</template>
