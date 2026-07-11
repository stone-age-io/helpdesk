<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { pb } from '@/pb'
import { useAuthStore } from '@/stores/auth'
import type { Staff, Ticket, TicketComment } from '@/types'
import { TICKET_PRIORITIES, TICKET_STATUSES } from '@/types'
import TicketBadges from '@/components/TicketBadges.vue'
import TimeEntriesCard from '@/components/TimeEntriesCard.vue'
import VisitsCard from '@/components/VisitsCard.vue'
import SearchSelect from '@/components/SearchSelect.vue'
import { format } from 'date-fns'

const route = useRoute()
const router = useRouter()
const auth = useAuthStore()
const id = route.params.id as string

const ticket = ref<Ticket | null>(null)
const comments = ref<TicketComment[]>([])
const staff = ref<Staff[]>([])
const loading = ref(true)
const error = ref('')

const newComment = ref('')
const internalNote = ref(false)
const posting = ref(false)

const requesterName = computed(() => {
  const r = ticket.value?.expand?.requester
  return r ? r.name || r.email : null
})

const staffOptions = computed(() => staff.value.map((s) => ({ id: s.id, label: s.name, sublabel: s.email })))

async function load() {
  loading.value = true
  error.value = ''
  try {
    ticket.value = await pb.collection('tickets').getOne<Ticket>(id, {
      expand: 'customer,assignee,requester',
    })
    comments.value = await pb.collection('ticket_comments').getFullList<TicketComment>({
      filter: `ticket = '${id}'`,
      sort: 'created',
      expand: 'author_staff,author_user',
    })
    staff.value = await pb.collection('staff').getFullList<Staff>({ sort: 'name', filter: 'active = true' })
  } catch (err: any) {
    error.value = err?.message || 'Failed to load ticket'
  } finally {
    loading.value = false
  }
}

async function updateField(field: 'status' | 'priority' | 'assignee', value: string) {
  if (!ticket.value) return
  try {
    ticket.value = await pb.collection('tickets').update<Ticket>(id, { [field]: value }, {
      expand: 'customer,assignee,requester',
    })
  } catch (err: any) {
    error.value = err?.message || `Failed to update ${field}`
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
    })
    newComment.value = ''
    internalNote.value = false
    comments.value = await pb.collection('ticket_comments').getFullList<TicketComment>({
      filter: `ticket = '${id}'`,
      sort: 'created',
      expand: 'author_staff,author_user',
    })
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

onMounted(load)
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
            <div class="flex items-center gap-2 flex-wrap">
              <h1 class="text-xl font-bold">#{{ ticket.number }} — {{ ticket.title }}</h1>
              <TicketBadges :status="ticket.status" :priority="ticket.priority" />
            </div>
            <p v-if="ticket.body" class="whitespace-pre-wrap text-sm mt-2">{{ ticket.body }}</p>
            <p v-if="ticket.origin_subject" class="text-xs font-mono text-base-content/50 mt-2">
              via {{ ticket.origin_subject }}
            </p>
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
            <div>
              <div class="text-xs text-base-content/60">Customer</div>
              <router-link
                v-if="ticket.expand?.customer"
                :to="`/staff/customers/${ticket.customer}`"
                class="link link-hover font-medium"
              >{{ ticket.expand.customer.name }}</router-link>
            </div>
            <div v-if="requesterName">
              <div class="text-xs text-base-content/60">Requester</div>
              <div class="text-sm">{{ requesterName }}</div>
            </div>
            <div>
              <div class="text-xs text-base-content/60">Source</div>
              <div class="text-sm">{{ ticket.source }}</div>
            </div>
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
      </div>
    </div>
  </div>
</template>
