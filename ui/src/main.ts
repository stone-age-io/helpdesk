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
