<script setup lang="ts">
// Address-search + map picker for a location's coordinates. Two-way binds the
// lat/lng v-models so the parent form's numeric inputs stay the source of truth
// and the map merely visualizes/edits them. Three ways to set the pin: search a
// place (explicit — Enter or the button, never debounced, to respect
// Nominatim's usage policy), click the map, or drag the pin. The basemap
// (OpenFreeMap) and the geocoder (Nominatim) are both public endpoints hit from
// the browser, so this needs internet and degrades to manual lat/lng entry when
// offline.
//
// Adapted from the access-control sibling's LocationPicker; the map setup is
// inlined (helpdesk needs one draggable pin, not the sibling's marker layer).
import { ref, computed, onMounted, onUnmounted, watch, nextTick } from 'vue'
import L from 'leaflet'
import 'leaflet/dist/leaflet.css'
// maplibre-gl is held at v5 on purpose. v5 inlines its tile-parsing worker into
// dist/maplibre-gl.js; v6 splits it out and resolves it as a sibling file
// (`new URL('./maplibre-gl-worker.mjs', import.meta.url)`) that Vite never emits
// once the library is bundled into a hashed chunk. Nothing throws and nothing
// reaches the console — the style still loads and its background layer still
// paints, so water, landuse, roads and labels vanish together and the map reads
// as a flat sheet of theme colour rather than as a failure. v5 is also the
// version OpenFreeMap's own quick start pins. Mirrors the platform's pin.
import 'maplibre-gl/dist/maplibre-gl.css'
// Side-effect import: registers L.maplibreGL and augments the leaflet module types.
import '@maplibre/maplibre-gl-leaflet'
import { fixLeafletIcons } from '@/utils/leafletIcons'
import { theme } from '@/theme'

const lat = defineModel<number>('lat', { required: true })
const lng = defineModel<number>('lng', { required: true })
// The address doubles as the geocoder input: type + Enter to search, pick a
// result to set the coordinates AND canonicalize this same text. Left as-is
// (free text) when the user doesn't search — so unresolvable locations still work.
const address = defineModel<string>('address', { required: true })

// Read-only mode (parent's view/edit toggle): the map still pans/zooms so it
// works as a location display, but the pin can't be moved and search is off.
const props = defineProps<{ disabled?: boolean }>()

// Basemap styles. Keyless, and vector rather than raster.
//
// Both of the raster sources this used had to move: CARTO put its basemaps
// behind an API key and is retiring them, and the light layer was calling
// tile.openstreetmap.org directly, which the OSMF tile usage policy does not
// allow for a product. OpenFreeMap replaces both — no key, no request cap,
// commercial use permitted, and self-hostable if a deployment ever cannot reach
// tiles.openfreemap.org. Lifted from the platform, which moved first.
//
// These are MapLibre style documents, not {z}/{x}/{y} templates (OpenFreeMap
// publishes no raster endpoint), so the basemap renders through L.maplibreGL
// onto a WebGL canvas instead of L.tileLayer. The draggable pin above it is
// unchanged Leaflet. fiord is a designed dark style, so the brightness lift a
// dark raster layer would want is neither needed nor safe: the GL canvas lands
// in .leaflet-tile-pane, so a filter there would wash out the whole basemap.
const STYLE_LIGHT = 'https://tiles.openfreemap.org/styles/bright'
const STYLE_DARK = 'https://tiles.openfreemap.org/styles/fiord'
// OpenFreeMap's style JSON carries no `attribution` on its sources, so MapLibre
// renders no credit of its own and ODbL still requires one. It goes on the map's
// attribution control at init rather than on the layer: it is the same for both
// styles, and L.maplibreGL's options are typed as MapLibre's own, which have no
// Leaflet `attribution` key.
const TILE_ATTRIBUTION =
  '&copy; <a href="https://openfreemap.org">OpenFreeMap</a> &middot; <a href="https://www.openmaptiles.org/">OpenMapTiles</a> &middot; <a href="https://www.openstreetmap.org/copyright">OpenStreetMap</a>'
const DEFAULT_CENTER: [number, number] = [39.8283, -98.5795]
const DEFAULT_ZOOM = 4

// Bind the map to the element via a template ref (L.map accepts an
// HTMLElement) — no DOM id, so no collisions and nothing that needs a secure
// context.
const mapEl = ref<HTMLElement | null>(null)

interface NominatimResult {
  display_name: string
  lat: string
  lon: string
}

const searching = ref(false)
const searchError = ref('')
const results = ref<NominatimResult[]>([])

let map: L.Map | null = null
let basemapLayer: L.MaplibreGL | null = null
let marker: L.Marker | null = null
// Recenter at most once (on the first non-zero coordinates, e.g. an edit-mode
// record load) — never on every manual keystroke.
let centered = false

const hasCoords = computed(() => (lat.value ?? 0) !== 0 || (lng.value ?? 0) !== 0)

function round6(n: number): number {
  return Math.round(n * 1e6) / 1e6
}

function setCoords(la: number, ln: number) {
  lat.value = round6(la)
  lng.value = round6(ln)
}

function applyTiles() {
  if (!map) return
  if (basemapLayer) map.removeLayer(basemapLayer)
  // attributionControl: false — the credit lives on the Leaflet control (set
  // once at init), not on a second one MapLibre would draw itself.
  basemapLayer = L.maplibreGL({
    style: theme.value === 'dark' ? STYLE_DARK : STYLE_LIGHT,
    attributionControl: false,
  })
  basemapLayer.addTo(map)
}

function placeMarker(la: number, ln: number) {
  if (!map) return
  if (!marker) {
    marker = L.marker([la, ln], { draggable: !props.disabled })
    marker.on('dragend', () => {
      const ll = marker!.getLatLng()
      setCoords(ll.lat, ll.lng)
    })
    marker.addTo(map)
  } else {
    marker.setLatLng([la, ln])
  }
}

// Reflect external model changes (record load, manual numeric input, clear) onto
// the map. Our own writes land here too but no-op via the epsilon check.
function syncMarkerFromModel() {
  if (!map) return
  if (!hasCoords.value) {
    if (marker) {
      map.removeLayer(marker)
      marker = null
    }
    centered = false
    return
  }
  const la = lat.value
  const ln = lng.value
  if (marker) {
    const ll = marker.getLatLng()
    if (Math.abs(ll.lat - la) < 1e-7 && Math.abs(ll.lng - ln) < 1e-7) return
  }
  placeMarker(la, ln)
  if (!centered) {
    map.setView([la, ln], 17)
    centered = true
  }
}

async function search() {
  const q = (address.value || '').trim()
  if (!q || searching.value || props.disabled) return
  searching.value = true
  searchError.value = ''
  results.value = []
  try {
    const url = `https://nominatim.openstreetmap.org/search?format=json&limit=5&q=${encodeURIComponent(q)}`
    const resp = await fetch(url, { headers: { Accept: 'application/json' } })
    if (!resp.ok) throw new Error(`Search failed (${resp.status})`)
    results.value = (await resp.json()) as NominatimResult[]
    if (results.value.length === 0) searchError.value = 'No matching places found.'
  } catch (err: any) {
    searchError.value = err?.message || 'Address search failed (needs internet).'
  } finally {
    searching.value = false
  }
}

function selectResult(r: NominatimResult) {
  const la = round6(parseFloat(r.lat))
  const ln = round6(parseFloat(r.lon))
  if (Number.isNaN(la) || Number.isNaN(ln)) return
  setCoords(la, ln)
  // Drive the map from the parsed values directly — reading lat/lng back here
  // returns the previous value, since the v-model prop hasn't re-flowed down yet.
  placeMarker(la, ln)
  map?.setView([la, ln], 17)
  centered = true
  results.value = []
  // Write the canonical address back into the shared field.
  address.value = r.display_name
}

function clearPin() {
  setCoords(0, 0)
}

onMounted(() => {
  if (!mapEl.value) return
  fixLeafletIcons()
  map = L.map(mapEl.value, {
    center: hasCoords.value ? [lat.value, lng.value] : DEFAULT_CENTER,
    zoom: hasCoords.value ? 17 : DEFAULT_ZOOM,
    // Was an L.tileLayer option; the GL layer supplies no zoom bounds, and
    // without this Leaflet would zoom in without limit.
    maxZoom: 19,
    zoomControl: true,
  })
  map.attributionControl.setPrefix(false)
  map.attributionControl.addAttribution(TILE_ATTRIBUTION)
  applyTiles()
  centered = hasCoords.value
  if (hasCoords.value) placeMarker(lat.value, lng.value)
  map.on('click', (e: L.LeafletMouseEvent) => {
    if (props.disabled) return
    setCoords(e.latlng.lat, e.latlng.lng)
  })
  nextTick(() => map?.invalidateSize())
})

onUnmounted(() => {
  map?.remove()
  map = null
})

watch([lat, lng], syncMarkerFromModel)
watch(theme, applyTiles)
// Toggling edit mode flips whether the pin can be dragged.
watch(() => props.disabled, (d) => {
  if (!marker) return
  if (d) marker.dragging?.disable()
  else marker.dragging?.enable()
})
</script>

<template>
  <div class="space-y-2">
    <!-- Address = the geocoder input. Explicit search (Enter or the button) per
         Nominatim's usage policy; picking a result sets the coordinates and
         canonicalizes this text. Free text is kept when not searched. -->
    <div class="form-control relative">
      <label class="label py-1"><span class="label-text">Address</span></label>
      <div class="flex gap-2">
        <input
          v-model="address"
          type="text"
          placeholder="Search or type an address…"
          class="input input-bordered input-sm flex-1"
          :disabled="disabled"
          @keydown.enter.prevent="search"
        />
        <button type="button" class="btn btn-primary btn-sm" :disabled="searching || disabled" @click="search">
          <span v-if="searching" class="loading loading-spinner loading-xs"></span>
          <span v-else>Search</span>
        </button>
      </div>
      <ul
        v-if="results.length"
        class="absolute z-[600] mt-1 w-full bg-base-100 border border-base-300 rounded-box shadow-lg max-h-60 overflow-y-auto"
      >
        <li v-for="(r, i) in results" :key="i">
          <button
            type="button"
            class="w-full text-left px-3 py-2 text-sm hover:bg-base-200 transition-colors"
            @click="selectResult(r)"
          >
            {{ r.display_name }}
          </button>
        </li>
      </ul>
    </div>
    <p v-if="searchError" class="text-xs text-error">{{ searchError }}</p>

    <!-- Map -->
    <div class="relative h-72 rounded-lg overflow-hidden border border-base-300">
      <div ref="mapEl" class="absolute inset-0 z-0"></div>
      <button
        v-if="hasCoords && !disabled"
        type="button"
        class="btn btn-xs absolute top-2 left-2 z-[400] bg-base-100/90 backdrop-blur border-base-300 shadow-sm hover:bg-base-200"
        @click="clearPin"
      >
        Clear pin
      </button>
    </div>

    <p class="text-xs leading-relaxed text-base-content/60">
      <template v-if="!disabled">Search for a place, click the map, or drag the pin to set coordinates. </template>
      Map by
      <a href="https://openfreemap.org" target="_blank" rel="noopener" class="link">OpenFreeMap</a>,
      geocoding by
      <a href="https://www.openstreetmap.org/copyright" target="_blank" rel="noopener" class="link">OpenStreetMap</a>
      (needs internet).
    </p>
  </div>
</template>
