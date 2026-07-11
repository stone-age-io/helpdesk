<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { pb } from '@/pb'
import { useAuthStore } from '@/stores/auth'
import type { Ticket, TicketComment } from '@/types'
import TicketBadges from '@/components/TicketBadges.vue'
import { format } from 'date-fns'

const route = useRoute()
const router = useRouter()
const auth = useAuthStore()
const id = route.params.id as string

const ticket = ref<Ticket | null>(null)
const comments = ref<TicketComment[]>([])
const loading = ref(true)
const error = ref('')

const newComment = ref('')
const posting = ref(false)

async function loadComments() {
  // Rules already exclude internal notes for requesters.
  comments.value = await pb.collection('ticket_comments').getFullList<TicketComment>({
    filter: `ticket = '${id}'`,
    sort: 'created',
    expand: 'author_staff,author_user',
  })
}

async function load() {
  loading.value = true
  try {
    ticket.value = await pb.collection('tickets').getOne<Ticket>(id)
    await loadComments()
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
    })
    newComment.value = ''
    await loadComments()
  } catch (err: any) {
    error.value = err?.message || 'Failed to post comment'
  } finally {
    posting.value = false
  }
}

function authorLabel(c: TicketComment): string {
  const s = c.expand?.author_staff
  if (s) return `${s.name || 'Support'} (support)`
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
        <li><a @click="router.push('/portal/tickets')">My Tickets</a></li>
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
