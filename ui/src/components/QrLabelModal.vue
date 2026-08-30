<script setup lang="ts">
// A printable, operator-branded QR label for one thing or location, sized to
// real label stock.
//
// The payload is the bare `code` — no host, no organization, no kind token
// (ADR 0002 in platform-docs). Three consequences shape this component:
//
//   Maximum error correction is affordable. A short code at EC level H (30%
//   recovery) is a version-1 or version-2 symbol; the URL form of the same
//   identifier needs version 6, roughly four times the modules on an
//   identically sized sticker. A label lives on a device in a plant room and
//   gets scratched, painted and wiped, so that headroom goes to recovery.
//
//   The human-readable line is not decoration. It is the fallback for the label
//   that will not scan — greasy, torn, or in a closet too dark to focus in —
//   where a tech reads the code aloud or types it into the scanner's manual
//   field. It gets equal billing.
//
//   The customer's name is deliberately NOT printed. A sticker in a public
//   hallway is readable by anyone walking past, and a tenant name beside a
//   device naming convention is free reconnaissance. It is shown on screen,
//   where the operator already knows whose device they are looking at.
//
// The operator's brand IS printed, and it is the one piece of context that
// earns its space: whoever finds a broken device needs to know who services it.
//
// ── Sizing ───────────────────────────────────────────────────────────────────
//
// Everything below is in MILLIMETRES, not pixels. CSS mm maps to physical mm at
// print, so the artwork comes off the printer at the size of the stock instead
// of at whatever the browser happens to map 96 px/inch to. Screen preview is a
// side effect of the same units and is close enough to judge.
//
// Both sizes reserve an RFID INLAY KEEP-OUT: a clear band across the centre of
// the label, where the chip on a length-wise UHF dipole sits. Printing over the
// chip bump causes print voids and can crack the die under a thermal head. The
// artwork therefore straddles it — QR to one side, text to the other, nothing
// in the middle — so the SAME layout prints correctly on plain stock and on
// RFID stock. Toggling "RFID stock" only reveals the reserved zone so it can be
// checked against a specific inlay's datasheet; it does not change the layout.
//
// Inlay geometry varies by vendor. These are conservative defaults for a
// centred UHF dipole, not a guarantee for any particular stock.
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import QRCode from 'qrcode'
import { useBrandingStore } from '@/stores/branding'

interface LabelSize {
  key: '2x1'
  label: string
  /** Stock dimensions, millimetres. */
  w: number
  h: number
  /** Printer registration margin held clear on every edge. */
  pad: number
  /** Square QR side. Height-bound, and must clear the keep-out horizontally. */
  qr: number
  /** Width of the centred clear band reserved for the inlay chip. */
  keepOut: number
  /** Point sizes — print-native units, so they survive the mm layout. */
  brandPt: number
  codePt: number
  namePt: number
}

// Geometry is checked rather than eyeballed: with a 4-module quiet zone, the
// worst-case symbol among realistic codes is 33 modules across. At these QR
// sizes that is 0.61 mm per module on the small stock and 1.21 mm on the large
// — both above the ~0.5 mm a phone camera needs.
const SIZES = [
  {
    key: '2x1',
    label: '2″ × 1″',
    w: 50.8,
    h: 25.4,
    pad: 1.5,
    qr: 20,
    keepOut: 7,
    brandPt: 4,
    codePt: 8,
    namePt: 5,
  },
  {
    key: '4x2',
    label: '4″ × 2″',
    w: 101.6,
    h: 50.8,
    pad: 3,
    qr: 40,
    keepOut: 12,
    brandPt: 7,
    codePt: 16,
    namePt: 9,
  },
] as unknown as LabelSize[]

const props = defineProps<{
  code: string
  name: string
  kind: 'thing' | 'location'
  customerName?: string
}>()
const emit = defineEmits<{ close: [] }>()

const branding = useBrandingStore()
const dataUrl = ref('')
const error = ref('')
const sizeKey = ref<string>(SIZES[0].key)
const showKeepOut = ref(false)

const size = computed(() => SIZES.find((s) => s.key === sizeKey.value) ?? SIZES[0])

// Absolute placement inside a fixed-size box: the most predictable thing a
// print engine can be handed, and it makes the keep-out arithmetic legible.
const keepOutLeft = computed(() => size.value.w / 2 - size.value.keepOut / 2)
const textLeft = computed(() => keepOutLeft.value + size.value.keepOut + 1)

const labelStyle = computed(() => ({
  width: `${size.value.w}mm`,
  height: `${size.value.h}mm`,
}))
const qrStyle = computed(() => ({
  left: `${size.value.pad}mm`,
  width: `${size.value.qr}mm`,
  height: `${size.value.qr}mm`,
}))
const textStyle = computed(() => ({
  left: `${textLeft.value}mm`,
  right: `${size.value.pad}mm`,
}))
const keepOutStyle = computed(() => ({
  left: `${keepOutLeft.value}mm`,
  width: `${size.value.keepOut}mm`,
}))

async function render() {
  error.value = ''
  if (!props.code) {
    error.value = 'This record has no code, so it cannot carry a label. Add one first.'
    dataUrl.value = ''
    return
  }
  try {
    dataUrl.value = await QRCode.toDataURL(props.code, {
      errorCorrectionLevel: 'H',
      // 4 is the quiet zone ISO/IEC 18004 requires, and the library's default.
      // Carrying it inside the image rather than relying on surrounding white
      // space means the clearance is guaranteed by the symbol itself, whatever
      // the label layout does around it.
      margin: 4,
      // Rendered far larger than it prints so the symbol is crisp at any DPI;
      // the mm box below is what sets the physical size.
      width: 1024,
      color: { dark: '#000000', light: '#ffffff' },
    })
  } catch (err: any) {
    error.value = err?.message || 'Failed to render the QR code'
  }
}

// @page cannot be expressed in a scoped style block or interpolated from a
// template, so the page box is written imperatively and torn down with the
// modal. Without it the browser prints the label onto whatever paper size is
// selected, complete with its default margins.
let pageStyleEl: HTMLStyleElement | null = null
function applyPageSize() {
  if (!pageStyleEl) {
    pageStyleEl = document.createElement('style')
    pageStyleEl.id = 'qr-label-page-size'
    document.head.appendChild(pageStyleEl)
  }
  pageStyleEl.textContent = `@page { size: ${size.value.w}mm ${size.value.h}mm; margin: 0; }`
}

onMounted(() => {
  render()
  applyPageSize()
})
watch(() => props.code, render)
watch(size, applyPageSize)
onBeforeUnmount(() => {
  pageStyleEl?.remove()
  pageStyleEl = null
})

// window is not in template scope under <script setup>.
function print() {
  window.print()
}
</script>

<template>
  <dialog class="modal modal-open">
    <div class="modal-box max-w-lg">
      <h3 class="font-bold text-lg mb-1">Label</h3>
      <p class="text-sm text-base-content/60 mb-4">
        Scanned in the field app or the platform console.
        <span v-if="customerName">{{ customerName }}.</span>
      </p>

      <div v-if="error" class="alert alert-warning py-2 text-sm">{{ error }}</div>

      <template v-if="dataUrl">
        <div class="flex items-center gap-4 mb-3 flex-wrap">
          <div class="join">
            <button
              v-for="s in SIZES"
              :key="s.key"
              class="btn btn-sm join-item"
              :class="sizeKey === s.key ? 'btn-active' : ''"
              @click="sizeKey = s.key"
            >
              {{ s.label }}
            </button>
          </div>
          <label class="label cursor-pointer gap-2 py-0">
            <input v-model="showKeepOut" type="checkbox" class="toggle toggle-sm" />
            <span class="label-text text-sm">RFID stock</span>
          </label>
        </div>

        <!--
          qr-print-area is the only thing the print stylesheet leaves visible.
          Colours are pinned rather than themed: this goes onto white stock, so
          a dark-theme label would print as a black rectangle.
        -->
        <div class="overflow-x-auto">
          <div class="qr-print-area relative bg-white text-black" :style="labelStyle">
            <img :src="dataUrl" alt="" class="qr-label__qr absolute top-1/2 -translate-y-1/2" :style="qrStyle" />

            <div class="qr-label__text absolute top-1/2 -translate-y-1/2 overflow-hidden" :style="textStyle">
              <div class="flex items-center gap-1 leading-none">
                <img v-if="branding.logoUrl" :src="branding.logoUrl" alt="" class="object-contain"
                     :style="{ height: `${size.brandPt}pt`, width: `${size.brandPt}pt` }" />
                <span class="uppercase tracking-wide font-semibold truncate"
                      :style="{ fontSize: `${size.brandPt}pt` }">{{ branding.appName }}</span>
              </div>
              <div class="font-mono font-bold leading-tight break-all"
                   :style="{ fontSize: `${size.codePt}pt`, marginTop: '0.6mm' }">{{ code }}</div>
              <div class="leading-tight break-words"
                   :style="{ fontSize: `${size.namePt}pt`, marginTop: '0.3mm' }">{{ name }}</div>
              <!--
                Things dominate an inventory, so only the rarer kind is marked —
                an unmarked label reads as a device, which is the common case.
              -->
              <div v-if="kind === 'location'" class="uppercase tracking-wide leading-none"
                   :style="{ fontSize: `${size.brandPt}pt`, marginTop: '0.4mm' }">Site</div>
            </div>

            <!-- Guide only: reserved on both stocks, drawn on request, never printed. -->
            <div v-if="showKeepOut" class="qr-label__keepout absolute inset-y-0 border-x border-dashed border-error/70 bg-error/10"
                 :style="keepOutStyle"></div>
          </div>
        </div>

        <p v-if="showKeepOut" class="text-xs text-base-content/60 mt-2">
          The dashed band is held clear for the RFID inlay chip and is never printed. It is a
          conservative default for a centred UHF dipole — check it against your stock's datasheet.
        </p>
      </template>

      <div class="modal-action mt-4">
        <button class="btn btn-ghost btn-sm" @click="emit('close')">Close</button>
        <button class="btn btn-primary btn-sm" :disabled="!dataUrl" @click="print">Print</button>
      </div>
    </div>
    <form method="dialog" class="modal-backdrop"><button @click.prevent="emit('close')">close</button></form>
  </dialog>
</template>

<!--
  Deliberately NOT scoped. The print rule has to reach every element on the page
  to hide it, which a scoped attribute selector cannot do. It is inert until this
  modal mounts, and the modal only mounts while it is open. The @page box itself
  is written from script — see applyPageSize.
-->
<style>
@media print {
  body * {
    visibility: hidden;
  }
  .qr-print-area,
  .qr-print-area * {
    visibility: visible;
  }
  .qr-print-area {
    position: absolute;
    top: 0;
    left: 0;
    /* The stock IS the page, so no offset and no border. */
    margin: 0;
    border: none !important;
  }
  /* A guide, not artwork. */
  .qr-label__keepout {
    display: none !important;
  }
}
</style>
