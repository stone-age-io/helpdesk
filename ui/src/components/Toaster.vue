<script setup lang="ts">
// Renders the toast store. Mounted once at the app root so any view can raise a
// toast without wiring. Top-end placement keeps clear of the mobile sticky
// composer (bottom) and reads as a transient confirmation; click to dismiss.
import { useToastStore } from '@/stores/toast'
import type { ToastKind } from '@/stores/toast'

const toast = useToastStore()

const kindClass: Record<ToastKind, string> = {
  success: 'alert-success',
  error: 'alert-error',
  info: 'alert-info',
}
const kindIcon: Record<ToastKind, string> = {
  success: '✓',
  error: '⚠',
  info: 'ℹ',
}
</script>

<template>
  <div class="toast toast-top toast-end z-[100] pad-safe-top max-w-[90vw]">
    <TransitionGroup name="toast">
      <div
        v-for="t in toast.toasts"
        :key="t.id"
        class="alert shadow-lg text-sm cursor-pointer max-w-sm"
        :class="kindClass[t.kind]"
        role="status"
        aria-live="polite"
        @click="toast.dismiss(t.id)"
      >
        <span aria-hidden="true">{{ kindIcon[t.kind] }}</span>
        <span class="min-w-0 break-words">{{ t.message }}</span>
      </div>
    </TransitionGroup>
  </div>
</template>

<style scoped>
.toast-enter-active,
.toast-leave-active {
  transition:
    opacity 0.2s ease,
    transform 0.2s ease;
}
.toast-enter-from,
.toast-leave-to {
  opacity: 0;
  transform: translateY(-0.5rem);
}
</style>
