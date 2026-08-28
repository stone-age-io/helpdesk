import { defineStore } from 'pinia'
import { ref } from 'vue'

// The built-in app name, shown until (and unless) an operator branding overlay
// provides its own. Overridden at runtime by /branding/branding.json.
const DEFAULT_APP_NAME = 'Service Desk'

interface BrandingManifest {
  appName?: string
  logo?: string
}

// Operator branding overlay: the app name + logo are fetched once at boot from
// /branding/branding.json (served by the helpdesk backend from the configured
// branding.dir, with a silent empty {} fallback when unconfigured). The theme
// colors ride a separate /branding/theme.css <link> in index.html — no JS.
export const useBrandingStore = defineStore('branding', () => {
  const appName = ref<string>(DEFAULT_APP_NAME)
  const logoUrl = ref<string | null>(null)
  // Whether appName came from the overlay or is still the built-in default.
  // shellName() is the only reason this is tracked separately — see below.
  const nameFromOverlay = ref(false)

  async function load() {
    try {
      const res = await fetch('/branding/branding.json', { cache: 'no-cache' })
      if (res.ok) {
        const manifest = (await res.json()) as BrandingManifest
        if (manifest.appName) {
          appName.value = manifest.appName
          nameFromOverlay.value = true
        }
        if (manifest.logo) logoUrl.value = `/branding/${manifest.logo}`
      }
    } catch {
      // No branding overlay configured — defaults stand.
    }
  }

  // The wordmark for one shell. Each shell has its own stock label — the staff
  // app says "Service Desk", the portal says "Support" — because on a stock
  // install those read better than one name repeated in both places.
  //
  // An operator's appName REPLACES that label rather than sitting beside it.
  // Otherwise a branded install shows the operator's logo next to our stock
  // name, which reads as someone else's product. This is the one place the
  // distinction matters, which is why nameFromOverlay exists at all: appName
  // alone can't tell "operator chose this" from "nobody configured anything".
  function shellName(fallback: string): string {
    return nameFromOverlay.value ? appName.value : fallback
  }

  return { appName, logoUrl, nameFromOverlay, shellName, load }
})
