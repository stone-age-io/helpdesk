<script setup lang="ts">
// Slide-in reference for the NATS channel: shows the subject pattern and a
// representative JSON envelope for one event. Unlike the template reference,
// this content is NOT hand-maintained — it's fetched from
// GET /api/helpdesk/notifications/{event_type}/nats-sample, which the backend
// renders from the same code that publishes (SampleEnvelope), so it can't
// drift from the wire contract.
import { computed, ref, watch } from 'vue'
import { pb } from '@/pb'

const props = defineProps<{ open: boolean; eventType: string }>()
const emit = defineEmits<{ (e: 'close'): void }>()

interface Sample {
  event_type: string
  subject: string
  envelope: unknown
}

const sample = ref<Sample | null>(null)
const loading = ref(false)
const error = ref('')

const prettyEnvelope = computed(() =>
  sample.value ? JSON.stringify(sample.value.envelope, null, 2) : '',
)

async function fetchSample() {
  if (!props.eventType) return
  loading.value = true
  error.value = ''
  sample.value = null
  try {
    sample.value = await pb.send(`/api/helpdesk/notifications/${props.eventType}/nats-sample`, {
      method: 'GET',
    })
  } catch (err: any) {
    error.value = err?.data?.message || err?.message || 'Failed to load sample'
  } finally {
    loading.value = false
  }
}

// Fetch when the drawer opens, and re-fetch if the event changes while open.
watch(
  () => [props.open, props.eventType],
  ([isOpen]) => {
    if (isOpen) fetchSample()
  },
)
</script>

<template>
  <teleport to="body">
    <transition name="drawer-fade">
      <div v-if="open" class="fixed inset-0 z-[60]" role="dialog" aria-modal="true" aria-label="NATS event format">
        <div class="absolute inset-0 bg-black/40" @click="emit('close')"></div>
        <transition name="drawer-slide">
          <aside
            v-if="open"
            class="absolute right-0 top-0 h-full w-full max-w-md bg-base-100 shadow-xl border-l border-base-300 flex flex-col"
          >
            <div class="flex items-center justify-between px-4 py-3 border-b border-base-300 flex-none">
              <h3 class="font-bold text-base">NATS event format</h3>
              <button class="btn btn-ghost btn-sm btn-circle" aria-label="Close" @click="emit('close')">✕</button>
            </div>

            <div class="overflow-y-auto px-4 py-3 space-y-4 text-sm">
              <p class="text-base-content/70">
                When <span class="font-medium">Publish to NATS</span> is on, this event publishes a fixed,
                versioned JSON envelope (<code class="text-xs">schema: helpdesk.event</code>, v1). The shape is
                code-defined — not editable — and the fields below are exactly what this event carries;
                other events differ.
              </p>

              <div v-if="loading" class="flex justify-center p-8">
                <span class="loading loading-spinner loading-md"></span>
              </div>
              <div v-else-if="error" class="alert alert-error py-2 text-sm">{{ error }}</div>

              <template v-else-if="sample">
                <section class="space-y-1.5">
                  <h4 class="font-semibold text-xs uppercase tracking-wider text-base-content/60">Subject</h4>
                  <code class="block text-xs bg-base-200 px-2 py-1.5 rounded break-all">{{ sample.subject }}</code>
                  <p class="text-xs text-base-content/60">
                    <code class="text-xs">&lt;customer&gt;</code> is the ticket's customer id. Consumers can
                    filter with <code class="text-xs">helpdesk.*.events.&gt;</code>.
                  </p>
                </section>

                <section class="space-y-1.5">
                  <h4 class="font-semibold text-xs uppercase tracking-wider text-base-content/60">Payload</h4>
                  <pre class="text-xs bg-base-200 rounded p-2 overflow-x-auto"><code>{{ prettyEnvelope }}</code></pre>
                </section>
              </template>
            </div>
          </aside>
        </transition>
      </div>
    </transition>
  </teleport>
</template>

<style scoped>
.drawer-fade-enter-active,
.drawer-fade-leave-active {
  transition: opacity 0.2s ease;
}
.drawer-fade-enter-from,
.drawer-fade-leave-to {
  opacity: 0;
}
.drawer-slide-enter-active,
.drawer-slide-leave-active {
  transition: transform 0.2s ease;
}
.drawer-slide-enter-from,
.drawer-slide-leave-to {
  transform: translateX(100%);
}
</style>
