<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { pb } from '@/pb'
import type { NotificationSendLog, NotificationTemplate } from '@/types'
import ResponsiveList, { type Column } from '@/components/ResponsiveList.vue'
import Pager from '@/components/Pager.vue'
import TemplateReferenceDrawer from '@/components/TemplateReferenceDrawer.vue'
import NatsReferenceDrawer from '@/components/NatsReferenceDrawer.vue'

// Event first: ResponsiveList promotes the first column to the mobile card
// header, and a card headlined by a raw timestamp identifies nothing.
const sendColumns: Column<NotificationSendLog>[] = [
  { key: 'event_type', label: 'Event' },
  { key: 'created', label: 'When', class: 'whitespace-nowrap', format: (v) => new Date(v).toLocaleString() },
  { key: 'channel', label: 'Channel', format: (v) => v || 'email' },
  { key: 'recipient', label: 'Recipient', mobileLabel: 'To' },
  { key: 'status', label: 'Status' },
  { key: 'payload_summary', label: 'Context' },
]

const templates = ref<NotificationTemplate[]>([])
const selectedType = ref('')
const loading = ref(true)
const saving = ref(false)
const error = ref('')
const savedFlash = ref(false)

const sends = ref<NotificationSendLog[]>([])
const sendPage = ref(1)
const sendTotalPages = ref(1)
const helpOpen = ref(false)
const natsHelpOpen = ref(false)

// Editable working copy of the selected template; extras edited as one
// address per line.
const form = ref({ enabled: true, publish_nats: false, subject: '', body: '', requester: false, assignee: false, all_staff: false, extras: '' })

const selected = computed(() => templates.value.find((t) => t.event_type === selectedType.value) || null)

function fillForm(t: NotificationTemplate) {
  form.value = {
    enabled: t.enabled,
    publish_nats: t.publish_nats,
    subject: t.subject,
    body: t.body,
    requester: t.recipients.requester,
    assignee: t.recipients.assignee,
    all_staff: t.recipients.all_staff,
    extras: (t.recipients.extras || []).join('\n'),
  }
}

function select(eventType: string) {
  selectedType.value = eventType
  const t = templates.value.find((x) => x.event_type === eventType)
  if (t) fillForm(t)
}

async function load() {
  loading.value = true
  error.value = ''
  try {
    const res = await pb.send('/api/helpdesk/notifications', { method: 'GET' })
    templates.value = res.templates
    if (templates.value.length > 0) select(selectedType.value || templates.value[0].event_type)
    await loadSends()
  } catch (err: any) {
    error.value = err?.message || 'Failed to load templates'
  } finally {
    loading.value = false
  }
}

// The send log is append-only (pruned at 90 days) — page through it rather
// than showing a fixed slice.
async function loadSends() {
  const res = await pb.collection('notification_send_log').getList<NotificationSendLog>(sendPage.value, 15, {
    sort: '-created',
  })
  sends.value = res.items
  sendTotalPages.value = res.totalPages
}
watch(sendPage, () => loadSends().catch(() => {}))

async function save() {
  if (!selected.value) return
  saving.value = true
  error.value = ''
  try {
    const updated = await pb.send(`/api/helpdesk/notifications/${selected.value.event_type}`, {
      method: 'PATCH',
      body: {
        enabled: form.value.enabled,
        publish_nats: form.value.publish_nats,
        subject: form.value.subject,
        body: form.value.body,
        recipients: {
          requester: form.value.requester,
          assignee: form.value.assignee,
          all_staff: form.value.all_staff,
          extras: form.value.extras.split('\n').map((s) => s.trim()).filter(Boolean),
        },
      },
    })
    const i = templates.value.findIndex((t) => t.event_type === updated.event_type)
    if (i >= 0) templates.value[i] = updated
    fillForm(updated)
    savedFlash.value = true
    setTimeout(() => (savedFlash.value = false), 1500)
  } catch (err: any) {
    error.value = err?.data?.message || err?.message || 'Failed to save'
  } finally {
    saving.value = false
  }
}

// Send the CURRENT editor contents (saved or not) rendered against sample
// data to the logged-in admin — a preflight before saving.
const testing = ref(false)
const testResult = ref('')
async function sendTest() {
  if (!selected.value) return
  testing.value = true
  testResult.value = ''
  error.value = ''
  try {
    const res = await pb.send(`/api/helpdesk/notifications/${selected.value.event_type}/test`, {
      method: 'POST',
      body: { subject: form.value.subject, body: form.value.body },
    })
    testResult.value = res.sent ? `Sent to ${res.to}` : `Send failed: ${res.error}`
  } catch (err: any) {
    testResult.value = ''
    error.value = err?.data?.message || err?.message || 'Test send failed'
  } finally {
    testing.value = false
  }
}

// Refill the textareas from the compiled-in defaults; nothing persists
// until Save is clicked.
async function resetToDefaults() {
  if (!selected.value) return
  error.value = ''
  try {
    const res = await pb.send(`/api/helpdesk/notifications/${selected.value.event_type}/defaults`, { method: 'GET' })
    form.value.subject = res.subject
    form.value.body = res.body
    form.value.requester = res.recipients.requester
    form.value.assignee = res.recipients.assignee
    form.value.all_staff = res.recipients.all_staff
    form.value.extras = (res.recipients.extras || []).join('\n')
  } catch (err: any) {
    error.value = err?.message || 'Failed to load defaults'
  }
}

onMounted(load)
</script>

<template>
  <div class="space-y-4">
    <div class="flex items-center justify-between gap-2">
      <h1 class="text-2xl font-bold">Notifications</h1>
      <button class="btn btn-ghost btn-sm gap-1" @click="helpOpen = true">
        <span aria-hidden="true">❔</span> Template reference
      </button>
    </div>

    <TemplateReferenceDrawer :open="helpOpen" @close="helpOpen = false" />
    <NatsReferenceDrawer :open="natsHelpOpen" :event-type="selected?.event_type || ''" @close="natsHelpOpen = false" />

    <div v-if="error" class="alert alert-error py-2 text-sm">{{ error }}</div>
    <div v-if="loading" class="flex justify-center p-12"><span class="loading loading-spinner loading-lg"></span></div>

    <div v-else class="grid grid-cols-1 lg:grid-cols-3 gap-4 items-start">
      <div class="card bg-base-100 shadow-sm">
        <div class="card-body p-3">
          <ul class="menu p-0">
            <li v-for="t in templates" :key="t.event_type">
              <a class="flex items-center gap-2" :class="{ active: t.event_type === selectedType }" @click="select(t.event_type)">
                <span class="flex-1 min-w-0 truncate">{{ t.name }}</span>
                <span class="flex items-center gap-1 shrink-0">
                  <span
                    class="badge-soft"
                    :class="t.enabled ? 'badge-soft-success' : 'badge-soft-neutral'"
                    :title="`Email ${t.enabled ? 'enabled' : 'disabled'}`"
                    :aria-label="`Email ${t.enabled ? 'enabled' : 'disabled'}`"
                  >
                    <span class="badge-dot"></span>email
                  </span>
                  <span
                    class="badge-soft"
                    :class="t.publish_nats ? 'badge-soft-success' : 'badge-soft-neutral'"
                    :title="`NATS ${t.publish_nats ? 'enabled' : 'disabled'}`"
                    :aria-label="`NATS ${t.publish_nats ? 'enabled' : 'disabled'}`"
                  >
                    <span class="badge-dot"></span>nats
                  </span>
                </span>
              </a>
            </li>
          </ul>
        </div>
      </div>

      <div v-if="selected" class="card bg-base-100 shadow-sm lg:col-span-2">
        <div class="card-body space-y-3">
          <div class="flex items-center justify-between">
            <h2 class="card-title text-base">{{ selected.name }}</h2>
            <code class="text-xs text-base-content/50">{{ selected.event_type }}</code>
          </div>

          <div class="form-control">
            <label class="label cursor-pointer justify-start gap-3 py-1">
              <input v-model="form.enabled" type="checkbox" class="toggle toggle-success toggle-sm" :disabled="saving" />
              <span class="label-text">Send email</span>
            </label>
          </div>

          <div class="form-control">
            <label class="label cursor-pointer justify-start gap-3 py-1">
              <input v-model="form.publish_nats" type="checkbox" class="toggle toggle-sm" :disabled="saving" />
              <span class="label-text">Publish to NATS</span>
            </label>
            <label v-if="form.publish_nats" class="label py-0">
              <span class="label-text-alt text-base-content/60">
                Publishes a JSON event to
                <code class="text-xs">helpdesk.&lt;customer&gt;.events.{{ selected.event_type }}</code>
                — <button type="button" class="link" @click="natsHelpOpen = true">see event format</button>
              </span>
            </label>
          </div>

          <div class="form-control">
            <label class="label py-1"><span class="label-text">Subject</span></label>
            <input v-model="form.subject" type="text" class="input input-bordered input-sm font-mono" :disabled="saving" />
          </div>

          <div class="form-control">
            <label class="label py-1"><span class="label-text">Body</span></label>
            <textarea v-model="form.body" rows="10" class="textarea textarea-bordered textarea-sm font-mono" :disabled="saving"></textarea>
            <label class="label py-1">
              <span class="label-text-alt text-base-content/60">
                Go text/template —
                <button type="button" class="link" @click="helpOpen = true">variables &amp; helpers reference</button>
              </span>
            </label>
          </div>

          <div class="form-control">
            <label class="label py-1"><span class="label-text">Recipients</span></label>
            <div class="flex flex-wrap gap-4">
              <label class="label cursor-pointer justify-start gap-2 py-0">
                <input v-model="form.requester" type="checkbox" class="checkbox checkbox-sm" :disabled="saving" />
                <span class="label-text">Requester</span>
              </label>
              <label class="label cursor-pointer justify-start gap-2 py-0">
                <input v-model="form.assignee" type="checkbox" class="checkbox checkbox-sm" :disabled="saving" />
                <span class="label-text">Assignee</span>
              </label>
              <label class="label cursor-pointer justify-start gap-2 py-0">
                <input v-model="form.all_staff" type="checkbox" class="checkbox checkbox-sm" :disabled="saving" />
                <span class="label-text">All staff</span>
              </label>
            </div>
          </div>

          <div class="form-control">
            <label class="label py-1"><span class="label-text">Extra addresses (one per line)</span></label>
            <textarea v-model="form.extras" rows="2" class="textarea textarea-bordered textarea-sm font-mono" placeholder="ops@example.com" :disabled="saving"></textarea>
          </div>

          <div class="flex flex-wrap justify-between items-center gap-2 pt-1">
            <div class="flex items-center gap-2">
              <button class="btn btn-ghost btn-sm" :disabled="saving" @click="resetToDefaults">Reset to defaults</button>
              <button class="btn btn-ghost btn-sm" :disabled="testing" @click="sendTest">
                <span v-if="testing" class="loading loading-spinner loading-xs"></span>
                Send test to me
              </button>
            </div>
            <div class="flex items-center gap-2">
              <span v-if="testResult" class="text-sm" :class="testResult.startsWith('Sent') ? 'text-success' : 'text-error'">{{ testResult }}</span>
              <span v-if="savedFlash" class="text-success text-sm">Saved</span>
              <button class="btn btn-primary btn-sm" :disabled="saving" @click="save">
                <span v-if="saving" class="loading loading-spinner loading-xs"></span>
                Save
              </button>
            </div>
          </div>
        </div>
      </div>
    </div>

    <div v-if="!loading" class="card bg-base-100 shadow-sm">
      <div class="card-body">
        <h2 class="card-title text-base">Recent sends</h2>
        <ResponsiveList :items="sends" :columns="sendColumns" :clickable="false">
          <template #cell-event_type="{ value }"><code class="text-xs">{{ value }}</code></template>
          <template #cell-status="{ item }">
            <span class="badge-soft" :class="item.status === 'sent' ? 'badge-soft-success' : item.status === 'failed' ? 'badge-soft-error' : 'badge-soft-neutral'" :title="item.error">
              {{ item.status }}
            </span>
          </template>
          <template #cell-payload_summary="{ value }"><span class="text-sm text-base-content/60">{{ value }}</span></template>
          <template #empty>
            <span class="text-sm text-base-content/50">Nothing sent yet.</span>
          </template>
        </ResponsiveList>
        <Pager v-model:page="sendPage" :total-pages="sendTotalPages" class="pt-2" />
      </div>
    </div>
  </div>
</template>
