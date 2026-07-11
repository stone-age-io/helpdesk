<script setup lang="ts">
// Requester-facing status stepper. Collapses the raw event churn (a status can
// bounce back and forth) into the canonical lifecycle a requester cares about.
// "Current" is derived from the ticket's status now — not the furthest stage
// ever reached — so a reopened ticket reads sensibly instead of regressing.
// Reused by the desktop rail and the mobile collapsible panel.
import { computed } from 'vue'
import type { Ticket, TicketEvent } from '@/types'
import { format } from 'date-fns'

const props = defineProps<{ ticket: Ticket; statusEvents: TicketEvent[] }>()

const STEPS = ['Open', 'In progress', 'Resolved'] as const
function stageIndex(status?: string): number {
  if (status === 'resolved' || status === 'closed') return 2
  if (status === 'in_progress' || status === 'waiting') return 1
  return 0
}
const currentStage = computed(() => stageIndex(props.ticket.status))
const stepLabel = (i: number) => (i === 2 && props.ticket.status === 'closed' ? 'Closed' : STEPS[i])
// First time the ticket entered a stage (stage 0 = creation).
function stepReachedAt(i: number): string {
  if (i === 0) return props.ticket.created || ''
  return props.statusEvents.find((e) => stageIndex(e.new_value) === i)?.created || ''
}
</script>

<template>
  <div>
    <h2 class="font-semibold text-sm mb-2">Progress</h2>
    <ol>
      <li v-for="(_, i) in STEPS" :key="i" class="flex gap-3 relative pb-4 last:pb-0">
        <!-- connector -->
        <span
          v-if="i < STEPS.length - 1"
          class="absolute left-[5px] top-3.5 -bottom-0.5 w-px"
          :class="i < currentStage ? 'bg-primary' : 'bg-base-300'"
        ></span>
        <!-- dot -->
        <span
          class="relative z-10 mt-1 w-2.5 h-2.5 rounded-full shrink-0"
          :class="
            i < currentStage
              ? 'bg-primary'
              : i === currentStage
                ? 'bg-primary ring-4 ring-primary/20'
                : 'bg-base-300'
          "
        ></span>
        <div class="flex-1 -mt-0.5">
          <div class="text-sm" :class="i <= currentStage ? 'font-medium' : 'text-base-content/40'">
            {{ stepLabel(i) }}
          </div>
          <div v-if="i <= currentStage && stepReachedAt(i)" class="text-xs text-base-content/50">
            {{ format(new Date(stepReachedAt(i)), 'MMM d, HH:mm') }}
          </div>
        </div>
      </li>
    </ol>
  </div>
</template>
