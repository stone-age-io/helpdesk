<script setup lang="ts">
// Camera scanner for the field app. Emits the decoded string; resolving it is
// the caller's job.
//
// In-app scanning is the whole point, not a convenience (ADR 0002). The QR
// payload is a bare code rather than a URL specifically so that a sticker
// someone forged and stuck over ours cannot send a human to arbitrary content —
// the worst a forged label achieves is resolving a different record inside an
// already-authenticated session. That property only holds if the decoded string
// is never treated as a destination, which is why this emits a string and never
// navigates on its own.
//
// Manual entry is not a fallback bolted on: it is half the design. Every label
// prints its code in human-readable text precisely because the symbol will
// eventually be scratched, greasy, or in a closet too dark to focus in, and a
// tech standing at the thing needs a way through that does not involve walking
// back to a laptop.
import { onBeforeUnmount, ref } from 'vue'
import { Html5Qrcode, Html5QrcodeSupportedFormats } from 'html5-qrcode'

const emit = defineEmits<{ decoded: [value: string] }>()

const state = ref<'idle' | 'scanning' | 'error'>('idle')
const error = ref('')
const manual = ref('')
const readerId = 'qr-scanner-reader'

let scanner: Html5Qrcode | null = null

async function stop() {
  if (!scanner) return
  try {
    // isScanning guards the common double-stop: the user taps Cancel while the
    // decode callback is already tearing things down.
    if (scanner.isScanning) await scanner.stop()
    scanner.clear()
  } catch {
    // Nothing useful to do — the camera is going away either way.
  }
  scanner = null
}

async function start() {
  error.value = ''
  state.value = 'scanning'
  // The viewfinder element must exist and be laid out before the library
  // measures it, and v-if only renders it once state flips above.
  await new Promise((r) => setTimeout(r, 50))
  try {
    scanner = new Html5Qrcode(readerId, {
      formatsToSupport: [Html5QrcodeSupportedFormats.QR_CODE],
      verbose: false,
    })
    await scanner.start(
      { facingMode: 'environment' },
      { fps: 10, qrbox: { width: 240, height: 240 } },
      async (text) => {
        // Stop before emitting: the parent may navigate, and a live camera
        // stream outliving its viewfinder leaves the torch on.
        await stop()
        state.value = 'idle'
        emit('decoded', text.trim())
      },
      () => {
        // Per-frame decode misses are the normal case, not errors.
      },
    )
  } catch (err: any) {
    state.value = 'error'
    error.value =
      err?.message?.includes('Permission') || err?.name === 'NotAllowedError'
        ? 'Camera permission was denied. Allow it in your browser settings, or type the code below.'
        : err?.message || 'Could not start the camera. Type the code below instead.'
    await stop()
  }
}

async function cancel() {
  await stop()
  state.value = 'idle'
}

function submitManual() {
  const value = manual.value.trim()
  if (!value) return
  manual.value = ''
  emit('decoded', value)
}

onBeforeUnmount(stop)
</script>

<template>
  <div class="space-y-3">
    <div v-if="state === 'scanning'" class="space-y-2">
      <div :id="readerId" class="w-full rounded-lg overflow-hidden bg-black"></div>
      <button class="btn btn-ghost btn-sm w-full" @click="cancel">Cancel</button>
    </div>

    <button v-else class="btn btn-primary w-full gap-2" @click="start">
      <span class="text-lg" aria-hidden="true">📷</span>
      Scan a label
    </button>

    <div v-if="error" class="alert alert-warning py-2 text-sm">{{ error }}</div>

    <div v-if="state !== 'scanning'" class="form-control">
      <label class="label py-1"><span class="label-text text-sm">Or type the code</span></label>
      <div class="join">
        <input
          v-model="manual"
          type="text"
          placeholder="DOOR-1"
          class="input input-bordered input-sm join-item flex-1 font-mono text-base-content"
          @keydown.enter="submitManual"
        />
        <button class="btn btn-sm join-item" :disabled="!manual.trim()" @click="submitManual">Go</button>
      </div>
    </div>
  </div>
</template>
