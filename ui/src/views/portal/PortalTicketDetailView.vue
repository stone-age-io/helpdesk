<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { pb } from '@/pb'
import { useAuthStore } from '@/stores/auth'
import { useToastStore } from '@/stores/toast'
import type { Ticket, TicketComment, TicketEvent, Visit } from '@/types'
import TicketBadges from '@/components/TicketBadges.vue'
import CategoryBadge from '@/components/CategoryBadge.vue'
import AttachmentList from '@/components/AttachmentList.vue'
import FileInput from '@/components/FileInput.vue'
import Avatar from '@/components/Avatar.vue'
import TicketProgress from '@/components/TicketProgress.vue'
import { format, formatDistanceToNow } from 'date-fns'

const route = useRoute()
const router = useRouter()
const auth = useAuthStore()
const toast = useToastStore()
const id = route.params.id as string

const ticket = ref<Ticket | null>(null)
const comments = ref<TicketComment[]>([])
const visits = ref<Visit[]>([])
const statusEvents = ref<TicketEvent[]>([])
// Aggregate time logged, only when this customer has opted in — the route
// returns 403 otherwise and we simply show nothing. Never per-entry detail.
const timeMinutes = ref<number | null>(null)
const loading = ref(true)
const error = ref('')

function fmtHours(m: number): string {
  const h = Math.floor(m / 60)
  return h > 0 ? `${h}h ${m % 60}m` : `${m}m`
}

// The status stepper lives in <TicketProgress>; statusEvents feeds it. The
// read rule (migration 1808000000) scopes ticket_events to field='status' on
// the requester's own tickets, and the actor is never requested — nothing
// leaks.
const newComment = ref('')
const commentFiles = ref<File[]>([])
const posting = ref(false)

async function loadTicket() {
  // Expand category for the badge; the ticket_categories read rule (migration
  // 1808000000) lets requesters resolve the label.
  ticket.value = await pb.collection('tickets').getOne<Ticket>(id, { expand: 'category' })
}

async function loadComments() {
  // Rules already exclude internal notes for requesters.
  comments.value = await pb.collection('ticket_comments').getFullList<TicketComment>({
    filter: `ticket = '${id}'`,
    sort: 'created',
    expand: 'author_staff,author_user',
  })
}

async function loadStatusEvents() {
  // No actor expand — the rule would drop it anyway, and we don't want it.
  try {
    statusEvents.value = await pb.collection('ticket_events').getFullList<TicketEvent>({
      filter: `ticket = '${id}' && field = 'status'`,
      sort: 'created',
    })
  } catch {
    // Optional context; the thread still renders without the trail.
  }
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

async function loadTimeTotal() {
  try {
    const res = await pb.send(`/api/helpdesk/tickets/${id}/time-total`, {})
    timeMinutes.value = typeof res?.minutes === 'number' ? res.minutes : null
  } catch {
    // 403 (customer opted out) or any error → show nothing.
    timeMinutes.value = null
  }
}

async function load() {
  loading.value = true
  try {
    await loadTicket()
    await loadComments()
    await loadVisits()
    await loadStatusEvents()
    await loadTimeTotal()
  } catch (err: any) {
    error.value = err?.message || 'Failed to load ticket'
  } finally {
    loading.value = false
  }
}

// Two-stage lifecycle: a `resolved` ticket is in its grace window — a requester
// reply reopens it (surface that intent, and confirm it after). A `closed`
// ticket is final: no reply box, a follow-up is a new ticket (the server rule
// blocks the comment either way).
const isResolved = computed(() => ticket.value?.status === 'resolved')
const isFinalClosed = computed(() => ticket.value?.status === 'closed')

async function postComment() {
  if (!newComment.value.trim()) return
  posting.value = true
  error.value = ''
  const wasResolved = isResolved.value
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
    await Promise.all([loadTicket(), loadComments(), loadStatusEvents()])
    // Confirm the send — and call out the reopen when it happened, since the
    // status flip is otherwise silent.
    if (wasResolved && !isResolved.value) toast.success('Reply sent — ticket reopened')
    else toast.success('Reply sent')
  } catch (err: any) {
    error.value = err?.message || 'Failed to post comment'
    toast.error('Could not send your reply')
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
    loadStatusEvents()
    loadTimeTotal()
  }, 500)
}
let unsubTicket: (() => void) | null = null
let unsubComments: (() => void) | null = null
let unsubVisits: (() => void) | null = null

// Comment identity, portal-side. A staff reply's author record isn't readable
// here (we hide the technician), so it shows as a neutral "Support" — never
// the fall-through "System" it used to render as. The requester's own replies
// read as "You".
const isSupport = (c: TicketComment) => !!c.author_staff
const isMine = (c: TicketComment) => !!c.author_user && c.author_user === auth.record?.id
function authorLabel(c: TicketComment): string {
  if (isSupport(c)) return 'Support'
  if (isMine(c)) return 'You'
  const u = c.expand?.author_user
  if (u) return u.name || u.email
  if (c.author_user) return 'Requester'
  return 'System'
}
function authorRecord(c: TicketComment): Record<string, any> | null {
  if (isSupport(c)) return null
  if (isMine(c)) return auth.record
  return c.expand?.author_user || null
}

// One chronological story, requester-safe: comments (as cards) interleaved with
// status milestones and site-visit milestones (as slim inline rows). The
// progress stepper stays as the at-a-glance summary; this is the detail. What
// feeds it is exactly what the rules already allow a requester to read — status
// events only (never priority/assignee/category/…), non-canceled visits with no
// technician, non-internal comments. Nothing new is fetched or exposed.
type TimelineItem =
  | { kind: 'comment'; key: string; at: string; comment: TicketComment }
  | { kind: 'status'; key: string; at: string; event: TicketEvent }
  | { kind: 'visit'; key: string; at: string; visit: Visit }

// A visit is one record, not a per-transition log, so it appears as a single
// milestone at its most telling moment: completion time when completed, its
// (possibly future) slot when scheduled, else when requested — so an upcoming
// visit sorts to the tail as "coming up".
function visitAt(v: Visit): string {
  if (v.status === 'completed') return v.completed_at || v.scheduled_at || v.created
  if (v.scheduled_at) return v.scheduled_at
  return v.created
}

const timeline = computed<TimelineItem[]>(() => {
  const items: TimelineItem[] = [
    ...comments.value.map((c) => ({ kind: 'comment' as const, key: 'c' + c.id, at: c.created, comment: c })),
    ...statusEvents.value.map((e) => ({ kind: 'status' as const, key: 's' + e.id, at: e.created, event: e })),
    ...visits.value.map((v) => ({ kind: 'visit' as const, key: 'v' + v.id, at: visitAt(v), visit: v })),
  ]
  return items.sort((a, b) => a.at.localeCompare(b.at))
})

const STATUS_TEXT: Record<string, string> = {
  open: 'Open',
  in_progress: 'In progress',
  waiting: 'Waiting',
  resolved: 'Resolved',
  closed: 'Closed',
}
const statusText = (s?: string) => STATUS_TEXT[s || ''] || (s || '').replace(/_/g, ' ')
const visitGlyph = (status: string) =>
  status === 'completed' ? '✅' : status === 'scheduled' ? '🗓️' : '📋'
// Requester-facing visit line: current state + its relevant time, never a tech.
function visitLine(v: Visit): string {
  if (v.status === 'completed') {
    const at = v.completed_at || v.scheduled_at
    return at ? `On-site visit completed — ${format(new Date(at), 'MMM d, HH:mm')}` : 'On-site visit completed'
  }
  if (v.status === 'scheduled') {
    return v.scheduled_at
      ? `Site visit scheduled — ${format(new Date(v.scheduled_at), 'EEE, MMM d HH:mm')}`
      : 'Site visit scheduled'
  }
  return 'On-site visit requested — scheduling in progress'
}

onMounted(async () => {
  await load()
  try {
    unsubTicket = await pb.collection('tickets').subscribe(id, scheduleReload)
    unsubComments = await pb.collection('ticket_comments').subscribe('*', scheduleReload)
    // So a scheduled/completed visit weaves into the thread without a refresh.
    unsubVisits = await pb.collection('visits').subscribe('*', scheduleReload)
  } catch {
    // Realtime is progressive enhancement; the view works without it.
  }
})

onUnmounted(() => {
  clearTimeout(reloadTimer)
  unsubTicket?.()
  unsubComments?.()
  unsubVisits?.()
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

    <div class="flex flex-col xl:flex-row gap-4 items-start">
      <!-- Main: subject + conversation + composer -->
      <div class="flex-1 w-full min-w-0 space-y-4">
        <div class="card bg-base-100 shadow-sm">
          <div class="card-body">
            <h1 class="text-xl font-bold">#{{ ticket.number }} — {{ ticket.title }}</h1>
            <p v-if="ticket.assignee" class="text-xs text-base-content/60 flex items-center gap-1">
              🧑‍🔧 An agent is working on this ticket
            </p>
            <p v-if="ticket.body" class="whitespace-pre-wrap text-sm mt-2">{{ ticket.body }}</p>
            <AttachmentList :record="ticket" :files="ticket.attachments" />
          </div>
        </div>

        <!-- Mobile: status + progress collapsed directly under the header, so
             the context isn't buried below the conversation. Desktop keeps the
             detail cards in the rail. -->
        <details class="group xl:hidden card bg-base-100 shadow-sm">
          <summary class="list-none cursor-pointer select-none flex items-center gap-2 py-3 px-4 [&::-webkit-details-marker]:hidden">
            <span class="font-semibold text-sm">Status</span>
            <TicketBadges :status="ticket.status" :priority="ticket.priority" />
            <span class="ml-auto text-base-content/50 transition-transform group-open:rotate-90">▸</span>
          </summary>
          <div class="px-4 pb-4 space-y-3">
            <div class="space-y-2 text-sm">
              <div class="flex items-center justify-between gap-2">
                <span class="text-base-content/60">Category</span>
                <CategoryBadge
                  v-if="ticket.expand?.category?.name"
                  :name="ticket.expand?.category?.name"
                  :color="ticket.expand?.category?.color"
                />
                <span v-else class="text-base-content/40">—</span>
              </div>
              <div class="flex items-center justify-between gap-2">
                <span class="text-base-content/60">Opened</span>
                <span>{{ format(new Date(ticket.created), 'MMM d, yyyy') }}</span>
              </div>
              <div v-if="ticket.updated" class="flex items-center justify-between gap-2">
                <span class="text-base-content/60">Updated</span>
                <span>{{ formatDistanceToNow(new Date(ticket.updated), { addSuffix: true }) }}</span>
              </div>
              <div v-if="timeMinutes !== null" class="flex items-center justify-between gap-2">
                <span class="text-base-content/60">Time logged</span>
                <span>{{ fmtHours(timeMinutes) }}</span>
              </div>
            </div>
            <div class="divider my-0"></div>
            <TicketProgress :ticket="ticket" :status-events="statusEvents" />
          </div>
        </details>

        <div v-if="error" class="alert alert-error py-2 text-sm">{{ error }}</div>

        <!-- Unified thread: replies as cards, status + visit milestones as slim
             inline rows. The stepper in the rail is the summary; this is the
             chronological detail. -->
        <div class="space-y-2">
          <template v-for="item in timeline" :key="item.key">
            <!-- Reply -->
            <div v-if="item.kind === 'comment'" class="card bg-base-100 shadow-sm">
              <div class="card-body py-3 px-4">
                <div class="flex items-start gap-2.5">
                  <!-- Staff replies get a neutral support glyph (no technician
                       avatar); requesters get their own avatar. -->
                  <div v-if="isSupport(item.comment)" class="avatar placeholder shrink-0">
                    <div class="w-8 rounded-full bg-primary/15 text-primary"><span class="text-sm">🛟</span></div>
                  </div>
                  <Avatar v-else :record="authorRecord(item.comment)" :name="authorLabel(item.comment)" size="sm" />
                  <div class="flex-1 min-w-0">
                    <div class="flex items-center gap-2 text-xs text-base-content/60 flex-wrap">
                      <span class="font-semibold text-base-content">{{ authorLabel(item.comment) }}</span>
                      <span v-if="isSupport(item.comment)" class="badge-soft badge-soft-neutral">support</span>
                      <span>{{ format(new Date(item.comment.created), 'MMM d, yyyy HH:mm') }}</span>
                    </div>
                    <p class="whitespace-pre-wrap text-sm mt-0.5">{{ item.comment.body }}</p>
                    <AttachmentList :record="item.comment" :files="item.comment.attachments" />
                  </div>
                </div>
              </div>
            </div>

            <!-- Status milestone -->
            <div v-else-if="item.kind === 'status'" class="flex items-center gap-2 px-2 text-xs text-base-content/60 leading-snug">
              <span class="w-6 text-center text-sm shrink-0" aria-hidden="true">🔄</span>
              <span class="flex-1">
                Status changed to
                <span class="font-semibold text-base-content">{{ statusText(item.event.new_value) }}</span>
                <span class="text-base-content/40"> · {{ format(new Date(item.event.created), 'MMM d, HH:mm') }}</span>
              </span>
            </div>

            <!-- Visit milestone (no technician, portal-side) -->
            <div v-else class="flex items-center gap-2 px-2 text-xs text-base-content/60 leading-snug">
              <span class="w-6 text-center text-sm shrink-0" aria-hidden="true">{{ visitGlyph(item.visit.status) }}</span>
              <span class="flex-1">
                <span class="font-medium text-base-content/80">{{ visitLine(item.visit) }}</span>
                <span v-if="item.visit.location" class="text-base-content/50"> · 📍 {{ item.visit.location }}</span>
              </span>
            </div>
          </template>
          <p v-if="timeline.length === 0" class="text-sm text-base-content/50 px-1">No activity yet.</p>
        </div>

        <!-- Composer. Sticky at the viewport bottom on mobile so replying is
             always in reach; static on desktop. -->
        <div class="card bg-base-100 shadow-sm sticky bottom-0 z-20 shadow-lg xl:static xl:z-auto xl:shadow-sm">
          <!-- Closed is final: no reply box; a follow-up is a new ticket. -->
          <div v-if="isFinalClosed" class="card-body py-3 px-4">
            <p class="text-sm text-base-content/70 flex items-center gap-1.5">
              <span aria-hidden="true">🔒</span>
              This ticket is closed. To follow up, please
              <RouterLink :to="{ name: 'portal-ticket-new' }" class="link link-primary">open a new ticket</RouterLink>.
            </p>
          </div>
          <div v-else class="card-body py-3 px-4 space-y-2">
            <p v-if="ticket.awaiting_requester" class="text-xs text-info font-medium flex items-center gap-1.5">
              <span aria-hidden="true">⏳</span>
              Support is waiting on your reply.
            </p>
            <p v-else-if="isResolved" class="text-xs text-warning flex items-center gap-1.5">
              <span aria-hidden="true">↩️</span>
              This ticket is resolved. Replying will reopen it.
            </p>
            <textarea
              v-model="newComment"
              rows="3"
              class="textarea textarea-bordered w-full"
              :placeholder="isResolved ? 'Reply to reopen this ticket…' : 'Add a reply…'"
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

      <!-- Rail: read-only details (desktop; mobile shows the collapsible above) -->
      <div class="w-full xl:w-80 space-y-4 xl:sticky xl:top-4 self-stretch xl:self-start">
        <div class="card bg-base-100 shadow-sm hidden xl:block">
          <div class="card-body py-4 px-4 gap-2 text-sm">
            <div class="flex items-center justify-between gap-2">
              <span class="text-base-content/60">Status</span>
              <TicketBadges :status="ticket.status" />
            </div>
            <div class="flex items-center justify-between gap-2">
              <span class="text-base-content/60">Priority</span>
              <TicketBadges :priority="ticket.priority" />
            </div>
            <div class="flex items-center justify-between gap-2">
              <span class="text-base-content/60">Category</span>
              <CategoryBadge
                v-if="ticket.expand?.category?.name"
                :name="ticket.expand?.category?.name"
                :color="ticket.expand?.category?.color"
              />
              <span v-else class="text-base-content/40">—</span>
            </div>
            <div class="flex items-center justify-between gap-2">
              <span class="text-base-content/60">Opened</span>
              <span>{{ format(new Date(ticket.created), 'MMM d, yyyy') }}</span>
            </div>
            <div v-if="ticket.updated" class="flex items-center justify-between gap-2">
              <span class="text-base-content/60">Updated</span>
              <span>{{ formatDistanceToNow(new Date(ticket.updated), { addSuffix: true }) }}</span>
            </div>
            <div v-if="timeMinutes !== null" class="flex items-center justify-between gap-2">
              <span class="text-base-content/60">Time logged</span>
              <span>{{ fmtHours(timeMinutes) }}</span>
            </div>
          </div>
        </div>

        <!-- Progress stepper (desktop; mobile shows it in the collapsible above).
             Site visits are no longer a separate card — they're woven into the
             thread as milestones. -->
        <div class="card bg-base-100 shadow-sm hidden xl:block">
          <div class="card-body py-4 px-4">
            <TicketProgress :ticket="ticket" :status-events="statusEvents" />
          </div>
        </div>
      </div>
    </div>
  </div>
</template>
