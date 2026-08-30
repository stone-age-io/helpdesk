// Due-date buckets, defined once and spent by both the queue's `due` filter and
// the dashboard's Due tiles.
//
// The backlog-age buckets next door are written twice — once in
// TicketQueueView's ageClause and once in DashboardView's counting block — kept
// in sync by a comment in each file saying they must agree. That is a standing
// drift risk and there was no reason to repeat it here: a tile whose number
// disagrees with the queue it opens is worse than no tile.
//
// Both consumers must evaluate these PER QUERY, never once at setup. A board
// left open overnight would otherwise keep measuring "overdue" against whenever
// the tab happened to load.

import { format } from 'date-fns'

/**
 * Format a stored calendar date for display.
 *
 * `format(new Date(due_at), …)` is the obvious thing and it is wrong here for
 * the same reason the boundaries were: the value is UTC midnight of a named
 * day, so a browser west of Greenwich renders it as the day before. Parse the
 * date half into a LOCAL date instead and the day survives the round trip.
 * Returns '' for an empty value, so callers can render nothing rather than
 * "Invalid Date".
 */
export function formatDay(value?: string, pattern = 'MMM d'): string {
  if (!value) return ''
  const [y, m, d] = value.slice(0, 10).split('-').map(Number)
  if (!y || !m || !d) return ''
  return format(new Date(y, m - 1, d), pattern)
}

export interface DueBucket {
  value: string
  label: string
  /** Short form for the dashboard tiles, where the column is narrow. */
  tileLabel: string
}

export const DUE_BUCKETS: DueBucket[] = [
  { value: 'overdue', label: 'Overdue', tileLabel: 'Overdue' },
  { value: 'today', label: 'Due today', tileLabel: 'Today' },
  { value: '7days', label: 'Due within 7 days', tileLabel: 'Next 7 days' },
]

/**
 * The local calendar day, `offsetDays` from today, as YYYY-MM-DD.
 */
export function dayKey(offsetDays = 0): string {
  const d = new Date()
  d.setDate(d.getDate() + offsetDays)
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`
}

/**
 * A PocketBase datetime boundary for one end of that day.
 *
 * Deliberately NOT DispatchView's `pbTime`, and the difference is the whole
 * reason this function has a comment. `pbTime` converts a local midnight to
 * UTC, which is right for `visits.scheduled_at` — an instant, a moment a
 * technician is somewhere. `due_at` is a calendar DATE: both writers (the
 * generator's Format("2006-01-02") and the type="date" input) store it as
 * UTC midnight of the named day. Passing that through a local→UTC conversion
 * shifts the boundary by the offset, so west of Greenwich everything due today
 * compares as already overdue. Build the boundary in UTC directly instead —
 * the day is chosen locally, then read as the date it plainly is.
 */
function boundary(offsetDays: number, endOfDay: boolean): string {
  return `${dayKey(offsetDays)} ${endOfDay ? '23:59:59.999Z' : '00:00:00.000Z'}`
}

/**
 * Whether a stored due date has passed, by calendar day. Compares the date
 * halves as strings — ISO dates sort correctly and it avoids re-introducing
 * the timezone shift described above.
 */
export function isPastDue(dueAt?: string): boolean {
  if (!dueAt) return false
  return dueAt.slice(0, 10) < dayKey()
}

/**
 * The PocketBase filter clause for one bucket, or '' for an unknown value.
 *
 * Every clause requires `due_at != ''` — most tickets have no due date, and
 * without that term an empty date sorts below every boundary and would make the
 * whole undated backlog read as overdue.
 */
export function dueClause(bucket: string): string {
  const dated = `due_at != ''`
  switch (bucket) {
    case 'overdue':
      return `${dated} && due_at < '${boundary(0, false)}'`
    case 'today':
      return `${dated} && due_at >= '${boundary(0, false)}' && due_at <= '${boundary(0, true)}'`
    case '7days':
      return `${dated} && due_at >= '${boundary(0, false)}' && due_at <= '${boundary(7, true)}'`
    default:
      return ''
  }
}

/**
 * The clause the dashboard counts with: a bucket, scoped to active tickets.
 * Resolved and closed work is not "overdue" — it is done, however late it was.
 */
export function activeDueFilter(bucket: string): string {
  return `status != 'resolved' && status != 'closed' && (${dueClause(bucket)})`
}
