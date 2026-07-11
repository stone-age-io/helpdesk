<script setup lang="ts">
// Role-neutral ticket deep link (/t/{id}) used by notification emails:
// forwards to the staff or portal detail view based on who is logged in.
// Anonymous visitors go through login and are bounced back here after.
import { onMounted } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useAuthStore } from '@/stores/auth'

const route = useRoute()
const router = useRouter()
const auth = useAuthStore()

onMounted(() => {
  const id = route.params.id as string
  if (auth.isStaff) {
    router.replace(`/staff/tickets/${id}`)
  } else if (auth.isRequester) {
    router.replace(`/portal/tickets/${id}`)
  } else {
    router.replace({ name: 'login', query: { redirect: route.fullPath } })
  }
})
</script>

<template>
  <div class="min-h-screen flex items-center justify-center">
    <span class="loading loading-spinner loading-lg"></span>
  </div>
</template>
