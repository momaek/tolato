<script setup lang="ts">
import { computed, onMounted } from "vue"
import { RouterLink, RouterView, useRoute } from "vue-router"
import { storeToRefs } from "pinia"
import { bootstrapApp } from "@/app/bootstrap"
import TaskStatusBadge from "@/features/tasks/TaskStatusBadge.vue"
import { useConnectionStore } from "@/entities/connection/store"
import { useSessionStore } from "@/entities/session/store"
import { useTasksStore } from "@/entities/tasks/store"
import { Badge } from "@/shared/ui"
import { formatDateTime } from "@/shared/lib/format"

const route = useRoute()
const connectionStore = useConnectionStore()
const sessionStore = useSessionStore()
const tasksStore = useTasksStore()

const { state, lastSyncAt } = storeToRefs(connectionStore)
const { currentUser } = storeToRefs(sessionStore)
const { activeTask } = storeToRefs(tasksStore)

const navItems = [
  { to: "/console/agent", label: "Console" },
  { to: "/nodes", label: "Nodes" },
  { to: "/tasks", label: "Tasks" },
  { to: "/audits", label: "Audits" },
  { to: "/settings", label: "Settings" },
]

const sectionTitle = computed(() => {
  const current = navItems.find((item) => route.path.startsWith(item.to))
  return current?.label ?? "Console"
})

onMounted(() => {
  void bootstrapApp()
})
</script>

<template>
  <div class="min-h-screen bg-background text-foreground">
    <header class="border-b border-border/70 bg-background/90 backdrop-blur">
      <div class="mx-auto flex max-w-[1600px] items-center justify-between gap-6 px-6 py-4">
        <div class="space-y-1">
          <p class="text-xl font-semibold tracking-tight">ToLaTo</p>
          <p class="text-sm text-muted-foreground">
            AI-driven multi-node operations console
          </p>
        </div>

        <div class="flex items-center gap-3">
          <Badge variant="outline" class="rounded-full border-border px-3 py-1">
            {{ state }}
          </Badge>
          <Badge variant="outline" class="rounded-full border-border px-3 py-1">
            Last sync {{ formatDateTime(lastSyncAt) }}
          </Badge>
          <TaskStatusBadge v-if="activeTask" :status="activeTask.status" />
          <div class="rounded-full border border-border px-3 py-1 text-sm text-muted-foreground">
            {{ currentUser?.name ?? "Loading..." }}
          </div>
        </div>
      </div>
    </header>

    <div class="mx-auto flex max-w-[1600px]">
      <aside class="hidden min-h-[calc(100vh-73px)] w-64 shrink-0 border-r border-border/70 bg-card/60 p-4 lg:block">
        <div class="space-y-6">
          <div>
            <p class="text-xs font-semibold uppercase tracking-[0.18em] text-muted-foreground">
              Workspace
            </p>
            <p class="mt-2 text-lg font-semibold">{{ sectionTitle }}</p>
          </div>

          <nav class="space-y-2">
            <RouterLink
              v-for="item in navItems"
              :key="item.to"
              :to="item.to"
              class="flex rounded-2xl px-4 py-3 text-sm font-medium transition"
              :class="
                route.path.startsWith(item.to)
                  ? 'bg-primary text-primary-foreground shadow-sm'
                  : 'text-muted-foreground hover:bg-muted hover:text-foreground'
              "
            >
              {{ item.label }}
            </RouterLink>
          </nav>
        </div>
      </aside>

      <main class="min-h-[calc(100vh-73px)] flex-1 p-6">
        <RouterView />
      </main>
    </div>
  </div>
</template>
