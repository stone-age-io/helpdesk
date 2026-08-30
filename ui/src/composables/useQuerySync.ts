// Filter state <-> URL query, for the three staff boards that carry filters
// (ticket queue, Reports, Dispatch).
//
// All three already READ filters from the query — that is how a dashboard tile
// or a "View all ->" link arrives pre-filtered — but only Reports and Dispatch
// wrote anything back, and only their `view`. So the filters you set by hand
// existed nowhere but component state: a filtered board could not be linked,
// survive a reload, or come back when you clicked into a ticket and pressed
// back. That last one is the everyday cost — reading the queue means opening
// tickets out of it, and every trip back reset the filters you had just set.
//
// Three decisions worth not re-litigating:
//
//   REPLACE, never push. A filter is an adjustment to the view you are already
//   looking at, not a place you travelled to. Pushing would bury the page you
//   came from under one history entry per keystroke, so Back — which should
//   return you to the dashboard or the ticket — would instead walk you
//   backwards through your own filter edits.
//
//   Defaults are omitted, not written. `?status=active` on a queue that is
//   already showing active tickets is noise, and it makes two identical views
//   produce two different-looking links. What is absent from the query is what
//   the page would have chosen anyway.
//
//   Outbound only. Nothing here watches the route and pushes values back into
//   the refs. That direction reads as the natural completion of this and is a
//   trap: a ref writes the query, the query writes the ref, and the guard that
//   stops the loop has to distinguish "the user navigated" from "we just wrote
//   this", which is state this does not need to hold. Vue Router remounts these
//   views on every arrival that matters (dashboard tile, deep link, Back), and
//   the mount-time read already covers those.
import { watch, type Ref } from 'vue'
import { useRoute, useRouter, type LocationQueryRaw } from 'vue-router'

/**
 * Read a string query param, ignoring the array form Vue Router produces for
 * repeated keys — a filter that arrives twice has no meaningful value.
 */
export function useQueryValue() {
  const route = useRoute()
  return (key: string, fallback = ''): string =>
    typeof route.query[key] === 'string' ? (route.query[key] as string) : fallback
}

/**
 * Mirror `fields` into the URL query whenever any of them changes. A field
 * equal to its entry in `defaults` (or empty) is dropped from the query rather
 * than written. Query keys not named in `fields` are left alone.
 *
 * Only fires on change, so landing on a page never rewrites its own URL.
 */
export function useQuerySync(fields: Record<string, Ref<unknown>>, defaults: Record<string, string> = {}) {
  const route = useRoute()
  const router = useRouter()
  const keys = Object.keys(fields)

  watch(Object.values(fields), () => {
    const query: LocationQueryRaw = { ...route.query }
    for (const key of keys) {
      const value = String(fields[key].value ?? '')
      query[key] = value && value !== (defaults[key] ?? '') ? value : undefined
    }
    // A replace that lands on the query already showing is a no-op navigation;
    // Vue Router reports it as a failure rather than throwing, but swallow it
    // so an unhandled rejection never reaches the console.
    router.replace({ query }).catch(() => {})
  })
}
