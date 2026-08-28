<script setup lang="ts">
import { computed } from 'vue'

// Minimal JSON Schema form: renders top-level primitive properties of an
// object schema. Nested objects / arrays fall back to a JSON textarea so the
// user can still edit them. Deliberately covers the common case only; complex
// schemas can upgrade later without changing the contract.
//
// Ported from the sibling platform app (ui/src/components/common/JsonSchemaForm.vue),
// which is where the schemas themselves are authored. The helpdesk is downstream:
// it renders a type's metadata_schema, it never builds one — so the platform's
// SchemaBuilder/SchemaFieldEditor pair is deliberately NOT ported.

interface Props {
  schema: any
  modelValue: Record<string, any>
  disabled?: boolean
}

const props = withDefaults(defineProps<Props>(), { disabled: false })
const emit = defineEmits<{
  (e: 'update:modelValue', value: Record<string, any>): void
}>()

interface Field {
  name: string
  type: string
  format?: string
  required: boolean
  description?: string
  title?: string
  enumValues?: any[]
}

const fields = computed<Field[]>(() => {
  const s = props.schema
  if (!s || s.type !== 'object' || !s.properties) return []
  const required = new Set<string>(Array.isArray(s.required) ? s.required : [])
  return Object.entries(s.properties).map(([name, raw]: [string, any]) => ({
    name,
    type: raw.type || 'string',
    format: raw.format,
    required: required.has(name),
    description: raw.description,
    title: raw.title,
    enumValues: raw.enum,
  }))
})

function setField(name: string, value: any) {
  emit('update:modelValue', { ...props.modelValue, [name]: value })
}

function inputType(f: Field): string {
  if (f.type === 'boolean') return 'checkbox'
  if (f.type === 'integer' || f.type === 'number') return 'number'
  if (f.format === 'date-time') return 'datetime-local'
  if (f.format === 'date') return 'date'
  if (f.format === 'time') return 'time'
  return 'text'
}

function isPrimitive(f: Field): boolean {
  return ['string', 'integer', 'number', 'boolean'].includes(f.type)
}

function castOnInput(f: Field, raw: string): any {
  if (f.type === 'integer') {
    const n = parseInt(raw, 10)
    return Number.isNaN(n) ? '' : n
  }
  if (f.type === 'number') {
    const n = parseFloat(raw)
    return Number.isNaN(n) ? '' : n
  }
  return raw
}
</script>

<template>
  <div v-if="fields.length === 0" class="text-xs opacity-60 italic">
    Schema has no top-level properties to render.
  </div>
  <div v-else class="space-y-3">
    <div v-for="f in fields" :key="f.name" class="form-control">
      <label class="label py-1">
        <span class="label-text text-xs">
          <code class="font-mono">{{ f.name }}</code>
          <span v-if="f.required" class="text-error ml-1">*</span>
          <span v-if="f.title" class="text-base-content/60 ml-2">{{ f.title }}</span>
        </span>
      </label>

      <!-- Enum → select -->
      <select
        v-if="f.enumValues && f.enumValues.length"
        class="select select-bordered select-sm"
        :value="modelValue[f.name] ?? ''"
        :disabled="disabled"
        @change="setField(f.name, ($event.target as HTMLSelectElement).value)"
      >
        <option value="">— —</option>
        <option v-for="opt in f.enumValues" :key="String(opt)" :value="opt">{{ opt }}</option>
      </select>

      <!-- Boolean → toggle, with the literal value beside it. This value round-trips
           to and from the platform, so show `true`/`false` rather than leaving the
           reader to infer it from a switch position. -->
      <label v-else-if="f.type === 'boolean'" class="flex items-center gap-2">
        <input
          type="checkbox"
          class="toggle toggle-primary"
          :checked="!!modelValue[f.name]"
          :disabled="disabled"
          @change="setField(f.name, ($event.target as HTMLInputElement).checked)"
        />
        <code class="text-xs text-base-content/60">{{ modelValue[f.name] ? 'true' : 'false' }}</code>
      </label>

      <!-- Primitive scalar → typed input -->
      <input
        v-else-if="isPrimitive(f)"
        :type="inputType(f)"
        class="input input-bordered input-sm font-mono"
        :value="modelValue[f.name] ?? ''"
        :disabled="disabled"
        :required="f.required"
        @input="setField(f.name, castOnInput(f, ($event.target as HTMLInputElement).value))"
      />

      <!-- Fallback (object / array / unknown) → JSON textarea -->
      <textarea
        v-else
        class="textarea textarea-bordered font-mono text-xs"
        rows="3"
        :value="typeof modelValue[f.name] === 'string' ? modelValue[f.name] : JSON.stringify(modelValue[f.name] ?? null, null, 2)"
        :disabled="disabled"
        @blur="(e) => {
          try { setField(f.name, JSON.parse((e.target as HTMLTextAreaElement).value)) } catch { /* keep raw string */ setField(f.name, (e.target as HTMLTextAreaElement).value) }
        }"
      ></textarea>

      <div v-if="f.description" class="text-[10px] text-base-content/60 mt-0.5">{{ f.description }}</div>
    </div>
  </div>
</template>
