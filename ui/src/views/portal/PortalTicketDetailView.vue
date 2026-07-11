<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { pb } from '@/pb'
import { useAuthStore } from '@/stores/auth'
import type { Ticket, TicketComment, TicketEvent, Visit } from '@/types'
import TicketBadges from '@/components/TicketBadges.vue'
import CategoryBadge from '@/components/CategoryBadge.vue'
import AttachmentList from '@/components/AttachmentList.vue'
import FileInput from '@/components/FileInput.vue'
import Avatar from '@/components/Avatar.vue'
import { format, formatDistanceToNow } from 'date-fns'

const route = useRoute()
const router = useRouter()
const auth = useAuthStore()
const id = route.params.id as string

const ticket = ref<Ticket | null>(null)
const comments = ref<TicketComment[]>([])
const visits = ref<Visit[]>([])
const statusEvents = ref<TicketEvent[]>([])
const loading = ref(true)
const error = ref('')

// Status stepper: collapse the raw event churn (a status can bounce back and
// forth) into the canonical lifecycle a requester cares about. "Current" is
// derived from the ticket's status now — not the furthest stage ever reached —
// so a reopened ticket reads sensibly instead of showing a regression.
const STEPS = ['Open', 'In progress', 'Resolved'] as const
function stageIndex(status?: string): number {
  if (status === 'resolved' || status === 'closed') return 2
  if (status === 'in_progress' || status === 'waiting') return 1
  return 0
}
const currentStage = computed(() => stageIndex(ticket.value?.status))
// Name the terminal stage by the actual status.
const stepLabel = (i: number) => (i === 2 && ticket.value?.status === 'closed' ? 'Closed' : STEPS[i])
// First time the ticket entered a stage (stage 0 = creation). The status read
// rule (migration 1808000000) scopes ticket_events to field='status' on the
// requester's own tickets; the actor is never requested, so nothing leaks.
function stepReachedAt(i: number): string {
  if (i === 0) return ticket.value?.created || ''
  return statusEvents.value.find((e) => stageIndex(e.new_value) === i)?.created || ''
}

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

async function load() {
  loading.value = true
  try {
    await loadTicket()
    await loadComments()
    await loadVisits()
    await loadStatusEvents()
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
    await Promise.all([loadTicket(), loadComments(), loadStatusEvents()])
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
    loadStatusEvents()
  }, 500)
}
let unsubTicket: (() => void) | null = null
let unsubComments: (() => void) | null = null

const visitBadge: Record<string, string> = {
  requested: 'badge-warning',
  scheduled: 'badge-info',
  completed: 'badge-success',
}

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

        <div v-if="error" class="alert alert-error py-2 text-sm">{{ error }}</div>

        <!-- Conversation -->
        <div class="space-y-2">
          <div v-for="c in comments" :key="c.id" class="card bg-base-100 shadow-sm">
            <div class="card-body py-3 px-4">
              <div class="flex items-start gap-2.5">
                <!-- Staff replies get a neutral support glyph (no technician
                     avatar); requesters get their own avatar. -->
                <div v-if="isSupport(c)" class="avatar placeholder shrink-0">
                  <div class="w-8 rounded-full bg-primary/15 text-primary"><span class="text-sm">🛟</span></div>
                </div>
                <Avatar v-else :record="authorRecord(c)" :name="authorLabel(c)" size="sm" />
                <div class="flex-1 min-w-0">
                  <div class="flex items-center gap-2 text-xs text-base-content/60 flex-wrap">
                    <span class="font-semibold text-base-content">{{ authorLabel(c) }}</span>
                    <span v-if="isSupport(c)" class="badge badge-ghost badge-xs">support</span>
                    <span>{{ format(new Date(c.created), 'MMM d, yyyy HH:mm') }}</span>
                  </div>
                  <p class="whitespace-pre-wrap text-sm mt-0.5">{{ c.body }}</p>
                  <AttachmentList :record="c" :files="c.attachments" />
                </div>
              </div>
            </div>
          </div>
          <p v-if="comments.length === 0" class="text-sm text-base-content/50 px-1">No replies yet.</p>
        </div>

        <!-- Composer -->
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

      <!-- Rail: read-only details -->
      <div class="w-full xl:w-80 space-y-4 xl:sticky xl:top-4 self-stretch xl:self-start">
        <div class="card bg-base-100 shadow-sm">
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
          </div>
        </div>

        <!-- Progress stepper -->
        <div class="card bg-base-100 shadow-sm">
          <div class="card-body py-4 px-4">
            <h2 class="font-semibold text-sm mb-2">Progress</h2>
            <ol>
              <li v-for="(_, i) in STEPS" :key="i" class="flex gap-3 relative pb-4 last:pb-0">
                <!-- connector -->
                <span
                  v-if="i < STEPS.length - 1"
                  class="absolute left-[5px] top-3.5 -bottom-0.5 w-px"
                  :class="i < currentStage ? 'bg-primary' : 'bg-base-300'"
                ></span>
                <!-- dot -->
                <span
                  class="relative z-10 mt-1 w-2.5 h-2.5 rounded-full shrink-0"
                  :class="
                    i < currentStage
                      ? 'bg-primary'
                      : i === currentStage
                        ? 'bg-primary ring-4 ring-primary/20'
                        : 'bg-base-300'
                  "
                ></span>
                <div class="flex-1 -mt-0.5">
                  <div class="text-sm" :class="i <= currentStage ? 'font-medium' : 'text-base-content/40'">
                    {{ stepLabel(i) }}
                  </div>
                  <div v-if="i <= currentStage && stepReachedAt(i)" class="text-xs text-base-content/50">
                    {{ format(new Date(stepReachedAt(i)), 'MMM d, HH:mm') }}
                  </div>
                </div>
              </li>
            </ol>
          </div>
        </div>

        <!-- Site visits -->
        <div v-if="visits.length > 0" class="card bg-base-100 shadow-sm">
          <div class="card-body py-4 px-4 space-y-2">
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
      </div>
    </div>
  </div>
</template>
