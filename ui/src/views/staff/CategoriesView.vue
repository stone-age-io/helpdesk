<script setup lang="ts">
// Admin-managed ticket categories: add, rename, recolor, reorder, retire.
// Reached only by admins (route meta.adminOnly + collection rules). Retiring
// (active=false) is preferred over deleting — a deleted category leaves
// historical tickets with a blank category; deactivating just hides it from
// the pickers while keeping the label on old tickets.
//
// Reads as a roster (read-only rows, edit via a panel above the list) to match
// the other staff list views via the shared ResponsiveList.
import { nextTick, onMounted, ref } from 'vue'
import { pb } from '@/pb'
import type { TicketCategory } from '@/types'
import CategoryBadge from '@/components/CategoryBadge.vue'
import ActiveBadge from '@/components/ActiveBadge.vue'
import ResponsiveList, { type Column } from '@/components/ResponsiveList.vue'

const DEFAULT_COLOR = '#6b7280'

const columns: Column<TicketCategory>[] = [
  { key: 'name', label: 'Name' },
  { key: 'key', label: 'Key' },
  { key: 'color', label: 'Color' },
  { key: 'sort_order', label: 'Order' },
  { key: 'active', label: 'Status' },
]

const categories = ref<TicketCategory[]>([])
const loading = ref(true)
const error = ref('')
const saving = ref(false)

// New-category form.
const newName = ref('')
const newColor = ref(DEFAULT_COLOR)
const creating = ref(false)

// Inline row editing (admin), in a panel above the list.
const editing = ref<TicketCategory | null>(null)
const editForm = ref({ name: '', key: '', color: DEFAULT_COLOR, sort_order: 0 })

function slugify(s: string): string {
  return s
    .toLowerCase()
    .trim()
    .replace(/[^a-z0-9]+/g, '-')
    .replace(/^-+|-+$/g, '')
}

async function load() {
  loading.value = true
  error.value = ''
  try {
    categories.value = await pb
      .collection('ticket_categories')
      .getFullList<TicketCategory>({ sort: 'sort_order,name' })
  } catch (err: any) {
    error.value = err?.message || 'Failed to load categories'
  } finally {
    loading.value = false
  }
}

async function create() {
  const name = newName.value.trim()
  if (!name) return
  creating.value = true
  error.value = ''
  try {
    await pb.collection('ticket_categories').create({
      name,
      key: slugify(name),
      active: true,
      color: newColor.value,
      sort_order: (categories.value.at(-1)?.sort_order || 0) + 1,
    })
    newName.value = ''
    newColor.value = DEFAULT_COLOR
    await load()
  } catch (err: any) {
    error.value = err?.message || 'Failed to create category (name/key must be unique)'
  } finally {
    creating.value = false
  }
}

// The edit panel renders above the list, which can be off-screen when the
// triggering row is below the fold — bring it into view.
const editCard = ref<HTMLElement | null>(null)
function startEdit(cat: TicketCategory) {
  editing.value = cat
  editForm.value = {
    name: cat.name || '',
    key: cat.key || '',
    color: cat.color || DEFAULT_COLOR,
    sort_order: cat.sort_order,
  }
  nextTick(() => editCard.value?.scrollIntoView({ behavior: 'smooth', block: 'nearest' }))
}

async function saveEdit() {
  if (!editing.value) return
  saving.value = true
  error.value = ''
  try {
    await pb.collection('ticket_categories').update(editing.value.id, {
      name: editForm.value.name.trim(),
      key: slugify(editForm.value.key || editForm.value.name),
      color: editForm.value.color,
      sort_order: editForm.value.sort_order,
    })
    editing.value = null
    await load()
  } catch (err: any) {
    error.value = err?.message || 'Failed to save (name/key must be unique)'
  } finally {
    saving.value = false
  }
}

async function toggleActive(cat: TicketCategory) {
  error.value = ''
  try {
    await pb.collection('ticket_categories').update(cat.id, { active: !cat.active })
    await load()
  } catch (err: any) {
    error.value = err?.message || 'Failed to update'
  }
}

async function remove(cat: TicketCategory) {
  if (!confirm(`Delete “${cat.name}”? Tickets already tagged with it keep the label until re-classified. Consider deactivating instead.`)) return
  error.value = ''
  try {
    await pb.collection('ticket_categories').delete(cat.id)
    if (editing.value?.id === cat.id) editing.value = null
    await load()
  } catch (err: any) {
    error.value = err?.message || 'Failed to delete'
  }
}

onMounted(load)
</script>

<template>
  <div class="space-y-4">
    <h1 class="text-2xl font-bold">Categories</h1>
    <p class="text-sm text-base-content/60">
      What tickets are about — used for filtering and reporting. Staff classify
      tickets; requesters never see this list.
    </p>

    <div v-if="error" class="alert alert-error py-2 text-sm">{{ error }}</div>

    <!-- New category -->
    <form class="flex flex-col sm:flex-row gap-2 sm:items-end" @submit.prevent="create">
      <div class="form-control">
        <label class="label py-1"><span class="label-text text-xs">New category</span></label>
        <input v-model="newName" type="text" placeholder="e.g. VoIP" class="input input-bordered input-sm w-full sm:w-64" :disabled="creating" />
      </div>
      <div class="form-control">
        <label class="label py-1"><span class="label-text text-xs">Color</span></label>
        <input v-model="newColor" type="color" class="input input-bordered input-sm w-16 p-1" :disabled="creating" />
      </div>
      <button type="submit" class="btn btn-primary btn-sm" :disabled="creating || !newName.trim()">Add</button>
    </form>

    <!-- Edit panel: lives above the list (an inline table row can't render
         inside the mobile card layout). -->
    <div v-if="editing" ref="editCard" class="card bg-base-100 shadow-sm">
      <div class="card-body p-4 space-y-2">
        <h2 class="card-title text-sm">Edit {{ editing.name }}</h2>
        <form class="flex flex-col sm:flex-row sm:flex-wrap gap-2 items-stretch sm:items-end" @submit.prevent="saveEdit">
          <div class="form-control">
            <label class="label py-1"><span class="label-text text-xs">Name *</span></label>
            <input v-model="editForm.name" type="text" class="input input-bordered input-sm w-full sm:w-48" :disabled="saving" />
          </div>
          <div class="form-control">
            <label class="label py-1"><span class="label-text text-xs">Key</span></label>
            <input v-model="editForm.key" type="text" class="input input-bordered input-sm w-full sm:w-40 font-mono" :disabled="saving" />
          </div>
          <div class="form-control">
            <label class="label py-1"><span class="label-text text-xs">Color</span></label>
            <input v-model="editForm.color" type="color" class="input input-bordered input-sm w-16 p-1" :disabled="saving" />
          </div>
          <div class="form-control">
            <label class="label py-1"><span class="label-text text-xs">Order</span></label>
            <input v-model.number="editForm.sort_order" type="number" class="input input-bordered input-sm w-full sm:w-24" :disabled="saving" />
          </div>
          <div class="flex gap-2">
            <button type="submit" class="btn btn-primary btn-sm" :disabled="saving || !editForm.name.trim()">
              <span v-if="saving" class="loading loading-spinner loading-xs"></span>
              Save
            </button>
            <button type="button" class="btn btn-ghost btn-sm" :disabled="saving" @click="editing = null">Cancel</button>
          </div>
        </form>
      </div>
    </div>

    <div v-if="loading" class="flex justify-center p-12"><span class="loading loading-spinner loading-lg"></span></div>

    <ResponsiveList v-else :items="categories" :columns="columns" :clickable="false">
      <template #cell-name="{ item }"><CategoryBadge :name="item.name" :color="item.color" /></template>
      <template #card-name="{ item }"><CategoryBadge :name="item.name" :color="item.color" /></template>
      <template #cell-key="{ value }"><span class="font-mono text-xs">{{ value || '—' }}</span></template>
      <template #cell-color="{ item }">
        <span class="inline-flex items-center gap-1.5">
          <span class="inline-block h-4 w-4 rounded border border-base-300 shrink-0" :style="{ backgroundColor: item.color || DEFAULT_COLOR }"></span>
          <span class="font-mono text-xs text-base-content/60">{{ item.color || '—' }}</span>
        </span>
      </template>
      <template #cell-sort_order="{ value }"><span class="text-sm tabular-nums">{{ value }}</span></template>
      <template #cell-active="{ value }"><ActiveBadge :active="value" /></template>
      <template #actions="{ item }">
        <button class="btn btn-ghost btn-xs" @click="editing?.id === item.id ? (editing = null) : startEdit(item)">
          {{ editing?.id === item.id ? 'Cancel' : 'Edit' }}
        </button>
        <button class="btn btn-ghost btn-xs" @click="toggleActive(item)">{{ item.active ? 'Deactivate' : 'Activate' }}</button>
        <button class="btn btn-ghost btn-xs text-error" @click="remove(item)">Delete</button>
      </template>
      <template #empty>
        <span class="text-base-content/60">No categories yet.</span>
      </template>
    </ResponsiveList>
  </div>
</template>
