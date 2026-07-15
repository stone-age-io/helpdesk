<script setup lang="ts">
// Staff detail / edit view (admin only — the route carries meta.adminOnly).
// Identity, role, active, and password reset for one staff member, plus their
// open assigned tickets. Two self-guards mirror the old inline list: you can't
// change your own role or deactivate yourself (locking yourself out).
import { computed, onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { pb } from '@/pb'
import { useAuthStore } from '@/stores/auth'
import type { Staff, Ticket } from '@/types'
import TicketBadges from '@/components/TicketBadges.vue'
import Avatar from '@/components/Avatar.vue'

const route = useRoute()
const router = useRouter()
const auth = useAuthStore()
const id = route.params.id as string

const member = ref<Staff | null>(null)
const tickets = ref<Ticket[]>([])
const loading = ref(true)
const error = ref('')
const saving = ref(false)

const isSelf = computed(() => id === auth.record?.id)
const form = ref({ name: '', email: '', role: 'agent' as 'agent' | 'admin', active: true })
const issued = ref<{ email: string; password: string } | null>(null)

function generatePassword(): string {
  const bytes = new Uint8Array(12)
  crypto.getRandomValues(bytes)
  return btoa(String.fromCharCode(...bytes)).replace(/[+/=]/g, '').slice(0, 16)
}

async function load() {
  loading.value = true
  error.value = ''
  try {
    member.value = await pb.collection('staff').getOne<Staff>(id)
    form.value = {
      name: member.value.name || '',
      email: member.value.email,
      role: member.value.role,
      active: member.value.active,
    }
    tickets.value = (
      await pb.collection('tickets').getList<Ticket>(1, 10, {
        filter: `assignee = '${id}' && status != 'resolved' && status != 'closed'`,
        sort: '-updated',
      })
    ).items
  } catch (err: any) {
    error.value = err?.message || 'Failed to load staff member'
  } finally {
    loading.value = false
  }
}

async function save() {
  saving.value = true
  error.value = ''
  // Never send role/active changes for yourself, even if the DOM was tampered.
  const data: Record<string, unknown> = { name: form.value.name, email: form.value.email }
  if (!isSelf.value) {
    data.role = form.value.role
    data.active = form.value.active
  }
  try {
    member.value = await pb.collection('staff').update<Staff>(id, data)
  } catch (err: any) {
    error.value = err?.data?.message || err?.message || 'Failed to save'
  } finally {
    saving.value = false
  }
}

async function resetPassword() {
  if (!member.value) return
  if (!confirm(`Reset the password for ${member.value.email}? Their current password stops working immediately.`)) return
  error.value = ''
  try {
    const password = generatePassword()
    await pb.collection('staff').update(id, { password, passwordConfirm: password })
    issued.value = { email: member.value.email, password }
  } catch (err: any) {
    error.value = err?.data?.message || err?.message || 'Failed to reset password'
  }
}

onMounted(load)
</script>

<template>
  <div v-if="loading" class="flex justify-center p-12"><span class="loading loading-spinner loading-lg"></span></div>

  <div v-else-if="member" class="space-y-4">
    <div class="breadcrumbs text-sm">
      <ul>
        <li><a @click="router.push('/staff/staff')">Staff</a></li>
        <li>{{ member.name || member.email }}</li>
      </ul>
    </div>
    <div class="flex items-center gap-3">
      <Avatar :record="member" :name="member.name || member.email" size="md" />
      <h1 class="text-2xl font-bold">{{ member.name || member.email }}</h1>
      <span v-if="isSelf" class="badge-soft badge-soft-neutral">you</span>
    </div>

    <div v-if="error" class="alert alert-error py-2 text-sm">{{ error }}</div>

    <div v-if="issued" class="alert alert-info py-2 text-sm">
      <span>
        Credentials for <b>{{ issued.email }}</b> — one-time password:
        <code class="font-mono font-bold">{{ issued.password }}</code>
        — share securely, then <button class="link" @click="issued = null">dismiss</button>.
      </span>
    </div>

    <div class="grid grid-cols-1 lg:grid-cols-2 gap-4 items-start">
      <div class="card bg-base-100 shadow-sm">
        <div class="card-body space-y-3">
          <h2 class="card-title text-base">Details</h2>
          <div class="form-control">
            <label class="label py-1"><span class="label-text">Name</span></label>
            <input v-model="form.name" type="text" class="input input-bordered input-sm" :disabled="saving" />
          </div>
          <div class="form-control">
            <label class="label py-1"><span class="label-text">Email</span></label>
            <input v-model="form.email" type="email" class="input input-bordered input-sm" :disabled="saving" />
          </div>
          <div class="form-control">
            <label class="label py-1"><span class="label-text">Role</span></label>
            <select v-model="form.role" class="select select-bordered select-sm" :disabled="saving || isSelf">
              <option value="agent">agent</option>
              <option value="admin">admin</option>
            </select>
            <label v-if="isSelf" class="label py-0"><span class="label-text-alt text-base-content/60">You can't change your own role.</span></label>
          </div>
          <div class="form-control">
            <label class="label cursor-pointer justify-start gap-3 py-1">
              <input v-model="form.active" type="checkbox" class="toggle toggle-success toggle-sm" :disabled="saving || isSelf" />
              <span class="label-text">Active</span>
            </label>
          </div>
          <div class="flex justify-between items-center pt-1">
            <button class="btn btn-ghost btn-sm" @click="resetPassword">Reset password</button>
            <button class="btn btn-primary btn-sm" :disabled="saving" @click="save">
              <span v-if="saving" class="loading loading-spinner loading-xs"></span>
              Save
            </button>
          </div>
        </div>
      </div>

      <div class="card bg-base-100 shadow-sm">
        <div class="card-body">
          <h2 class="card-title text-base">Open assigned tickets</h2>
          <div class="divide-y divide-base-200">
            <router-link
              v-for="t in tickets"
              :key="t.id"
              :to="`/staff/tickets/${t.id}`"
              class="flex items-center gap-3 py-2 hover:bg-base-200/50 -mx-2 px-2 rounded"
            >
              <span class="font-mono text-xs text-base-content/50 w-10">#{{ t.number }}</span>
              <span class="flex-1 truncate">{{ t.title }}</span>
              <TicketBadges :status="t.status" :priority="t.priority" />
            </router-link>
            <p v-if="tickets.length === 0" class="py-3 text-sm text-base-content/50">Nothing open assigned.</p>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>
