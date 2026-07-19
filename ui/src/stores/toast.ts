import { defineStore } from 'pinia'
import { ref } from 'vue'

// Ephemeral, app-wide toast notifications. A single <Toaster> (mounted in
// App.vue) renders whatever this store holds; any view calls success/error/info
// to surface transient feedback that otherwise happens silently (a ticket
// created, a reply sent, an export finished). Deliberately tiny — no
// persistence, no queue limit worth enforcing at portal scale.
export type ToastKind = 'success' | 'error' | 'info'

export interface Toast {
  id: number
  message: string
  kind: ToastKind
}

export const useToastStore = defineStore('toast', () => {
  const toasts = ref<Toast[]>([])
  let seq = 0

  function dismiss(id: number) {
    toasts.value = toasts.value.filter((t) => t.id !== id)
  }

  // ttl <= 0 keeps the toast until dismissed (clicked). Errors linger longer
  // than confirmations since they're more likely to matter.
  function push(message: string, kind: ToastKind = 'success', ttl = 4000): number {
    const id = ++seq
    toasts.value.push({ id, message, kind })
    if (ttl > 0) setTimeout(() => dismiss(id), ttl)
    return id
  }

  const success = (m: string) => push(m, 'success', 4000)
  const error = (m: string) => push(m, 'error', 6000)
  const info = (m: string) => push(m, 'info', 4000)

  return { toasts, push, dismiss, success, error, info }
})
