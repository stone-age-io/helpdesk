<script setup lang="ts">
// Slide-in reference for the notification template editor. Content mirrors the
// backend render payload (internal/notifications/context.go → TicketContext)
// and the helper FuncMap (internal/notifications/funcs.go) — keep the two in
// sync when either changes.
defineProps<{ open: boolean }>()
const emit = defineEmits<{ (e: 'close'): void }>()

// Wrap a field path in template delimiters in JS (not the Vue template) —
// literal {{ }} inside a mustache expression would break the Vue compiler.
const mustache = (t: string) => `{{${t}}}`
const exGuard = '{{if .Ticket.OldStatus}}…{{end}}'

interface VarRow {
  token: string
  desc: string
  only?: string
}
interface VarGroup {
  title: string
  note?: string
  vars: VarRow[]
}

const groups: VarGroup[] = [
  {
    title: 'Ticket',
    vars: [
      { token: '.Ticket.Number', desc: 'Sequential ticket number, e.g. 42' },
      { token: '.Ticket.Title', desc: 'Ticket title' },
      { token: '.Ticket.Body', desc: 'Ticket description' },
      { token: '.Ticket.Status', desc: 'Raw status (open, in_progress…)' },
      { token: '.Ticket.OldStatus', desc: 'Previous status', only: 'status_changed' },
      { token: '.Ticket.Priority', desc: 'low / normal / high / urgent' },
      { token: '.Ticket.Source', desc: 'portal / agent / nats / webhook' },
      { token: '.Ticket.URL', desc: 'Deep link to the ticket (empty if app URL unset)' },
      { token: '.Ticket.ID', desc: 'Record id (rarely needed)' },
    ],
  },
  {
    title: 'People & customer',
    vars: [
      { token: '.Customer', desc: 'Customer (company) name' },
      { token: '.Requester.Name', desc: 'Requester display name' },
      { token: '.Requester.Email', desc: 'Requester email' },
      { token: '.Assignee.Name', desc: 'Assigned agent name (empty if unassigned)' },
      { token: '.Assignee.Email', desc: 'Assigned agent email' },
    ],
  },
  {
    title: 'Comment',
    note: 'Populated only on ticket.commented.',
    vars: [
      { token: '.Comment.AuthorName', desc: 'Who wrote the comment' },
      { token: '.Comment.Body', desc: 'Comment text' },
      { token: '.Comment.ByStaff', desc: 'true if a staff member authored it' },
    ],
  },
  {
    title: 'Visit',
    note: 'Populated only on visit.scheduled / visit.rescheduled / visit.canceled.',
    vars: [
      { token: '.Visit.ScheduledAt', desc: 'Visit time — wrap in formatTime' },
      { token: '.Visit.OldScheduledAt', desc: 'Previous time', only: 'rescheduled' },
      { token: '.Visit.AssigneeName', desc: 'Dispatched technician' },
      { token: '.Visit.Location', desc: 'On-site location / directions' },
      { token: '.Visit.Notes', desc: 'Dispatch notes' },
    ],
  },
]

interface FuncRow {
  sig: string
  desc: string
  example: string
}
const funcs: FuncRow[] = [
  {
    sig: 'formatTime <time>',
    desc: 'Render a timestamp in the server timezone (Jan 2, 2006 3:04 PM).',
    example: '{{formatTime .Visit.ScheduledAt}}',
  },
  {
    sig: 'statusLabel <status>',
    desc: 'Humanize a status: in_progress → in progress.',
    example: '{{statusLabel .Ticket.Status}}',
  },
  {
    sig: 'pluralize <n> <noun>',
    desc: 'Count + noun, adding an s when n ≠ 1: 1 ticket / 3 tickets.',
    example: '{{pluralize 3 "ticket"}}',
  },
]
</script>

<template>
  <teleport to="body">
    <transition name="drawer-fade">
      <div v-if="open" class="fixed inset-0 z-[60]" role="dialog" aria-modal="true" aria-label="Template reference">
        <div class="absolute inset-0 bg-black/40" @click="emit('close')"></div>
        <transition name="drawer-slide">
          <aside
            v-if="open"
            class="absolute right-0 top-0 h-full w-full max-w-md bg-base-100 shadow-xl border-l border-base-300 flex flex-col"
          >
            <div class="flex items-center justify-between px-4 py-3 border-b border-base-300 flex-none">
              <h3 class="font-bold text-base">Template reference</h3>
              <button class="btn btn-ghost btn-sm btn-circle" aria-label="Close" @click="emit('close')">✕</button>
            </div>

            <div class="overflow-y-auto px-4 py-3 space-y-5 text-sm">
              <p class="text-base-content/70">
                Subjects and bodies are
                <a href="https://pkg.go.dev/text/template" target="_blank" rel="noopener" class="link">Go text/template</a>.
                Reference fields with <code class="text-xs">{{ mustache('.Field') }}</code>, and guard event-specific
                ones with <code class="text-xs">{{ exGuard }}</code>.
              </p>

              <section v-for="g in groups" :key="g.title" class="space-y-1.5">
                <h4 class="font-semibold text-xs uppercase tracking-wider text-base-content/60">{{ g.title }}</h4>
                <p v-if="g.note" class="text-xs text-warning/90">{{ g.note }}</p>
                <ul class="space-y-1">
                  <li v-for="v in g.vars" :key="v.token" class="flex flex-col gap-0.5 border-b border-base-200/70 pb-1 last:border-0">
                    <div class="flex items-center gap-2 flex-wrap">
                      <code class="text-xs bg-base-200 px-1.5 py-0.5 rounded">{{ mustache(v.token) }}</code>
                      <span v-if="v.only" class="badge badge-ghost badge-xs">{{ v.only }} only</span>
                    </div>
                    <span class="text-xs text-base-content/60">{{ v.desc }}</span>
                  </li>
                </ul>
              </section>

              <section class="space-y-1.5">
                <h4 class="font-semibold text-xs uppercase tracking-wider text-base-content/60">Helpers</h4>
                <ul class="space-y-2">
                  <li v-for="f in funcs" :key="f.sig" class="space-y-0.5 border-b border-base-200/70 pb-2 last:border-0">
                    <code class="text-xs bg-base-200 px-1.5 py-0.5 rounded">{{ f.sig }}</code>
                    <p class="text-xs text-base-content/60">{{ f.desc }}</p>
                    <code class="text-xs text-primary">{{ f.example }}</code>
                  </li>
                </ul>
              </section>
            </div>
          </aside>
        </transition>
      </div>
    </transition>
  </teleport>
</template>

<style scoped>
.drawer-fade-enter-active,
.drawer-fade-leave-active {
  transition: opacity 0.2s ease;
}
.drawer-fade-enter-from,
.drawer-fade-leave-to {
  opacity: 0;
}
.drawer-slide-enter-active,
.drawer-slide-leave-active {
  transition: transform 0.2s ease;
}
.drawer-slide-enter-from,
.drawer-slide-leave-to {
  transform: translateX(100%);
}
</style>
