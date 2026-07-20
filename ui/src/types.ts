// Record interfaces mirroring migrations/1800000000_init.go.

export interface BaseRecord {
  id: string
  created: string
  updated?: string
  collectionName?: string
  expand?: Record<string, any>
}

// `field` is a mobile, on-site variant of `agent` (migration 1816000000) — same
// staff access, a different SPA shell. `agent`/`admin` keep the desk app.
export type StaffRole = 'agent' | 'field' | 'admin'

export interface Staff extends BaseRecord {
  email: string
  name: string
  role: StaffRole
  active: boolean
  avatar?: string
}

export interface Requester extends BaseRecord {
  email: string
  name: string
  customer: string
  active: boolean
  avatar?: string
  // The requester's direct line (migration 1812000000); the on-site contact
  // lives on the location, not here.
  phone?: string
}

export interface Customer extends BaseRecord {
  name: string
  active: boolean
  platform_org_id?: string
  notes?: string
  // Opt-in: expose the aggregate time logged on this customer's tickets to
  // their portal requesters (default false). Only the total, never entries.
  show_time_to_requester?: boolean
}

// Service delivery (migrations/1812000000). A location is a customer's physical
// place; `code` is the platform Location join key. A project groups 1..N tickets
// (installs + reactive work) at a location over a target window.
export interface Location extends BaseRecord {
  customer: string
  code?: string
  name: string
  address?: string
  notes?: string
  contact?: string
  contact_phone?: string
  // Optional coordinates (migration 1813000000): back the map pin and the
  // maps deep link on a ticket. A hand-entered site may have neither.
  lat?: number
  lng?: number
}

export type ProjectStatus = 'planned' | 'active' | 'completed' | 'canceled'

export interface Project extends BaseRecord {
  number: number
  customer: string
  location?: string
  title: string
  description?: string
  status: ProjectStatus
  start_date?: string
  target_date?: string
  lead?: string
}

export type TicketStatus = 'open' | 'in_progress' | 'waiting' | 'resolved' | 'closed'
export type TicketPriority = 'low' | 'normal' | 'high' | 'urgent'
export type TicketSource = 'portal' | 'agent' | 'nats' | 'webhook'
export type TicketType = 'issue' | 'install'

// Admin-managed classification (migrations/1806000000). `key` is the stable
// slug used in filters and machine payloads; `name` is display-only.
export interface TicketCategory extends BaseRecord {
  name: string
  key: string
  active: boolean
  sort_order: number
  color?: string
}

export interface Ticket extends BaseRecord {
  number: number
  customer: string
  title: string
  body?: string
  status: TicketStatus
  priority: TicketPriority
  assignee?: string
  requester?: string
  source: TicketSource
  origin_subject?: string
  attachments?: string[]
  // Reactive issue vs. planned install (staff-set; defaults to issue).
  type?: TicketType
  // Optional grouping into a project (installation / field work).
  project?: string
  // Staff estimate of effort in minutes (nil = unestimated); compared against
  // the logged time_entries total and rolled up per project.
  estimated_minutes?: number
  // Classification: what the ticket is about (staff-set) + provenance.
  category?: string
  asset?: string
  // Structured place (relation to locations) — the reporting axis. location_note
  // is free text: dispatch hints, or the unmatched-code fallback from intake.
  location?: string
  location_note?: string
  // Derived (server-maintained): the last public comment was staff's and the
  // ticket is still open, i.e. the requester's reply is what's holding it up.
  awaiting_requester?: boolean
  // When the ticket entered `resolved` (cleared when it leaves). Drives the
  // auto-close cron; nil unless currently resolved.
  resolved_at?: string
}

export interface TicketComment extends BaseRecord {
  ticket: string
  author_staff?: string
  author_user?: string
  body: string
  internal: boolean
  attachments?: string[]
}

export interface TicketEvent extends BaseRecord {
  ticket: string
  field: 'status' | 'priority' | 'assignee'
  old_value?: string
  new_value?: string
  actor_staff?: string
  actor_user?: string
}

export interface TimeEntry extends BaseRecord {
  ticket: string
  staff: string
  minutes: number
  work_date: string
  note?: string
  // Optional on-site session this labor belongs to. Empty = desk work. The
  // ticket stays the canonical ledger; this is an added dimension.
  visit?: string
  // Labor not to be invoiced (rework, goodwill). Stored as the exception so the
  // default (unset = false) means billable; reports split on it and the
  // customer-facing time total excludes it.
  non_billable?: boolean
}

// A running timer: at most one open row per agent (unique index on staff, see
// the time_sessions migration). Deleted on stop/cancel — the durable record is
// the TimeEntry the stop route mints from it. `expand.ticket` / `expand.visit`
// are loaded for the timer bar's label.
export interface TimeSession extends BaseRecord {
  staff: string
  ticket: string
  visit?: string
  started_at: string
  note?: string
}

export type VisitStatus = 'requested' | 'scheduled' | 'completed' | 'canceled'

// A `requested` visit has no assignee/time yet — an agent promoted the
// ticket to on-site work; the dispatcher schedules it later.
export interface Visit extends BaseRecord {
  ticket: string
  assignee?: string
  scheduled_at?: string
  completed_at?: string
  status: VisitStatus
  location?: string
  notes?: string
  // Scheduled block length in minutes (planned), distinct from the actual
  // labor logged against the visit in time_entries.
  duration_minutes?: number
}

export const TICKET_STATUSES: TicketStatus[] = ['open', 'in_progress', 'waiting', 'resolved', 'closed']
export const TICKET_PRIORITIES: TicketPriority[] = ['low', 'normal', 'high', 'urgent']
export const TICKET_TYPES: TicketType[] = ['issue', 'install']
export const VISIT_STATUSES: VisitStatus[] = ['requested', 'scheduled', 'completed', 'canceled']
export const PROJECT_STATUSES: ProjectStatus[] = ['planned', 'active', 'completed', 'canceled']

// Shapes served by the /api/helpdesk/notifications routes (not raw records).

export interface NotificationRecipients {
  requester: boolean
  assignee: boolean
  all_staff: boolean
  extras: string[]
}

export interface NotificationTemplate {
  id: string
  event_type: string
  name: string
  enabled: boolean
  publish_nats: boolean
  subject: string
  body: string
  updated: string
  updated_by: string
  recipients: NotificationRecipients
}

export interface NotificationSendLog extends BaseRecord {
  event_type: string
  template?: string
  recipient: string
  status: 'sent' | 'failed' | 'skipped'
  error?: string
  channel?: 'email' | 'nats'
  payload_summary?: string
}
