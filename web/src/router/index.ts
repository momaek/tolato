import { createRouter, createWebHistory } from 'vue-router'

const router = createRouter({
  history: createWebHistory(),
  routes: [
    {
      path: '/login',
      name: 'login',
      component: () => import('@/views/LoginView.vue'),
    },
    {
      path: '/',
      component: () => import('@/components/layout/AppLayout.vue'),
      children: [
        { path: '', redirect: '/chat' },
        {
          path: 'chat',
          name: 'chat',
          component: () => import('@/views/ChatView.vue'),
        },
        {
          path: 'chat/:conversationId',
          name: 'chat-conversation',
          component: () => import('@/views/ChatView.vue'),
        },
        {
          path: 'nodes',
          name: 'nodes',
          component: () => import('@/views/NodesView.vue'),
        },
        {
          path: 'nodes/:nodeId',
          name: 'node-detail',
          component: () => import('@/views/NodeDetailView.vue'),
        },
        {
          path: 'nodes/:nodeId/terminal',
          name: 'node-terminal',
          component: () => import('@/views/NodeTerminalView.vue'),
        },
        {
          path: 'audit',
          name: 'audit',
          component: () => import('@/views/AuditLogView.vue'),
        },
        {
          path: 'settings',
          name: 'settings',
          component: () => import('@/views/SettingsView.vue'),
        },
        {
          path: 'monitor',
          name: 'monitor',
          component: () => import('@/views/MonitorView.vue'),
        },
        {
          path: 'monitor/:linkId',
          name: 'link-detail',
          component: () => import('@/views/LinkDetailView.vue'),
        },
        {
          path: 'alerts',
          name: 'alerts',
          component: () => import('@/views/AlertsView.vue'),
        },
      ],
    },
  ],
})

router.beforeEach(async (to) => {
  const { useAppStore } = await import('@/stores/app')
  const appStore = useAppStore()
  if (to.path !== '/login' && !appStore.isAuthenticated) {
    return '/login'
  }
  if (to.path === '/login' && appStore.isAuthenticated) {
    return '/'
  }
})

export default router
