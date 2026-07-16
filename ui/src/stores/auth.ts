import { defineStore } from 'pinia'
import { computed, ref } from 'vue'
import { pb } from '@/pb'

// Two identity classes share one login page: staff (agents/admins) and
// requesters (`users` collection, customer-scoped). The authenticated
// collection name decides which app shell the router shows.
export const useAuthStore = defineStore('auth', () => {
  const record = ref<any>(pb.authStore.record)

  pb.authStore.onChange(() => {
    record.value = pb.authStore.record
  })

  const isAuthenticated = computed(() => pb.authStore.isValid && !!record.value)
  const isStaff = computed(() => record.value?.collectionName === 'staff')
  const isAdmin = computed(() => isStaff.value && record.value?.role === 'admin')
  // Field agents are staff on a mobile, on-site shell (migration 1816000000).
  // Same access as an agent — this only steers which app shell/route they land
  // in, so `isStaff` stays true and every staff route/rule still applies.
  const isField = computed(() => isStaff.value && record.value?.role === 'field')
  const isRequester = computed(() => record.value?.collectionName === 'users')
  // Avatar initial for the shells (sidebar, portal navbar).
  const initial = computed(() =>
    (record.value?.name || record.value?.email || '?').slice(0, 1).toUpperCase(),
  )

  async function login(email: string, password: string) {
    // Try staff first, fall back to requesters. Both failing surfaces the
    // requester error (identical "invalid credentials" shape either way).
    try {
      await pb.collection('staff').authWithPassword(email, password)
      return
    } catch {
      await pb.collection('users').authWithPassword(email, password)
    }
  }

  function logout() {
    pb.authStore.clear()
  }

  return { record, isAuthenticated, isStaff, isAdmin, isField, isRequester, initial, login, logout }
})
