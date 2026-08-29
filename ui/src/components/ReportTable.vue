<script setup lang="ts">
// The one table every Reports rollup renders through. Seven rollups with seven
// hand-written <table> blocks had already drifted apart in small ways, and the
// proportional bar below would have meant writing the same absolute-positioned
// div seven more times.
//
// Deliberately NOT ResponsiveList: that one keys on `item.id` (rollup rows have
// no id — they are keyed by their label) and drops to one card per row on
// mobile, which is the wrong shape for a dense numeric table. Here the table
// stays a table and scrolls sideways instead.
//
// The first column is the dimension; the rest are measures. Exactly one measure
// carries `bar`, and it should be the one the caller sorted by — the bar is a
// reading aid for the ranking, so a bar that disagrees with the row order would
// be worse than no bar at all.
import { computed } from 'vue'

export interface ReportColumn {
  key: string
  label: string
  /** Right-align + tabular figures. Every measure column wants this. */
  numeric?: boolean
  /** Render the value as minutes → "3h 15m" rather than a bare count. */
  hours?: boolean
  /** Draw the proportional bar from this column. At most one per table. */
  bar?: boolean
}

const props = withDefaults(
  defineProps<{
    columns: ReportColumn[]
    rows: Record<string, any>[]
    /** What the dimension cell shows for the unattributed bucket. */
    nullLabel?: string
    empty?: string
  }>(),
  { nullLabel: 'Unattributed', empty: 'No activity in range.' },
)

const dimensionKey = computed(() => props.columns[0]?.key || 'label')
const barKey = computed(() => props.columns.find((c) => c.bar)?.key || '')

function isNull(row: Record<string, any>): boolean {
  return row[dimensionKey.value] === '—'
}

// Scaled against the largest ATTRIBUTED value, so the real leader fills the row
// and the tail keeps its shape. Excluding the unattributed bucket matters more
// than it sounds: on the thing rollup it is routinely the biggest row by far
// (most reactive tickets name no device), and scaling to it squashed every
// actual device into the left quarter of the track — the ranking you came to
// read became the part of the chart you could not see. The null row still draws
// a bar, clamped at full width, which reads correctly as "off this scale".
const barMax = computed(() => {
  if (!barKey.value) return 0
  const attributed = props.rows.filter((r) => !isNull(r)).map((r) => Number(r[barKey.value]) || 0)
  return Math.max(0, ...attributed)
})
function barPct(row: Record<string, any>): number {
  if (!barMax.value) return 0
  return Math.min(100, ((Number(row[barKey.value]) || 0) / barMax.value) * 100)
}

function fmtHours(m: number): string {
  if (!m) return '—'
  const h = Math.floor(m / 60)
  return h > 0 ? `${h}h ${m % 60}m` : `${m}m`
}

function cell(col: ReportColumn, row: Record<string, any>): string {
  const v = row[col.key]
  if (col.hours) return fmtHours(Number(v) || 0)
  if (col.numeric) return v ? String(v) : '—'
  return v == null || v === '' ? '—' : String(v)
}
</script>

<template>
  <div class="overflow-x-auto">
    <table class="table table-sm">
      <thead>
        <tr>
          <th v-for="c in columns" :key="c.key" :class="c.numeric ? 'text-right' : ''">{{ c.label }}</th>
        </tr>
      </thead>
      <tbody>
        <tr v-for="(r, ri) in rows" :key="String(r[dimensionKey]) || ri">
          <td
            v-for="(c, ci) in columns"
            :key="c.key"
            :class="[c.numeric ? 'text-right font-mono tabular-nums' : '', ci === 0 ? 'relative' : '']"
          >
            <!-- The bar sits behind the dimension cell, so the measure columns
                 stay a clean numeric block. The unattributed row still draws one
                 (hiding it would misrepresent the total) but in a flat neutral,
                 so it doesn't read as part of the ranking it sits outside of. -->
            <div
              v-if="ci === 0 && barPct(r) > 0"
              class="absolute inset-y-px left-0 rounded-r"
              :class="isNull(r) ? 'bg-base-content/10' : 'bg-primary/15'"
              :style="{ width: barPct(r) + '%' }"
              aria-hidden="true"
            ></div>
            <span v-if="ci === 0" class="relative">
              <slot name="label" :row="r">
                <span v-if="isNull(r)" class="text-base-content/50">{{ nullLabel }}</span>
                <span v-else>{{ cell(c, r) }}</span>
              </slot>
            </span>
            <template v-else>{{ cell(c, r) }}</template>
          </td>
        </tr>
        <tr v-if="rows.length === 0">
          <td :colspan="columns.length" class="text-base-content/50">{{ empty }}</td>
        </tr>
      </tbody>
    </table>
  </div>
</template>
