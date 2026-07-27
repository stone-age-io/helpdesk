import { createApp } from 'vue'
import { createPinia } from 'pinia'
import App from './App.vue'
import router from './router'
import { initTheme } from './theme'
import { useBrandingStore } from './stores/branding'
import './style.css'

// Theme is applied synchronously before mount to avoid a flash of the wrong one.
initTheme()

const app = createApp(App)
app.use(createPinia())
app.use(router)

// Load the operator branding overlay before mounting so the app name/logo and
// browser tab title are correct on first paint. Defensively caught so a failed
// fetch can't block mount (the store keeps its defaults).
const brandingStore = useBrandingStore()
brandingStore
  .load()
  .catch(err => console.error('Branding load failed:', err))
  .finally(() => {
    document.title = brandingStore.appName
    app.mount('#app')
  })

// PWA install support: the service worker (ui/public/sw.js) is a deliberate
// no-op — it exists only to satisfy the browser's install criteria, so there
// is no offline cache to go stale. Skipped in dev.
if ('serviceWorker' in navigator && import.meta.env.PROD) {
  window.addEventListener('load', () => {
    navigator.serviceWorker
      .register('/sw.js')
      .catch(err => console.error('SW registration failed:', err))
  })
}
