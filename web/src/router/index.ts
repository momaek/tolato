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
      // Consent screen for `tolato auth login`. Deliberately outside AppLayout:
      // it is a single decision the user arrived at from a CLI, not a place to
      // navigate around from.
      path: '/cli-auth',
      name: 'cli-auth',
      component: () => import('@/views/CliAuthView.vue'),
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
          path: 'users',
          name: 'users',
          component: () => import('@/views/UsersView.vue'),
          meta: { requiresAdmin: true },
        },
        {
          path: 'permissions',
          name: 'permissions',
          component: () => import('@/views/PermissionsView.vue'),
          meta: { requiresAdmin: true },
        },
        {
          path: 'settings',
          name: 'settings',
          component: () => import('@/views/SettingsView.vue'),
        },
      ],
    },
  ],
})

router.beforeEach(async (to) => {
  const { useAppStore } = await import('@/stores/app')
  const appStore = useAppStore()
  if (to.path !== '/login' && !appStore.isAuthenticated) {
    // Carry the destination through the login so a deep link survives it. This
    // matters for /cli-auth, which is meaningless without its query string:
    // dropping it would send the CLI's request into the void and leave the user
    // staring at a chat window wondering what happened.
    return { path: '/login', query: { redirect: to.fullPath } }
  }
  if (to.path === '/login' && appStore.isAuthenticated) {
    return '/'
  }
  // Cosmetic guard — it keeps a member from landing on a page whose every
  // request would 403. The server is what actually enforces the role.
  if (to.meta.requiresAdmin && !appStore.isAdmin) {
    return '/chat'
  }
})

export default router
