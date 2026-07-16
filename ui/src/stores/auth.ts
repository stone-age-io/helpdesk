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

  // Read the reactive `record` FIRST. `pb.authStore.isValid` is a plain getter
  // (not reactive); if it's evaluated first and is false (logged-out state),
  // `&&` short-circuits and `record` is never read, so this computed never
  // tracks it as a dependency. It would then stay cached at false after an
  // interactive login and the router guard would bounce every push back to
  // /login — until a full reload rebuilt the computed with a valid token
  // ("works on refresh, not interactively").
  const isAuthenticated = computed(() => !!record.value && pb.authStore.isValid)
  const isStaff = computed(() => record.value?.collectionName === 'staff')
  const isAdmin = computed(() => isStaff.value && record.value?.role === 'admin')
  // Field agents are staff on a mobile, on-site shell (migration 1816000000).
  // Same access as an agent — this only steers which app shell/route they land
  // in, so `isStaff` stays true and every staff route/rule still applies.
  const isField = computed(() => isStaff.value && record.value?.role === 'field')
  const isRequester = computed(() => record.value?.collectionName === 'users')
  // Where this identity belongs after login / on a misrouted navigation. Single
  // source of truth so LoginView and the router guard can't drift (field agents
  // were landing on the agent dashboard when only LoginView was updated).
  const homePath = computed(() => {
    if (isField.value) return '/staff/today'
    if (isStaff.value) return '/staff/dashboard'
    if (isRequester.value) return '/portal/dashboard'
    return '/login'
  })
  // Avatar initial for the shells (sidebar, portal navbar).
  const initial = computed(() =>
    (record.value?.name || record.value?.email || '?').slice(0, 1).toUpperCase(),
  )

  async function login(email: string, password: string) {
    // Try staff first, fall back to requesters. Both failing surfaces the
    // requester error (identical "invalid credentials" shape either way).
    try {
      await pb.collection('staff').authWithPassword(email, password)
    } catch {
      await pb.collection('users').authWithPassword(email, password)
    }
    // Reflect the new auth into our reactive state *now*, synchronously, rather
    // than waiting on the pb.authStore.onChange callback to propagate. Otherwise
    // LoginView reads a stale `homePath` (still '/login') right after awaiting
    // this, pushes to the current route (a no-op), and appears stuck on login
    // until a refresh rehydrates the store from localStorage.
    record.value = pb.authStore.record
  }

  function logout() {
    pb.authStore.clear()
  }

  return { record, isAuthenticated, isStaff, isAdmin, isField, isRequester, homePath, initial, login, logout }
})
