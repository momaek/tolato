import { createRouter, createWebHistory } from 'vue-router'

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
    },
    {
      path: '/console/:sessionId',
      name: 'console-session',
      component: () => import('@/pages/console/ConsolePage.vue'),
      props: true,
    },
    {
      path: '/nodes',
      name: 'nodes',
      component: () => import('@/pages/nodes/NodesPage.vue'),
    },
    {
      path: '/nodes/:id',
      name: 'node-detail',
      component: () => import('@/pages/node-detail/NodeDetailPage.vue'),
      props: true,
    },
    {
      path: '/history',
      name: 'history',
      component: () => import('@/pages/history/HistoryPage.vue'),
    },
    {
      path: '/history/:taskId',
      name: 'history-detail',
      component: () => import('@/pages/history/HistoryPage.vue'),
      props: true,
    },
    {
      path: '/settings',
      name: 'settings',
      component: () => import('@/pages/settings/SettingsPage.vue'),
    },
  ],
})

export default router
