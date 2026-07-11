<script setup lang="ts">
// Edit the logged-in account's display name and avatar (staff or requester).
// The users/staff self-update rules allow name + avatar (not customer/active/
// role); authRefresh re-pulls the record so the shells update immediately.
import { computed, ref } from 'vue'
import { pb } from '@/pb'
import { useAuthStore } from '@/stores/auth'
import Avatar from '@/components/Avatar.vue'

const emit = defineEmits<{ (e: 'close'): void }>()
const auth = useAuthStore()

const name = ref(auth.record?.name || '')
const saving = ref(false)
const error = ref('')
const done = ref(false)

// Pending avatar selection: a File to upload, '' to clear, or null for "leave
// as-is". previewUrl shows the picked file before it's saved.
const avatarFile = ref<File | null>(null)
const clearAvatar = ref(false)
const previewUrl = ref('')
const fileInput = ref<HTMLInputElement | null>(null)

const hasAvatar = computed(() => !!auth.record?.avatar && !clearAvatar.value)

function onPick(e: Event) {
  const f = (e.target as HTMLInputElement).files?.[0]
  if (!f) return
  avatarFile.value = f
  clearAvatar.value = false
  if (previewUrl.value) URL.revokeObjectURL(previewUrl.value)
  previewUrl.value = URL.createObjectURL(f)
}

function removeAvatar() {
  avatarFile.value = null
  clearAvatar.value = true
  if (previewUrl.value) URL.revokeObjectURL(previewUrl.value)
  previewUrl.value = ''
  if (fileInput.value) fileInput.value.value = ''
}

async function submit() {
  saving.value = true
  error.value = ''
  const collection = auth.record?.collectionName
  try {
    const data: Record<string, any> = { name: name.value.trim() }
    if (avatarFile.value) data.avatar = avatarFile.value
    else if (clearAvatar.value) data.avatar = null
    await pb.collection(collection).update(auth.record.id, data)
    await pb.collection(collection).authRefresh()
    done.value = true
    setTimeout(() => emit('close'), 900)
  } catch (err: any) {
    error.value = err?.data?.message || err?.message || 'Failed to save profile'
  } finally {
    saving.value = false
  }
}
</script>

<template>
  <dialog class="modal modal-open">
    <div class="modal-box max-w-sm">
      <h3 class="font-bold text-lg mb-3">Edit profile</h3>
      <div v-if="done" class="alert alert-success py-2 text-sm">Saved.</div>
      <form v-else class="space-y-3" @submit.prevent="submit">
        <div v-if="error" class="alert alert-error py-2 text-sm">{{ error }}</div>

        <div class="flex items-center gap-3">
          <!-- Preview: picked file, then existing avatar, then initials. -->
          <div v-if="previewUrl" class="avatar">
            <div class="w-12 rounded-full"><img :src="previewUrl" alt="New avatar preview" class="object-cover" /></div>
          </div>
          <Avatar v-else :record="hasAvatar ? auth.record : null" :name="name || auth.record?.email" size="md" />
          <div class="flex flex-col gap-1">
            <input
              ref="fileInput"
              type="file"
              accept="image/png,image/jpeg,image/webp,image/gif"
              class="file-input file-input-bordered file-input-xs w-full max-w-[12rem]"
              :disabled="saving"
              @change="onPick"
            />
            <button
              v-if="hasAvatar || avatarFile"
              type="button"
              class="btn btn-ghost btn-xs w-fit text-error"
              :disabled="saving"
              @click="removeAvatar"
            >
              Remove photo
            </button>
          </div>
        </div>

        <div class="form-control">
          <label class="label py-1"><span class="label-text">Name</span></label>
          <input v-model="name" type="text" class="input input-bordered input-sm" maxlength="150" :disabled="saving" />
        </div>
        <div class="text-xs text-base-content/50">{{ auth.record?.email }}</div>
        <div class="modal-action mt-2">
          <button type="button" class="btn btn-ghost btn-sm" :disabled="saving" @click="emit('close')">Cancel</button>
          <button type="submit" class="btn btn-primary btn-sm" :disabled="saving || !name.trim()">
            <span v-if="saving" class="loading loading-spinner loading-xs"></span>
            Save
          </button>
        </div>
      </form>
    </div>
    <form method="dialog" class="modal-backdrop"><button @click.prevent="emit('close')">close</button></form>
  </dialog>
</template>
