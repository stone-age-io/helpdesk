import { createRouter, createWebHistory } from 'vue-router'
import { useAuthStore } from '@/stores/auth'

const router = createRouter({
  history: createWebHistory(),
  routes: [
    { path: '/login', name: 'login', component: () => import('@/views/LoginView.vue') },
    { path: '/forgot-password', name: 'forgot-password', component: () => import('@/views/ForgotPasswordView.vue') },
    { path: '/reset-password', name: 'reset-password', component: () => import('@/views/ResetPasswordView.vue') },

    // Staff app. One route tree; the shell (desk sidebar vs. field bottom-tabs)
    // is chosen by role inside StaffShell, so field agents browse the same
    // /staff/* URLs and every hardcoded desk link keeps working.
    {
      path: '/staff',
      component: () => import('@/components/StaffShell.vue'),
      meta: { requires: 'staff' },
      children: [
        { path: '', redirect: () => (useAuthStore().isField ? '/staff/today' : '/staff/dashboard') },
        { path: 'dashboard', name: 'dashboard', component: () => import('@/views/staff/DashboardView.vue') },
        // Field-agent surfaces (mobile, on-site). Reachable by any staff, but
        // only the field shell links to them.
        { path: 'today', name: 'field-today', component: () => import('@/views/staff/FieldTodayView.vue') },
        { path: 'schedule', name: 'field-schedule', component: () => import('@/views/staff/FieldScheduleView.vue') },
        { path: 'my-time', name: 'field-time', component: () => import('@/views/staff/FieldTimeLogView.vue') },
        { path: 'tickets', name: 'tickets', component: () => import('@/views/staff/TicketQueueView.vue') },
        { path: 'tickets/new', name: 'ticket-new', component: () => import('@/views/staff/TicketFormView.vue') },
        { path: 'tickets/:id', name: 'ticket-detail', component: () => import('@/views/staff/TicketDetailView.vue') },
        { path: 'dispatch', name: 'dispatch', component: () => import('@/views/staff/DispatchView.vue') },
        { path: 'projects', name: 'projects', component: () => import('@/views/staff/ProjectsView.vue') },
        { path: 'projects/new', name: 'project-new', component: () => import('@/views/staff/ProjectDetailView.vue') },
        { path: 'projects/:id', name: 'project-detail', component: () => import('@/views/staff/ProjectDetailView.vue') },
        { path: 'visits/:id/work', name: 'visit-work', component: () => import('@/views/staff/VisitWorkView.vue') },
        { path: 'reports', name: 'reports', component: () => import('@/views/staff/ReportsView.vue') },
      { path: 'customers', name: 'customers', component: () => import('@/views/staff/CustomerListView.vue') },
        { path: 'customers/:id', name: 'customer-detail', component: () => import('@/views/staff/CustomerDetailView.vue') },
        { path: 'requesters', name: 'requesters', component: () => import('@/views/staff/RequesterListView.vue') },
        { path: 'requesters/:id', name: 'requester-detail', component: () => import('@/views/staff/RequesterDetailView.vue') },
        { path: 'staff', name: 'staff-list', component: () => import('@/views/staff/StaffListView.vue'), meta: { adminOnly: true } },
        { path: 'staff/:id', name: 'staff-detail', component: () => import('@/views/staff/StaffDetailView.vue'), meta: { adminOnly: true } },
        { path: 'categories', name: 'categories', component: () => import('@/views/staff/CategoriesView.vue'), meta: { adminOnly: true } },
        { path: 'locations', name: 'locations', component: () => import('@/views/staff/LocationsView.vue') },
        { path: 'locations/new', name: 'location-new', component: () => import('@/views/staff/LocationDetailView.vue') },
        { path: 'locations/:id', name: 'location-detail', component: () => import('@/views/staff/LocationDetailView.vue') },
        { path: 'notifications', name: 'notifications', component: () => import('@/views/staff/NotificationTemplatesView.vue'), meta: { adminOnly: true } },
      ],
    },

    // Requester portal
    {
      path: '/portal',
      component: () => import('@/components/PortalLayout.vue'),
      meta: { requires: 'requester' },
      children: [
        { path: '', redirect: '/portal/dashboard' },
        { path: 'dashboard', name: 'portal-dashboard', component: () => import('@/views/portal/PortalDashboardView.vue') },
        { path: 'tickets', name: 'portal-tickets', component: () => import('@/views/portal/PortalTicketsView.vue') },
        { path: 'tickets/new', name: 'portal-ticket-new', component: () => import('@/views/portal/NewTicketView.vue') },
        { path: 'tickets/:id', name: 'portal-ticket-detail', component: () => import('@/views/portal/PortalTicketDetailView.vue') },
        { path: 'visits', name: 'portal-visits', component: () => import('@/views/portal/PortalVisitsView.vue') },
        { path: 'projects', name: 'portal-projects', component: () => import('@/views/portal/PortalProjectsView.vue') },
        { path: 'projects/:id', name: 'portal-project-detail', component: () => import('@/views/portal/PortalProjectDetailView.vue') },
      ],
    },

    // Role-neutral ticket deep link used by notification emails.
    { path: '/t/:id', name: 'ticket-link', component: () => import('@/views/TicketLinkView.vue') },

    { path: '/', redirect: '/login' },
    { path: '/:pathMatch(.*)*', redirect: '/login' },
  ],
})

router.beforeEach((to) => {
  const auth = useAuthStore()

  if (to.name === 'login') {
    return auth.isAuthenticated ? auth.homePath : true
  }

  const requires = to.matched.find((r) => r.meta.requires)?.meta.requires
  if (!requires) return true
  if (!auth.isAuthenticated) return { name: 'login' }
  // Wrong shell for this identity → bounce to wherever they belong.
  if (requires === 'staff' && !auth.isStaff) return auth.homePath
  if (requires === 'requester' && !auth.isRequester) return auth.homePath
  if (to.meta.adminOnly && !auth.isAdmin) return auth.homePath
  return true
})

export default router
