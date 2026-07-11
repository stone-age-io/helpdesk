<script setup lang="ts">
// Admin-managed ticket categories: add, rename, recolor, reorder, retire.
// Reached only by admins (route meta.adminOnly + collection rules). Retiring
// (active=false) is preferred over deleting — a deleted category leaves
// historical tickets with a blank category; deactivating just hides it from
// the pickers while keeping the label on old tickets.
import { onMounted, ref } from 'vue'
import { pb } from '@/pb'
import type { TicketCategory } from '@/types'
import CategoryBadge from '@/components/CategoryBadge.vue'

const categories = ref<TicketCategory[]>([])
const loading = ref(true)
const error = ref('')
const savingId = ref('')

// New-category form.
const newName = ref('')
const newColor = ref('#6b7280')
const creating = ref(false)

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
    newColor.value = '#6b7280'
    await load()
  } catch (err: any) {
    error.value = err?.message || 'Failed to create category (name/key must be unique)'
  } finally {
    creating.value = false
  }
}

async function save(cat: TicketCategory) {
  savingId.value = cat.id
  error.value = ''
  try {
    await pb.collection('ticket_categories').update(cat.id, {
      name: cat.name.trim(),
      key: slugify(cat.key || cat.name),
      color: cat.color,
      active: cat.active,
      sort_order: cat.sort_order,
    })
    await load()
  } catch (err: any) {
    error.value = err?.message || 'Failed to save (name/key must be unique)'
  } finally {
    savingId.value = ''
  }
}

async function remove(cat: TicketCategory) {
  if (!confirm(`Delete “${cat.name}”? Tickets already tagged with it keep the label until re-classified. Consider deactivating instead.`)) return
  error.value = ''
  try {
    await pb.collection('ticket_categories').delete(cat.id)
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

    <div v-if="loading" class="flex justify-center p-12"><span class="loading loading-spinner loading-lg"></span></div>

    <div v-else class="overflow-x-auto bg-base-100 rounded-lg shadow-sm">
      <table class="table table-sm">
        <thead>
          <tr>
            <th>Preview</th>
            <th>Name</th>
            <th>Key</th>
            <th class="w-16">Color</th>
            <th class="w-20">Order</th>
            <th class="w-20">Active</th>
            <th></th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="cat in categories" :key="cat.id">
            <td><CategoryBadge :name="cat.name" :color="cat.color" /></td>
            <td><input v-model="cat.name" type="text" class="input input-bordered input-xs w-40" /></td>
            <td><input v-model="cat.key" type="text" class="input input-bordered input-xs w-40 font-mono" /></td>
            <td><input v-model="cat.color" type="color" class="input input-bordered input-xs w-12 p-0.5" /></td>
            <td><input v-model.number="cat.sort_order" type="number" class="input input-bordered input-xs w-16" /></td>
            <td>
              <input v-model="cat.active" type="checkbox" class="toggle toggle-success toggle-sm" />
            </td>
            <td class="text-right whitespace-nowrap">
              <button class="btn btn-ghost btn-xs" :disabled="savingId === cat.id" @click="save(cat)">
                <span v-if="savingId === cat.id" class="loading loading-spinner loading-xs"></span>
                Save
              </button>
              <button class="btn btn-ghost btn-xs text-error" @click="remove(cat)">Delete</button>
            </td>
          </tr>
          <tr v-if="categories.length === 0">
            <td colspan="7" class="text-base-content/50">No categories yet.</td>
          </tr>
        </tbody>
      </table>
    </div>
  </div>
</template>
