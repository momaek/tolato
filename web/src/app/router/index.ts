import { createRouter, createWebHistory } from 'vue-router'

import { hasAccessToken } from '@/shared/auth/session'
import { appEnv } from '@/shared/config/env'

const router = createRouter({
  history: createWebHistory(),
  routes: [
    {
      path: '/',
      redirect: '/console',
    },
    {
      path: '/console',
      name: 'console',
      component: () => import('@/pages/console/ConsolePage.vue'),
      meta: {
        requiresAuth: true,
      },
    },
    {
      path: '/console/:sessionId',
      name: 'console-session',
      component: () => import('@/pages/console/ConsolePage.vue'),
      props: true,
      meta: {
        requiresAuth: true,
      },
    },
    {
      path: '/nodes',
      name: 'nodes',
      component: () => import('@/pages/nodes/NodesPage.vue'),
      meta: {
        requiresAuth: true,
      },
    },
    {
      path: '/nodes/:id',
      name: 'node-detail',
      component: () => import('@/pages/node-detail/NodeDetailPage.vue'),
      props: true,
      meta: {
        requiresAuth: true,
      },
    },
    {
      path: '/history',
      name: 'history',
      component: () => import('@/pages/history/HistoryPage.vue'),
      meta: {
        requiresAuth: true,
      },
    },
    {
      path: '/history/:taskId',
      name: 'history-detail',
      component: () => import('@/pages/history/HistoryPage.vue'),
      props: true,
      meta: {
        requiresAuth: true,
      },
    },
    {
      path: '/settings',
      name: 'settings',
      component: () => import('@/pages/settings/SettingsPage.vue'),
      meta: {
        requiresAuth: true,
      },
    },
    {
      path: '/login',
      name: 'login',
      component: () => import('@/pages/login/LoginPage.vue'),
    },
  ],
})

router.beforeEach((to) => {
  if (appEnv.useMock) {
    return true
  }

  const authenticated = hasAccessToken()
  if (to.name === 'login') {
    if (authenticated) {
      const next = typeof to.query.next === 'string' && to.query.next.startsWith('/')
        ? to.query.next
        : '/console'
      return next
    }
    return true
  }

  if (to.meta.requiresAuth && !authenticated) {
    return {
      name: 'login',
      query: {
        next: to.fullPath,
      },
    }
  }

  return true
})

export default router
