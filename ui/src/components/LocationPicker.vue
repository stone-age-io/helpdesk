<script setup lang="ts">
// Address-search + map picker for a location's coordinates. Two-way binds the
// lat/lng v-models so the parent form's numeric inputs stay the source of truth
// and the map merely visualizes/edits them. Three ways to set the pin: search a
// place (explicit — Enter or the button, never debounced, to respect
// Nominatim's usage policy), click the map, or drag the pin. Both the tiles and
// the geocoder are public OSM endpoints hit from the browser, so this needs
// internet and degrades to manual lat/lng entry when offline.
//
// Adapted from the access-control sibling's LocationPicker; the map setup is
// inlined (helpdesk needs one draggable pin, not the sibling's marker layer).
import { ref, computed, onMounted, onUnmounted, watch, nextTick } from 'vue'
import L from 'leaflet'
import 'leaflet/dist/leaflet.css'
import { fixLeafletIcons } from '@/utils/leafletIcons'
import { theme } from '@/theme'

const lat = defineModel<number>('lat', { required: true })
const lng = defineModel<number>('lng', { required: true })

// Read-only mode (parent's view/edit toggle): the map still pans/zooms so it
// works as a location display, but the pin can't be moved and search is off.
const props = defineProps<{ disabled?: boolean }>()

const TILE_LIGHT = 'https://{s}.tile.openstreetmap.org/{z}/{x}/{y}.png'
const TILE_DARK = 'https://{s}.basemaps.cartocdn.com/dark_all/{z}/{x}/{y}{r}.png'
const TILE_ATTRIBUTION =
  '&copy; <a href="https://www.openstreetmap.org/copyright">OpenStreetMap</a> contributors &copy; <a href="https://carto.com/attributions">CARTO</a>'
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

const searchQuery = ref('')
const searching = ref(false)
const searchError = ref('')
const results = ref<NominatimResult[]>([])

let map: L.Map | null = null
let tileLayer: L.TileLayer | null = null
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
  if (tileLayer) map.removeLayer(tileLayer)
  tileLayer = L.tileLayer(theme.value === 'dark' ? TILE_DARK : TILE_LIGHT, {
    attribution: TILE_ATTRIBUTION,
    maxZoom: 19,
  })
  tileLayer.addTo(map)
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
  const q = searchQuery.value.trim()
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
  searchQuery.value = r.display_name
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
    zoomControl: true,
  })
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
    <!-- Address search (explicit: Enter or the button) -->
    <div class="relative">
      <div class="flex gap-2">
        <input
          v-model="searchQuery"
          type="text"
          placeholder="Search an address or place…"
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
      Map &amp; geocoding by
      <a href="https://www.openstreetmap.org/copyright" target="_blank" rel="noopener" class="link">OpenStreetMap</a>
      (needs internet).
    </p>
  </div>
</template>
