<script setup lang="ts">
// Routed component for /staff. Picks the shell by role: field agents get the
// mobile, on-site bottom-tab layout; agents/admins get the desk sidebar. Both
// layouts own the child `<router-view>`, so the same /staff/* routes render in
// whichever chrome fits the role — no duplicate route tree, no prefix-aware
// links in the reused views.
import { useAuthStore } from '@/stores/auth'
import StaffLayout from '@/components/StaffLayout.vue'
import FieldLayout from '@/components/FieldLayout.vue'

const auth = useAuthStore()
</script>

<template>
  <FieldLayout v-if="auth.isField" />
  <StaffLayout v-else />
</template>
