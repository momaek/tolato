import { createRouter, createWebHistory } from "vue-router"
import AppShellLayout from "@/app/layouts/AppShellLayout.vue"
import { hasAuthToken } from "@/shared/api/auth-token"
import AuditsPage from "@/pages/AuditsPage.vue"
import ConsoleAgentPage from "@/pages/ConsoleAgentPage.vue"
import DirectShellPage from "@/pages/DirectShellPage.vue"
import LoginPage from "@/pages/LoginPage.vue"
import NodesPage from "@/pages/NodesPage.vue"
import SettingsPage from "@/pages/SettingsPage.vue"
import TaskDetailPage from "@/pages/TaskDetailPage.vue"
import TasksPage from "@/pages/TasksPage.vue"

export const router = createRouter({
  history: createWebHistory(),
  routes: [
    {
      path: "/login",
      component: LoginPage,
    },
    {
      path: "/",
      component: AppShellLayout,
      redirect: "/console/agent",
      children: [
        {
          path: "console/agent",
          component: ConsoleAgentPage,
        },
        {
          path: "console/direct-shell",
          component: DirectShellPage,
        },
        {
          path: "nodes",
          component: NodesPage,
        },
        {
          path: "tasks",
          component: TasksPage,
        },
        {
          path: "tasks/:taskId",
          component: TaskDetailPage,
        },
        {
          path: "audits",
          component: AuditsPage,
        },
        {
          path: "settings",
          component: SettingsPage,
        },
      ],
    },
  ],
})

router.beforeEach((to) => {
  const isMock = import.meta.env.VITE_USE_MOCK === "true"
  if (isMock) {
    return true
  }

  const authenticated = hasAuthToken()
  if (!authenticated && to.path !== "/login") {
    return "/login"
  }

  if (authenticated && to.path === "/login") {
    return "/console/agent"
  }

  return true
})
