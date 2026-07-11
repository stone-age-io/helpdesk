<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { pb } from '@/pb'
import { useAuthStore } from '@/stores/auth'
import type { Customer, Requester, Staff, Ticket, TicketComment } from '@/types'
import { TICKET_PRIORITIES, TICKET_STATUSES } from '@/types'
import TicketBadges from '@/components/TicketBadges.vue'
import TimeEntriesCard from '@/components/TimeEntriesCard.vue'
import VisitsCard from '@/components/VisitsCard.vue'
import ActivityCard from '@/components/ActivityCard.vue'
import SearchSelect from '@/components/SearchSelect.vue'
import AttachmentList from '@/components/AttachmentList.vue'
import FileInput from '@/components/FileInput.vue'
import { format } from 'date-fns'

const route = useRoute()
const router = useRouter()
const auth = useAuthStore()
const id = route.params.id as string

const ticket = ref<Ticket | null>(null)
const comments = ref<TicketComment[]>([])
const staff = ref<Staff[]>([])
const customers = ref<Customer[]>([])
const requesters = ref<Requester[]>([])
const loading = ref(true)
const error = ref('')

const newComment = ref('')
const internalNote = ref(false)
const commentFiles = ref<File[]>([])
const posting = ref(false)
// When off, field edits are saved without emailing the requester — for
// triage cleanup, a mis-set status, or an internal reassignment.
const notify = ref(true)

// Inline title/body editing.
const editingHeader = ref(false)
const editTitle = ref('')
const editBody = ref('')

const staffOptions = computed(() => staff.value.map((s) => ({ id: s.id, label: s.name, sublabel: s.email })))
const customerOptions = computed(() => customers.value.map((c) => ({ id: c.id, label: c.name })))
const requesterOptions = computed(() =>
  requesters.value.map((r) => ({ id: r.id, label: r.name || r.email, sublabel: r.name ? r.email : undefined })),
)

async function loadTicket() {
  ticket.value = await pb.collection('tickets').getOne<Ticket>(id, {
    expand: 'customer,assignee,requester',
  })
}

async function loadRequesters(customerId: string) {
  requesters.value = customerId
    ? await pb.collection('users').getFullList<Requester>({ filter: `customer = '${customerId}'`, sort: 'name' })
    : []
}

async function loadComments() {
  comments.value = await pb.collection('ticket_comments').getFullList<TicketComment>({
    filter: `ticket = '${id}'`,
    sort: 'created',
    expand: 'author_staff,author_user',
  })
}

async function load() {
  loading.value = true
  error.value = ''
  try {
    await Promise.all([loadTicket(), loadComments()])
    staff.value = await pb.collection('staff').getFullList<Staff>({ sort: 'name', filter: 'active = true' })
    customers.value = await pb.collection('customers').getFullList<Customer>({ sort: 'name' })
    await loadRequesters(ticket.value?.customer || '')
  } catch (err: any) {
    error.value = err?.message || 'Failed to load ticket'
  } finally {
    loading.value = false
  }
}

// updateField carries the quiet-notify intent (status/assignee can email).
async function updateField(field: 'status' | 'priority' | 'assignee', value: string) {
  if (!ticket.value) return
  try {
    ticket.value = await pb.collection('tickets').update<Ticket>(
      id,
      { [field]: value },
      {
        expand: 'customer,assignee,requester',
        // The backend hook reads this header and skips the outbound email.
        headers: notify.value ? {} : { 'X-Helpdesk-Quiet': '1' },
      },
    )
  } catch (err: any) {
    error.value = err?.message || `Failed to update ${field}`
  }
}

// patchPlain saves fields that never trigger email (title/body/customer/
// requester), so no quiet header is needed.
async function patchPlain(fields: Record<string, string>) {
  if (!ticket.value) return
  try {
    ticket.value = await pb.collection('tickets').update<Ticket>(id, fields, {
      expand: 'customer,assignee,requester',
    })
  } catch (err: any) {
    error.value = err?.message || 'Failed to save'
  }
}

function startEditHeader() {
  if (!ticket.value) return
  editTitle.value = ticket.value.title
  editBody.value = ticket.value.body || ''
  editingHeader.value = true
}
async function saveHeader() {
  if (!editTitle.value.trim()) return
  await patchPlain({ title: editTitle.value.trim(), body: editBody.value.trim() })
  editingHeader.value = false
}

// Changing customer clears the requester (it must belong to the customer),
// then reloads the requester picker for the new company.
async function changeCustomer(value: string) {
  if (!value || value === ticket.value?.customer) return
  await patchPlain({ customer: value, requester: '' })
  await loadRequesters(value)
}

async function postComment() {
  if (!newComment.value.trim()) return
  posting.value = true
  try {
    await pb.collection('ticket_comments').create({
      ticket: id,
      author_staff: auth.record?.id,
      body: newComment.value.trim(),
      internal: internalNote.value,
      attachments: commentFiles.value,
    })
    newComment.value = ''
    internalNote.value = false
    commentFiles.value = []
    await loadComments()
  } catch (err: any) {
    error.value = err?.message || 'Failed to post comment'
  } finally {
    posting.value = false
  }
}

function authorLabel(c: TicketComment): string {
  const s = c.expand?.author_staff
  if (s) return s.name || s.email
  const u = c.expand?.author_user
  if (u) return u.name || u.email
  return 'System'
}

// Live updates: another agent's reply, a status change, or a requester
// comment lands without a manual refresh. Debounced to collapse bursts.
let reloadTimer: ReturnType<typeof setTimeout> | undefined
function scheduleReload() {
  clearTimeout(reloadTimer)
  reloadTimer = setTimeout(() => {
    loadTicket().catch(() => {})
    loadComments().catch(() => {})
  }, 500)
}
let unsubTicket: (() => void) | null = null
let unsubComments: (() => void) | null = null

onMounted(async () => {
  await load()
  try {
    unsubTicket = await pb.collection('tickets').subscribe(id, scheduleReload)
    unsubComments = await pb.collection('ticket_comments').subscribe('*', scheduleReload)
  } catch {
    // Realtime is progressive enhancement; the view works without it.
  }
})

onUnmounted(() => {
  clearTimeout(reloadTimer)
  unsubTicket?.()
  unsubComments?.()
})
</script>

<template>
  <div v-if="loading" class="flex justify-center p-12"><span class="loading loading-spinner loading-lg"></span></div>
  <div v-else-if="error && !ticket" class="alert alert-error">{{ error }}</div>

  <div v-else-if="ticket" class="space-y-4">
    <div class="breadcrumbs text-sm">
      <ul>
        <li><a @click="router.push('/staff/tickets')">Tickets</a></li>
        <li>#{{ ticket.number }}</li>
      </ul>
    </div>

    <div v-if="error" class="alert alert-error py-2 text-sm">{{ error }}</div>

    <!-- Two columns only from xl up: at lg the permanent nav sidebar already
         takes ~16rem, so a side-by-side ticket sidebar would squeeze the
         thread to a sliver. -->
    <div class="flex flex-col xl:flex-row gap-4 items-start">
      <!-- Main column -->
      <div class="flex-1 space-y-4 w-full">
        <div class="card bg-base-100 shadow-sm">
          <div class="card-body">
            <template v-if="!editingHeader">
              <div class="flex items-start gap-2 flex-wrap">
                <h1 class="text-xl font-bold flex-1">#{{ ticket.number }} — {{ ticket.title }}</h1>
                <TicketBadges :status="ticket.status" :priority="ticket.priority" />
                <button class="btn btn-ghost btn-xs" @click="startEditHeader">Edit</button>
              </div>
              <p v-if="ticket.body" class="whitespace-pre-wrap text-sm mt-2">{{ ticket.body }}</p>
              <AttachmentList :record="ticket" :files="ticket.attachments" />
              <p v-if="ticket.origin_subject" class="text-xs font-mono text-base-content/50 mt-2">
                via {{ ticket.origin_subject }}
              </p>
            </template>
            <template v-else>
              <input v-model="editTitle" type="text" maxlength="300" class="input input-bordered input-sm w-full font-bold" />
              <textarea v-model="editBody" rows="5" class="textarea textarea-bordered w-full mt-2" placeholder="Details"></textarea>
              <div class="flex justify-end gap-2 mt-2">
                <button class="btn btn-ghost btn-sm" @click="editingHeader = false">Cancel</button>
                <button class="btn btn-primary btn-sm" :disabled="!editTitle.trim()" @click="saveHeader">Save</button>
              </div>
            </template>
          </div>
        </div>

        <!-- Thread -->
        <div class="space-y-2">
          <div
            v-for="c in comments"
            :key="c.id"
            class="card shadow-sm"
            :class="c.internal ? 'bg-warning/10 border border-warning/30' : 'bg-base-100'"
          >
            <div class="card-body py-3 px-4">
              <div class="flex items-center gap-2 text-xs text-base-content/60">
                <span class="font-semibold text-base-content">{{ authorLabel(c) }}</span>
                <span v-if="c.internal" class="badge badge-warning badge-xs">internal</span>
                <span>{{ format(new Date(c.created), 'MMM d, yyyy HH:mm') }}</span>
              </div>
              <p class="whitespace-pre-wrap text-sm">{{ c.body }}</p>
              <AttachmentList :record="c" :files="c.attachments" />
            </div>
          </div>
          <p v-if="comments.length === 0" class="text-sm text-base-content/50 px-1">No comments yet.</p>
        </div>

        <!-- Composer -->
        <div class="card bg-base-100 shadow-sm">
          <div class="card-body py-3 px-4 space-y-2">
            <textarea
              v-model="newComment"
              rows="3"
              class="textarea textarea-bordered w-full"
              placeholder="Write a reply…"
              :disabled="posting"
            ></textarea>
            <FileInput v-model:files="commentFiles" :disabled="posting" />
            <div class="flex justify-between items-center">
              <label class="label cursor-pointer gap-2">
                <input v-model="internalNote" type="checkbox" class="checkbox checkbox-sm checkbox-warning" :disabled="posting" />
                <span class="label-text text-sm">Internal note (hidden from requester)</span>
              </label>
              <button class="btn btn-primary btn-sm" :disabled="posting || !newComment.trim()" @click="postComment">
                <span v-if="posting" class="loading loading-spinner loading-xs"></span>
                Post
              </button>
            </div>
          </div>
        </div>
      </div>

      <!-- Sidebar -->
      <div class="w-full xl:w-80 space-y-4">
        <div class="card bg-base-100 shadow-sm">
          <div class="card-body py-4 px-4 space-y-3">
            <div class="form-control">
              <label class="label py-1">
                <span class="label-text text-xs">Customer</span>
                <router-link v-if="ticket.expand?.customer" :to="`/staff/customers/${ticket.customer}`" class="label-text-alt link link-hover">view →</router-link>
              </label>
              <SearchSelect
                :model-value="ticket.customer || ''"
                :options="customerOptions"
                size="sm"
                placeholder="Type a customer…"
                @update:model-value="changeCustomer"
              />
            </div>
            <div class="form-control">
              <label class="label py-1"><span class="label-text text-xs">Requester</span></label>
              <SearchSelect
                :model-value="ticket.requester || ''"
                :options="requesterOptions"
                size="sm"
                empty-label="None"
                placeholder="Type a name or email…"
                @update:model-value="patchPlain({ requester: $event })"
              />
            </div>
            <div>
              <div class="text-xs text-base-content/60">Source</div>
              <div class="text-sm">{{ ticket.source }}</div>
            </div>
            <label class="label cursor-pointer justify-start gap-2 py-1">
              <input v-model="notify" type="checkbox" class="toggle toggle-sm toggle-primary" />
              <span class="label-text text-xs">Email requester on changes</span>
            </label>
            <div class="form-control">
              <label class="label py-1"><span class="label-text text-xs">Status</span></label>
              <select class="select select-bordered select-sm" :value="ticket.status" @change="updateField('status', ($event.target as HTMLSelectElement).value)">
                <option v-for="s in TICKET_STATUSES" :key="s" :value="s">{{ s.replace('_', ' ') }}</option>
              </select>
            </div>
            <div class="form-control">
              <label class="label py-1"><span class="label-text text-xs">Priority</span></label>
              <select class="select select-bordered select-sm" :value="ticket.priority" @change="updateField('priority', ($event.target as HTMLSelectElement).value)">
                <option v-for="p in TICKET_PRIORITIES" :key="p" :value="p">{{ p }}</option>
              </select>
            </div>
            <div class="form-control">
              <label class="label py-1"><span class="label-text text-xs">Assignee</span></label>
              <SearchSelect
                :model-value="ticket.assignee || ''"
                :options="staffOptions"
                size="sm"
                empty-label="Unassigned"
                placeholder="Type a name…"
                @update:model-value="updateField('assignee', $event)"
              />
            </div>
          </div>
        </div>

        <TimeEntriesCard :ticket-id="id" />
        <VisitsCard :ticket-id="id" :staff="staff" />
        <ActivityCard :ticket-id="id" />
      </div>
    </div>
  </div>
</template>
