<script setup lang="ts">
import { computed } from 'vue'
import { pb } from '@/pb'

// Round avatar with an initials fallback. Pass a PocketBase record carrying an
// `avatar` file field (a staff/users record, or one expanded onto another
// record — e.g. a comment's author_staff) plus, optionally, an explicit name.
// With no uploaded file we render initials on a color deterministically
// derived from the name, so the same person is always the same color.
const props = withDefaults(
  defineProps<{
    record?: Record<string, any> | null
    name?: string
    size?: 'xs' | 'sm' | 'md'
  }>(),
  { size: 'sm' },
)

// Inner box sizing (daisyUI avatar needs an explicit width on the shape div).
const boxClass: Record<string, string> = {
  xs: 'w-6 text-[10px]',
  sm: 'w-8 text-xs',
  md: 'w-12 text-base',
}
const thumb: Record<string, string> = { xs: '40x40', sm: '40x40', md: '100x100' }

const displayName = computed(() => (props.name || props.record?.name || props.record?.email || '').trim())

const initials = computed(() => {
  const n = displayName.value
  if (!n) return '?'
  const parts = n.split(/\s+/)
  if (parts.length >= 2) return (parts[0][0] + parts[1][0]).toUpperCase()
  return n.slice(0, 2).toUpperCase()
})

const file = computed<string>(() => {
  const a = props.record?.avatar
  return Array.isArray(a) ? a[0] || '' : a || ''
})

const src = computed(() => {
  if (!file.value || !props.record?.id) return ''
  try {
    return pb.files.getURL(props.record, file.value, { thumb: thumb[props.size] })
  } catch {
    return ''
  }
})

// Deterministic hue from the name → a stable per-person color for the
// placeholder. Muted saturation/lightness reads well in both themes.
const placeholderStyle = computed(() => {
  const n = displayName.value
  let h = 0
  for (let i = 0; i < n.length; i++) h = (h * 31 + n.charCodeAt(i)) % 360
  return { backgroundColor: `hsl(${h} 45% 42%)`, color: 'white' }
})
</script>

<template>
  <div class="avatar shrink-0" :class="{ placeholder: !src }" :title="displayName">
    <div class="rounded-full" :class="boxClass[size]" :style="src ? undefined : placeholderStyle">
      <img v-if="src" :src="src" :alt="displayName" class="object-cover" />
      <span v-else class="font-bold leading-none">{{ initials }}</span>
    </div>
  </div>
</template>
