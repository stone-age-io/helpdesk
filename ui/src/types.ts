// Record interfaces mirroring migrations/1800000000_init.go.

export interface BaseRecord {
  id: string
  created: string
  updated?: string
  collectionName?: string
  expand?: Record<string, any>
}

export interface Staff extends BaseRecord {
  email: string
  name: string
  role: 'agent' | 'admin'
  active: boolean
}

export interface Requester extends BaseRecord {
  email: string
  name: string
  customer: string
  active: boolean
}

export interface Customer extends BaseRecord {
  name: string
  active: boolean
  platform_org_id?: string
  notes?: string
}

export type TicketStatus = 'open' | 'in_progress' | 'waiting' | 'resolved' | 'closed'
export type TicketPriority = 'low' | 'normal' | 'high' | 'urgent'
export type TicketSource = 'portal' | 'agent' | 'nats' | 'webhook'

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
}

export interface TicketComment extends BaseRecord {
  ticket: string
  author_staff?: string
  author_user?: string
  body: string
  internal: boolean
}

export interface TimeEntry extends BaseRecord {
  ticket: string
  staff: string
  minutes: number
  work_date: string
  note?: string
}

export type VisitStatus = 'requested' | 'scheduled' | 'completed' | 'canceled'

// A `requested` visit has no assignee/time yet — an agent promoted the
// ticket to on-site work; the dispatcher schedules it later.
export interface Visit extends BaseRecord {
  ticket: string
  assignee?: string
  scheduled_at?: string
  status: VisitStatus
  location?: string
  notes?: string
}

export const TICKET_STATUSES: TicketStatus[] = ['open', 'in_progress', 'waiting', 'resolved', 'closed']
export const TICKET_PRIORITIES: TicketPriority[] = ['low', 'normal', 'high', 'urgent']
export const VISIT_STATUSES: VisitStatus[] = ['requested', 'scheduled', 'completed', 'canceled']

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
  payload_summary?: string
}
