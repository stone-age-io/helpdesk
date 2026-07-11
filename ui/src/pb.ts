import PocketBase from 'pocketbase'

// Same-origin in production (SPA is embedded in the Go binary); the Vite dev
// server proxies /api to the backend.
export const pb = new PocketBase('/')

// Concurrent list loads on one view are expected; don't cancel them.
pb.autoCancellation(false)
