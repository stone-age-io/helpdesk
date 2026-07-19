<script setup lang="ts">
import { computed, nextTick, onMounted, onUnmounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { pb } from '@/pb'
import { useAuthStore } from '@/stores/auth'
import type { Customer, Location, Project, Requester, Staff, Ticket, TicketCategory, TicketComment, TicketEvent, TimeEntry, Visit } from '@/types'
import TicketBadges from '@/components/TicketBadges.vue'
import CategoryBadge from '@/components/CategoryBadge.vue'
import WorkCard from '@/components/WorkCard.vue'
import TicketPropertiesFields from '@/components/TicketPropertiesFields.vue'
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
const events = ref<TicketEvent[]>([])
// Visits + time are managed in the WorkCard rail; the timeline loads them
// independently, read-only, purely to interleave them into the activity stream.
const visits = ref<Visit[]>([])
const timeEntries = ref<TimeEntry[]>([])
const staff = ref<Staff[]>([])
const customers = ref<Customer[]>([])
const requesters = ref<Requester[]>([])
const categories = ref<TicketCategory[]>([])
const locations = ref<Location[]>([])
const projects = ref<Project[]>([])
const loading = ref(true)
const error = ref('')

const newComment = ref('')
const internalNote = ref(false)
const commentFiles = ref<File[]>([])
const posting = ref(false)
// Mobile only: the composer collapses to a slim sticky bar so a long thread
// stays readable, expanding to the full reply box on tap. Desktop renders the
// full composer statically and ignores this (see the isDesktop guard below).
const composerOpen = ref(false)
const composerTextarea = ref<HTMLTextAreaElement | null>(null)
function openComposer() {
  composerOpen.value = true
  nextTick(() => composerTextarea.value?.focus())
}
// When off, field edits are saved without emailing the requester — for
// triage cleanup, a mis-set status, or an internal reassignment.
const notify = ref(true)

// Reactive Tailwind `xl` breakpoint so the properties/time/visits panel
// renders exactly once — in the desktop rail, or grouped under the header on
// mobile — rather than mounting in both spots and fetching twice.
const isDesktop = ref(window.matchMedia('(min-width: 1280px)').matches)
let mq: MediaQueryList | undefined
const onBreakpoint = (e: MediaQueryListEvent) => (isDesktop.value = e.matches)

// Inline title/body editing.
const editingHeader = ref(false)
const editTitle = ref('')
const editBody = ref('')

const staffOptions = computed(() => staff.value.map((s) => ({ id: s.id, label: s.name, sublabel: s.email })))
const customerOptions = computed(() => customers.value.map((c) => ({ id: c.id, label: c.name })))
const categoryOptions = computed(() => categories.value.map((c) => ({ id: c.id, label: c.name })))
const requesterOptions = computed(() =>
  requesters.value.map((r) => ({ id: r.id, label: r.name || r.email, sublabel: r.name ? r.email : undefined })),
)
const locationOptions = computed(() =>
  locations.value.map((l) => ({ id: l.id, label: l.name, sublabel: l.code || l.address || undefined })),
)
const projectOptions = computed(() =>
  projects.value.map((p) => ({ id: p.id, label: `#${p.number} ${p.title}`, sublabel: p.status })),
)

// One chronological stream: comments (full cards) interleaved with the audit
// events (compact rows), oldest first, composer pinned at the bottom. This is
// the reorg — the standalone activity card is gone; its events now live here.
type TimelineItem =
  | { kind: 'comment'; key: string; created: string; comment: TicketComment }
  | { kind: 'event'; key: string; created: string; event: TicketEvent }
  | { kind: 'visit'; key: string; created: string; visit: Visit }
  | { kind: 'time'; key: string; created: string; time: TimeEntry }
// A visit is one record, not a per-transition log, so it appears as a single
// milestone placed at its most telling moment: when completed, its completion
// time; when scheduled, its (possibly future) slot; otherwise when requested.
// A scheduled visit therefore sorts to the tail as "upcoming".
function visitTimelineAt(v: Visit): string {
  if (v.status === 'completed') return v.completed_at || v.scheduled_at || v.created
  if (v.scheduled_at) return v.scheduled_at
  return v.created
}
const timeline = computed<TimelineItem[]>(() => {
  const items: TimelineItem[] = [
    ...comments.value.map((c) => ({ kind: 'comment' as const, key: 'c' + c.id, created: c.created, comment: c })),
    ...events.value.map((e) => ({ kind: 'event' as const, key: 'e' + e.id, created: e.created, event: e })),
    ...visits.value.map((v) => ({ kind: 'visit' as const, key: 'v' + v.id, created: visitTimelineAt(v), visit: v })),
    ...timeEntries.value.map((te) => ({ kind: 'time' as const, key: 't' + te.id, created: te.created, time: te })),
  ]
  return items.sort((a, b) => a.created.localeCompare(b.created))
})

async function loadTicket() {
  ticket.value = await pb.collection('tickets').getOne<Ticket>(id, {
    expand: 'customer,assignee,requester,category,location,project',
  })
}

async function loadRequesters(customerId: string) {
  requesters.value = customerId
    ? await pb.collection('users').getFullList<Requester>({ filter: `customer = '${customerId}'`, sort: 'name' })
    : []
}

async function loadLocations(customerId: string) {
  locations.value = customerId
    ? await pb.collection('locations').getFullList<Location>({ filter: `customer = '${customerId}'`, sort: 'name' })
    : []
}

async function loadProjects(customerId: string) {
  projects.value = customerId
    ? await pb.collection('projects').getFullList<Project>({ filter: `customer = '${customerId}'`, sort: '-created' })
    : []
}

async function loadComments() {
  comments.value = await pb.collection('ticket_comments').getFullList<TicketComment>({
    filter: `ticket = '${id}'`,
    sort: 'created',
    expand: 'author_staff,author_user',
  })
}

async function loadEvents() {
  try {
    events.value = await pb.collection('ticket_events').getFullList<TicketEvent>({
      filter: `ticket = '${id}'`,
      sort: 'created',
      expand: 'actor_staff,actor_user',
    })
  } catch {
    // Timeline still works from comments alone if the audit read fails.
  }
}

async function loadVisits() {
  // Expand the technician so the timeline can name who went (staff-side only;
  // the portal deliberately hides it). Read-only here — the WorkCard owns edits.
  try {
    visits.value = await pb.collection('visits').getFullList<Visit>({
      filter: `ticket = '${id}'`,
      sort: 'created',
      expand: 'assignee',
    })
  } catch {
    // Optional context; the thread still renders without it.
  }
}

async function loadTimeEntries() {
  // Expand staff so a logged-time row names the agent even if they're inactive
  // (the active-only staff list wouldn't resolve them).
  try {
    timeEntries.value = await pb.collection('time_entries').getFullList<TimeEntry>({
      filter: `ticket = '${id}'`,
      sort: 'created',
      expand: 'staff',
    })
  } catch {
    // Optional context.
  }
}

async function load() {
  loading.value = true
  error.value = ''
  try {
    await Promise.all([loadTicket(), loadComments(), loadEvents(), loadVisits(), loadTimeEntries()])
    staff.value = await pb.collection('staff').getFullList<Staff>({ sort: 'name', filter: 'active = true' })
    customers.value = await pb.collection('customers').getFullList<Customer>({ sort: 'name' })
    categories.value = await pb.collection('ticket_categories').getFullList<TicketCategory>({ sort: 'sort_order,name', filter: 'active = true' })
    await loadRequesters(ticket.value?.customer || '')
    await loadLocations(ticket.value?.customer || '')
    await loadProjects(ticket.value?.customer || '')
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
        expand: 'customer,assignee,requester,category,location,project',
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
async function patchPlain(fields: Record<string, string | number | null>) {
  if (!ticket.value) return
  try {
    ticket.value = await pb.collection('tickets').update<Ticket>(id, fields, {
      expand: 'customer,assignee,requester,category,location,project',
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
  await patchPlain({ customer: value, requester: '', location: '', project: '' })
  await loadRequesters(value)
  await loadLocations(value)
  await loadProjects(value)
}

// Inline-create a location for this ticket's customer from the picker, then
// select it. Any staff may create; admins curate them in the roster.
async function createLocation(label: string) {
  const customerId = ticket.value?.customer
  if (!customerId || !label.trim()) return
  try {
    const rec = await pb.collection('locations').create({ customer: customerId, name: label.trim() })
    await loadLocations(customerId)
    await patchPlain({ location: rec.id })
  } catch (err: any) {
    error.value = err?.message || 'Failed to create location'
  }
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
    // Reclaim the reading area on mobile once the reply is in.
    if (!isDesktop.value) composerOpen.value = false
    await loadComments()
  } catch (err: any) {
    error.value = err?.message || 'Failed to post comment'
  } finally {
    posting.value = false
  }
}

function authorRecord(c: TicketComment): Record<string, any> | null {
  return c.expand?.author_staff || c.expand?.author_user || null
}
function authorLabel(c: TicketComment): string {
  const s = c.expand?.author_staff
  if (s) return s.name || s.email
  const u = c.expand?.author_user
  if (u) return u.name || u.email
  return 'System'
}

function actorRecord(e: TicketEvent): Record<string, any> | null {
  return e.expand?.actor_staff || e.expand?.actor_user || null
}
function actorName(e: TicketEvent): string {
  return e.expand?.actor_staff?.name || e.expand?.actor_user?.name || e.expand?.actor_user?.email || 'System'
}
const humanize = (v?: string) => (v || '').replace(/_/g, ' ')

// Timeline visit + time rows (staff-side).
const staffById = computed(() => new Map(staff.value.map((s) => [s.id, s])))
function timeStaffName(te: TimeEntry): string {
  return te.expand?.staff?.name || staffById.value.get(te.staff)?.name || 'Staff'
}
function visitTechName(v: Visit): string {
  if (!v.assignee) return ''
  return v.expand?.assignee?.name || staffById.value.get(v.assignee)?.name || ''
}
const visitGlyph = (status: string) =>
  status === 'completed' ? '✅' : status === 'canceled' ? '✖️' : status === 'scheduled' ? '🗓️' : '📋'
function fmtMinutes(m: number): string {
  const h = Math.floor(m / 60)
  const mm = m % 60
  if (h > 0) return mm > 0 ? `${h}h ${mm}m` : `${h}h`
  return `${mm}m`
}
function fmtDate(v?: string): string {
  if (!v) return ''
  const d = new Date(v)
  return isNaN(d.getTime()) ? '' : format(d, 'MMM d, HH:mm')
}
function fmtDay(v?: string): string {
  if (!v) return ''
  const d = new Date(v)
  return isNaN(d.getTime()) ? '' : format(d, 'MMM d')
}

// Live updates: another agent's reply, a status change, or a requester
// comment lands without a manual refresh. Debounced to collapse bursts.
let reloadTimer: ReturnType<typeof setTimeout> | undefined
function scheduleReload() {
  clearTimeout(reloadTimer)
  reloadTimer = setTimeout(() => {
    loadTicket().catch(() => {})
    loadComments().catch(() => {})
    loadEvents().catch(() => {})
    loadVisits().catch(() => {})
    loadTimeEntries().catch(() => {})
  }, 500)
}
let unsubTicket: (() => void) | null = null
let unsubComments: (() => void) | null = null
let unsubEvents: (() => void) | null = null
let unsubVisits: (() => void) | null = null
let unsubTime: (() => void) | null = null

onMounted(async () => {
  mq = window.matchMedia('(min-width: 1280px)')
  mq.addEventListener('change', onBreakpoint)
  await load()
  try {
    unsubTicket = await pb.collection('tickets').subscribe(id, scheduleReload)
    unsubComments = await pb.collection('ticket_comments').subscribe('*', scheduleReload)
    unsubEvents = await pb.collection('ticket_events').subscribe('*', scheduleReload)
    unsubVisits = await pb.collection('visits').subscribe('*', scheduleReload)
    unsubTime = await pb.collection('time_entries').subscribe('*', scheduleReload)
  } catch {
    // Realtime is progressive enhancement; the view works without it.
  }
})

onUnmounted(() => {
  clearTimeout(reloadTimer)
  mq?.removeEventListener('change', onBreakpoint)
  unsubTicket?.()
  unsubComments?.()
  unsubEvents?.()
  unsubVisits?.()
  unsubTime?.()
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
      <!-- Main column: header + unified timeline + composer -->
      <div class="flex-1 space-y-4 w-full min-w-0">
        <div class="card bg-base-100 shadow-sm">
          <div class="card-body">
            <template v-if="!editingHeader">
              <div class="flex items-start justify-between gap-2">
                <h1 class="text-xl font-bold min-w-0">#{{ ticket.number }} — {{ ticket.title }}</h1>
                <button class="btn btn-ghost btn-xs shrink-0" @click="startEditHeader">Edit</button>
              </div>
              <div class="flex flex-wrap items-center gap-2 mt-2">
                <TicketBadges :status="ticket.status" :priority="ticket.priority" />
                <CategoryBadge :name="ticket.expand?.category?.name" :color="ticket.expand?.category?.color" />
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

        <!-- Work: field visits and labor unified in one card, organized by
             visit, directly under the header for both breakpoints — the
             operational work no longer lives in the rail. Rendered once here. -->
        <WorkCard :ticket-id="id" :staff="staff" :estimated-minutes="ticket?.estimated_minutes" />

        <!-- Mobile: properties collapse behind a summary directly under the
             header. Rendered here on mobile OR in the desktop rail — never
             both. Time + visits are in the Work section above for both. -->
        <div v-if="!isDesktop">
          <details class="group card bg-base-100 shadow-sm">
            <summary class="list-none cursor-pointer select-none flex items-center gap-2 py-3 px-4 [&::-webkit-details-marker]:hidden">
              <span class="font-semibold text-sm">Properties</span>
              <TicketBadges :status="ticket.status" :priority="ticket.priority" />
              <span class="ml-auto text-base-content/50 transition-transform group-open:rotate-90">▸</span>
            </summary>
            <div class="px-4 pb-4 space-y-3">
              <TicketPropertiesFields
                v-model:notify="notify"
                :ticket="ticket"
                :staff-options="staffOptions"
                :customer-options="customerOptions"
                :category-options="categoryOptions"
                :requester-options="requesterOptions"
                :location-options="locationOptions"
                :project-options="projectOptions"
                @update-field="updateField"
                @patch="patchPlain"
                @change-customer="changeCustomer"
                @create-location="createLocation"
              />
            </div>
          </details>
        </div>

        <!-- Unified timeline: comments as cards, audit events as inline rows -->
        <div class="space-y-2">
          <template v-for="item in timeline" :key="item.key">
            <!-- Comment -->
            <div
              v-if="item.kind === 'comment'"
              class="card shadow-sm"
              :class="item.comment.internal ? 'bg-warning/10 border border-warning/30' : 'bg-base-100'"
            >
              <div class="card-body py-3 px-4">
                <div class="flex items-center gap-2 text-xs text-base-content/60">
                  <Avatar :record="authorRecord(item.comment)" :name="authorLabel(item.comment)" size="xs" />
                  <span class="font-semibold text-base-content">{{ authorLabel(item.comment) }}</span>
                  <span v-if="item.comment.internal" class="badge-soft badge-soft-warning">internal</span>
                  <span>{{ format(new Date(item.comment.created), 'MMM d, yyyy HH:mm') }}</span>
                </div>
                <p class="whitespace-pre-wrap text-sm">{{ item.comment.body }}</p>
                <AttachmentList :record="item.comment" :files="item.comment.attachments" />
              </div>
            </div>

            <!-- Audit event -->
            <div
              v-else-if="item.kind === 'event'"
              class="flex items-center gap-2 px-2 text-xs text-base-content/60 leading-snug"
            >
              <Avatar :record="actorRecord(item.event)" :name="actorName(item.event)" size="xs" />
              <span class="flex-1">
                <span class="font-semibold text-base-content">{{ actorName(item.event) }}</span>
                changed {{ item.event.field }}
                <span class="text-base-content/50">{{ humanize(item.event.old_value) || '—' }}</span>
                →
                <span class="font-medium text-base-content/80">{{ humanize(item.event.new_value) || '—' }}</span>
                <span class="text-base-content/40"> · {{ formatDistanceToNow(new Date(item.event.created), { addSuffix: true }) }}</span>
              </span>
            </div>

            <!-- Visit milestone -->
            <div
              v-else-if="item.kind === 'visit'"
              class="flex items-center gap-2 px-2 text-xs text-base-content/60 leading-snug"
            >
              <span class="w-6 text-center text-sm shrink-0" aria-hidden="true">{{ visitGlyph(item.visit.status) }}</span>
              <span class="flex-1">
                <span class="font-semibold text-base-content">Site visit {{ item.visit.status }}</span>
                <template v-if="fmtDate(visitTimelineAt(item.visit))"> · {{ fmtDate(visitTimelineAt(item.visit)) }}</template>
                <template v-if="visitTechName(item.visit)"> · {{ visitTechName(item.visit) }}</template>
                <span v-if="item.visit.location" class="text-base-content/50"> · 📍 {{ item.visit.location }}</span>
              </span>
            </div>

            <!-- Logged time -->
            <div v-else class="flex items-center gap-2 px-2 text-xs text-base-content/60 leading-snug">
              <span class="w-6 text-center text-sm shrink-0" aria-hidden="true">⏱️</span>
              <span class="flex-1">
                <span class="font-semibold text-base-content">{{ timeStaffName(item.time) }}</span>
                logged <span class="font-medium text-base-content/80">{{ fmtMinutes(item.time.minutes) }}</span>
                <span v-if="item.time.note" class="text-base-content/50"> · {{ item.time.note }}</span>
                <span class="text-base-content/40"> · {{ fmtDay(item.time.work_date) }}</span>
              </span>
            </div>
          </template>
          <p v-if="timeline.length === 0" class="text-sm text-base-content/50 px-1">No activity yet.</p>
        </div>

        <!-- Composer. Sticky at the viewport bottom on mobile so replying is
             always in reach no matter how long the timeline; static on desktop.
             On mobile it collapses to a slim bar (below) to keep the thread
             readable, expanding to the full box on tap. -->
        <div class="sticky bottom-0 z-20 xl:static xl:z-auto">
          <!-- Collapsed mobile bar: one tap to expand. -->
          <button
            v-if="!isDesktop && !composerOpen"
            class="btn btn-block justify-start font-normal text-base-content/50 bg-base-100 border-base-300 shadow-lg"
            @click="openComposer"
          >
            Write a reply…
          </button>

          <!-- Full composer: always on desktop, on demand on mobile. -->
          <div v-else class="card bg-base-100 shadow-lg xl:shadow-sm">
            <div class="card-body py-3 px-4 space-y-2">
              <button
                v-if="!isDesktop"
                class="btn btn-ghost btn-xs self-end -mb-1"
                @click="composerOpen = false"
              >
                Collapse ▾
              </button>
              <textarea
                ref="composerTextarea"
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
      </div>

      <!-- Desktop controls rail: sticky so the workflow fields stay in view
           while the timeline scrolls. On mobile the same panel renders under
           the header (above) instead. -->
      <div v-if="isDesktop" class="w-full xl:w-80 space-y-4 xl:sticky xl:top-4 self-stretch xl:self-start">
        <div class="card bg-base-100 shadow-sm">
          <div class="card-body py-4 px-4 space-y-3">
            <TicketPropertiesFields
              v-model:notify="notify"
              :ticket="ticket"
              :staff-options="staffOptions"
              :customer-options="customerOptions"
              :category-options="categoryOptions"
              :requester-options="requesterOptions"
              :location-options="locationOptions"
              :project-options="projectOptions"
              @update-field="updateField"
              @patch="patchPlain"
              @change-customer="changeCustomer"
              @create-location="createLocation"
            />
          </div>
        </div>
      </div>
    </div>
  </div>
</template>
