<script setup lang="ts">
// The field shell's overflow hub.
//
// A phone bottom bar holds five thumb targets and the field app now has eight
// destinations, so one slot has to be a door rather than a place. This is that
// door — deliberately a plain launcher and not a dashboard: it is passed
// through, not read, and anything that took a moment to parse would be in the
// way.
//
// The desktop field sidebar links all of these directly, so this page is only
// ever the phone's answer. It groups by the question being asked rather than by
// collection: "Look up" is the scan-or-search path to a record, "Work" is the
// planning surface that lost its tab to make room.
import { onMounted } from 'vue'
import { useVisitContext } from '@/composables/useVisitContext'

const context = useVisitContext()

const groups = [
  {
    title: 'Look up',
    hint: 'Find the site or device in front of you.',
    items: [
      {
        label: 'Scan',
        icon: '📷',
        path: '/staff/scan',
        blurb: 'Read a label, or type the code printed on it.',
      },
      { label: 'Sites', icon: '📍', path: '/staff/locations', blurb: 'Addresses, access notes, contacts.' },
      { label: 'Devices', icon: '🧰', path: '/staff/things', blurb: 'Equipment and its ticket history.' },
    ],
  },
  {
    title: 'Work',
    hint: '',
    items: [{ label: 'Projects', icon: '📁', path: '/staff/projects', blurb: 'Rollouts you are on the crew for.' }],
  },
]

onMounted(context.load)
</script>

<template>
  <div class="space-y-6 max-w-2xl mx-auto">
    <h1 class="text-2xl font-bold">More</h1>

    <section v-for="group in groups" :key="group.title" class="space-y-2">
      <div>
        <h2 class="font-semibold">{{ group.title }}</h2>
        <p v-if="group.hint" class="text-sm text-base-content/60">{{ group.hint }}</p>
      </div>

      <ul class="space-y-2">
        <li v-for="item in group.items" :key="item.path">
          <!-- Card-sized rather than a menu row: these are tapped with a thumb,
               often with one hand and a ladder in the other. -->
          <RouterLink
            :to="item.path"
            class="card bg-base-100 shadow-sm hover:bg-base-200 transition-colors"
          >
            <div class="card-body p-3 flex-row items-center gap-3">
              <span class="text-2xl leading-none" aria-hidden="true">{{ item.icon }}</span>
              <span class="flex-1 min-w-0">
                <span class="block font-medium">{{ item.label }}</span>
                <span class="block text-xs text-base-content/60 truncate">{{ item.blurb }}</span>
              </span>
              <span class="text-base-content/30" aria-hidden="true">›</span>
            </div>
          </RouterLink>
        </li>
      </ul>
    </section>

    <!--
      Only shown when this tech actually has scheduled visits, which is the same
      self-hiding rule the roster context filters use: an admin with none should
      never be told about a control that would empty their list.
    -->
    <p v-if="context.has.value" class="text-xs text-base-content/50">
      Sites and Devices can narrow to the {{ context.customerIds.value.size }}
      {{ context.customerIds.value.size === 1 ? 'customer' : 'customers' }} you are scheduled at.
    </p>
  </div>
</template>
