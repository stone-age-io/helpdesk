<script setup lang="ts">
import { onMounted, onUnmounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { pb } from '@/pb'
import { useAuthStore } from '@/stores/auth'
import type { Ticket, TicketComment, Visit } from '@/types'
import TicketBadges from '@/components/TicketBadges.vue'
import AttachmentList from '@/components/AttachmentList.vue'
import FileInput from '@/components/FileInput.vue'
import { format } from 'date-fns'

const route = useRoute()
const router = useRouter()
const auth = useAuthStore()
const id = route.params.id as string

const ticket = ref<Ticket | null>(null)
const comments = ref<TicketComment[]>([])
const visits = ref<Visit[]>([])
const loading = ref(true)
const error = ref('')

const newComment = ref('')
const commentFiles = ref<File[]>([])
const posting = ref(false)

async function loadTicket() {
  ticket.value = await pb.collection('tickets').getOne<Ticket>(id)
}

async function loadComments() {
  // Rules already exclude internal notes for requesters.
  comments.value = await pb.collection('ticket_comments').getFullList<TicketComment>({
    filter: `ticket = '${id}'`,
    sort: 'created',
    expand: 'author_staff,author_user',
  })
}

async function loadVisits() {
  // Read-only context: canceled visits are noise here. No expand — the
  // technician's staff record isn't readable (or shown) portal-side.
  try {
    visits.value = await pb.collection('visits').getFullList<Visit>({
      filter: `ticket = '${id}' && status != 'canceled'`,
      sort: 'scheduled_at',
    })
  } catch {
    // Optional context card.
  }
}

async function load() {
  loading.value = true
  try {
    await loadTicket()
    await loadComments()
    await loadVisits()
  } catch (err: any) {
    error.value = err?.message || 'Failed to load ticket'
  } finally {
    loading.value = false
  }
}

async function postComment() {
  if (!newComment.value.trim()) return
  posting.value = true
  error.value = ''
  try {
    await pb.collection('ticket_comments').create({
      ticket: id,
      author_user: auth.record?.id,
      body: newComment.value.trim(),
      attachments: commentFiles.value,
    })
    newComment.value = ''
    commentFiles.value = []
    // Replying on a resolved/closed ticket reopens it server-side; refresh
    // the header so the requester sees the status flip back to open.
    await Promise.all([loadTicket(), loadComments()])
  } catch (err: any) {
    error.value = err?.message || 'Failed to post comment'
  } finally {
    posting.value = false
  }
}

// Live updates: a staff reply or status change appears without a refresh.
let reloadTimer: ReturnType<typeof setTimeout> | undefined
function scheduleReload() {
  clearTimeout(reloadTimer)
  reloadTimer = setTimeout(() => {
    loadTicket().catch(() => {})
    loadComments().catch(() => {})
    loadVisits()
  }, 500)
}
let unsubTicket: (() => void) | null = null
let unsubComments: (() => void) | null = null

const visitBadge: Record<string, string> = {
  requested: 'badge-warning',
  scheduled: 'badge-info',
  completed: 'badge-success',
}

function authorLabel(c: TicketComment): string {
  const s = c.expand?.author_staff
  if (s) return `${s.name || 'Support'} (support)`
  const u = c.expand?.author_user
  if (u) return u.name || u.email
  return 'System'
}

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
        <li><a @click="router.push('/portal/tickets')">Tickets</a></li>
        <li>#{{ ticket.number }}</li>
      </ul>
    </div>

    <div class="card bg-base-100 shadow-sm">
      <div class="card-body">
        <div class="flex items-center gap-2 flex-wrap">
          <h1 class="text-xl font-bold">#{{ ticket.number }} — {{ ticket.title }}</h1>
          <TicketBadges :status="ticket.status" />
        </div>
        <p v-if="ticket.body" class="whitespace-pre-wrap text-sm mt-2">{{ ticket.body }}</p>
        <AttachmentList :record="ticket" :files="ticket.attachments" />
      </div>
    </div>

    <div v-if="visits.length > 0" class="card bg-base-100 shadow-sm">
      <div class="card-body py-3 px-4 space-y-2">
        <h2 class="font-semibold text-sm">Site Visits</h2>
        <ul class="space-y-2">
          <li v-for="v in visits" :key="v.id" class="text-sm space-y-0.5">
            <div class="flex items-center gap-2">
              <template v-if="v.status === 'requested'">
                <span class="italic text-base-content/70">On-site visit requested — scheduling in progress</span>
              </template>
              <template v-else>
                <span class="font-medium whitespace-nowrap">{{ v.scheduled_at ? format(new Date(v.scheduled_at), 'EEE, MMM d HH:mm') : '' }}</span>
              </template>
              <span class="badge badge-xs" :class="visitBadge[v.status]">{{ v.status }}</span>
            </div>
            <div v-if="v.location" class="text-xs text-base-content/60">📍 {{ v.location }}</div>
          </li>
        </ul>
      </div>
    </div>

    <div v-if="error" class="alert alert-error py-2 text-sm">{{ error }}</div>

    <div class="space-y-2">
      <div v-for="c in comments" :key="c.id" class="card bg-base-100 shadow-sm">
        <div class="card-body py-3 px-4">
          <div class="flex items-center gap-2 text-xs text-base-content/60">
            <span class="font-semibold text-base-content">{{ authorLabel(c) }}</span>
            <span>{{ format(new Date(c.created), 'MMM d, yyyy HH:mm') }}</span>
          </div>
          <p class="whitespace-pre-wrap text-sm">{{ c.body }}</p>
          <AttachmentList :record="c" :files="c.attachments" />
        </div>
      </div>
      <p v-if="comments.length === 0" class="text-sm text-base-content/50 px-1">No replies yet.</p>
    </div>

    <div class="card bg-base-100 shadow-sm">
      <div class="card-body py-3 px-4 space-y-2">
        <textarea
          v-model="newComment"
          rows="3"
          class="textarea textarea-bordered w-full"
          placeholder="Add a reply…"
          :disabled="posting"
        ></textarea>
        <FileInput v-model:files="commentFiles" :disabled="posting" />
        <div class="flex justify-end">
          <button class="btn btn-primary btn-sm" :disabled="posting || !newComment.trim()" @click="postComment">
            <span v-if="posting" class="loading loading-spinner loading-xs"></span>
            Reply
          </button>
        </div>
      </div>
    </div>
  </div>
</template>
