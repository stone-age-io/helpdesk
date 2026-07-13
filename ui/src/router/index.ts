import { createRouter, createWebHistory } from 'vue-router'
import { useAuthStore } from '@/stores/auth'

const router = createRouter({
  history: createWebHistory(),
  routes: [
    { path: '/login', name: 'login', component: () => import('@/views/LoginView.vue') },
    { path: '/forgot-password', name: 'forgot-password', component: () => import('@/views/ForgotPasswordView.vue') },
    { path: '/reset-password', name: 'reset-password', component: () => import('@/views/ResetPasswordView.vue') },

    // Staff app
    {
      path: '/staff',
      component: () => import('@/components/StaffLayout.vue'),
      meta: { requires: 'staff' },
      children: [
        { path: '', redirect: '/staff/dashboard' },
        { path: 'dashboard', name: 'dashboard', component: () => import('@/views/staff/DashboardView.vue') },
        { path: 'tickets', name: 'tickets', component: () => import('@/views/staff/TicketQueueView.vue') },
        { path: 'tickets/new', name: 'ticket-new', component: () => import('@/views/staff/TicketFormView.vue') },
        { path: 'tickets/:id', name: 'ticket-detail', component: () => import('@/views/staff/TicketDetailView.vue') },
        { path: 'dispatch', name: 'dispatch', component: () => import('@/views/staff/DispatchView.vue') },
        { path: 'visits/:id/work', name: 'visit-work', component: () => import('@/views/staff/VisitWorkView.vue') },
        { path: 'reports', name: 'reports', component: () => import('@/views/staff/ReportsView.vue') },
      { path: 'customers', name: 'customers', component: () => import('@/views/staff/CustomerListView.vue') },
        { path: 'customers/:id', name: 'customer-detail', component: () => import('@/views/staff/CustomerDetailView.vue') },
        { path: 'requesters', name: 'requesters', component: () => import('@/views/staff/RequesterListView.vue') },
        { path: 'staff', name: 'staff-list', component: () => import('@/views/staff/StaffListView.vue'), meta: { adminOnly: true } },
        { path: 'categories', name: 'categories', component: () => import('@/views/staff/CategoriesView.vue'), meta: { adminOnly: true } },
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
    if (auth.isStaff) return '/staff/dashboard'
    if (auth.isRequester) return '/portal/dashboard'
    return true
  }

  const requires = to.matched.find((r) => r.meta.requires)?.meta.requires
  if (!requires) return true
  if (!auth.isAuthenticated) return { name: 'login' }
  if (requires === 'staff' && !auth.isStaff) return '/portal/dashboard'
  if (requires === 'requester' && !auth.isRequester) return '/staff/dashboard'
  if (to.meta.adminOnly && !auth.isAdmin) return '/staff/dashboard'
  return true
})

export default router
