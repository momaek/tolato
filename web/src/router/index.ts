import { createRouter, createWebHistory } from 'vue-router'
import { useAppStore } from '@/stores/app'

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
          path: 'audit',
          name: 'audit',
          component: () => import('@/views/AuditLogView.vue'),
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

router.beforeEach((to) => {
  const appStore = useAppStore()
  if (to.path !== '/login' && !appStore.isAuthenticated) {
    return '/login'
  }
  if (to.path === '/login' && appStore.isAuthenticated) {
    return '/'
  }
})

export default router
