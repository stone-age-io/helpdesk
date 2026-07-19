import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'
import { fileURLToPath, URL } from 'node:url'

// Build output goes straight into the Go embed directory (committed), so
// `go build` never needs npm — the access-control convention.
export default defineConfig({
  plugins: [vue()],
  resolve: {
    alias: {
      '@': fileURLToPath(new URL('./src', import.meta.url)),
    },
  },
  build: {
    outDir: '../internal/webui/public',
    emptyOutDir: true,
  },
  server: {
    port: 5174,
    proxy: {
      '/api': 'http://127.0.0.1:8090',
      // Operator branding overlay (theme.css / branding.json / logo) is served
      // by the helpdesk backend, so proxy it in dev too.
      '/branding': 'http://127.0.0.1:8090',
    },
  },
})
