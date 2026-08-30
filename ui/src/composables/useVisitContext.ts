// The customers the signed-in staff member is scheduled to be at.
//
// Helpdesk `staff` have no customer field — agents are cross-customer by design
// (migration 1800000000) — so there is no ambient tenant anywhere in the staff
// app. That is correct for authorization and unhelpful for a phone: a tech
// standing in a plant room opens Devices and gets every device belonging to
// every customer, alphabetically.
//
// Their scheduled visits are the only honest signal of where they actually are,
// so this returns that set. Two rules govern every use of it, and they are the
// reason this is a composable rather than three inline queries:
//
//   1. Context SORTS or OPTIONALLY narrows. It is never a silent filter. The
//      scanner in particular must resolve globally and disambiguate with a
//      picker (ADR 0002) — filtering there would confidently show a different
//      tenant's DOOR-1.
//   2. An empty set means "no context", not "nothing matches". Every caller
//      hides its context affordance when `has` is false, so a dispatcher or an
//      admin — who has no visits assigned — never sees a control that would
//      filter their roster down to nothing.
import { computed, ref } from 'vue'
import { pb } from '@/pb'
import { useAuthStore } from '@/stores/auth'
import type { Visit } from '@/types'

export function useVisitContext() {
  const auth = useAuthStore()
  const customerIds = ref<Set<string>>(new Set())

  const has = computed(() => customerIds.value.size > 0)
  const includes = (customerId: string) => customerIds.value.has(customerId)

  async function load() {
    const me = auth.record?.id
    if (!me) return
    try {
      const visits = await pb.collection('visits').getFullList<Visit>({
        filter: `assignee = '${me}' && status = 'scheduled'`,
        expand: 'ticket',
        sort: 'scheduled_at',
      })
      const ids = new Set<string>()
      for (const v of visits) {
        const customer = (v.expand?.ticket as { customer?: string } | undefined)?.customer
        if (customer) ids.add(customer)
      }
      customerIds.value = ids
    } catch {
      // No context just means no preferential sort and no context toggle. Every
      // caller still works — this is enhancement, never a precondition.
    }
  }

  return { customerIds, has, includes, load }
}
