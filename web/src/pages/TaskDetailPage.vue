<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from "vue"
import { storeToRefs } from "pinia"
import { useRoute } from "vue-router"
import RiskBadge from "@/features/tasks/RiskBadge.vue"
import TaskStatusBadge from "@/features/tasks/TaskStatusBadge.vue"
import { useAuditsStore } from "@/entities/audits/store"
import { useTasksStore } from "@/entities/tasks/store"
import { controlApi } from "@/shared/api/control-api"
import { Card, CardContent, CardHeader, CardTitle, ScrollArea, Table, TableBody, TableCell, TableHead, TableHeader, TableRow, Tabs, TabsContent, TabsList, TabsTrigger } from "@/shared/ui"
import { formatDateTime } from "@/shared/lib/format"

const route = useRoute()
const tasksStore = useTasksStore()
const auditsStore = useAuditsStore()
const isLoading = ref(false)
const loadError = ref("")
let pollTimer: number | null = null

const { byId } = storeToRefs(tasksStore)
const taskId = computed(() => String(route.params.taskId))
const task = computed(() => byId.value[taskId.value] ?? null)

const taskAudits = computed(() => {
  if (!task.value) {
    return []
  }

  return auditsStore.items.filter((item) => item.taskId === task.value?.id)
})

async function ensureTaskLoaded() {
  if (!taskId.value) {
    return
  }

  const shouldShowLoader = !task.value
  if (shouldShowLoader) {
    isLoading.value = true
  }
  loadError.value = ""

  try {
    const [taskDetail, audits] = await Promise.all([
      controlApi.getTask(taskId.value),
      controlApi.getAudits(taskId.value),
    ])

    if (taskDetail) {
      tasksStore.upsertTask(taskDetail)
    }

    if (audits.length > 0) {
      auditsStore.setAudits([
        ...auditsStore.items.filter((item) => item.taskId !== taskId.value),
        ...audits,
      ])
    }
  } catch (error) {
    loadError.value = error instanceof Error ? error.message : "Failed to load task"
  } finally {
    if (shouldShowLoader) {
      isLoading.value = false
    }
  }
}

onMounted(() => {
  void ensureTaskLoaded()
  pollTimer = window.setInterval(() => {
    void ensureTaskLoaded()
  }, 5000)
})

watch(taskId, () => {
  void ensureTaskLoaded()
})

onBeforeUnmount(() => {
  if (pollTimer !== null) {
    window.clearInterval(pollTimer)
  }
})
</script>

<template>
  <Card v-if="isLoading" class="border-none shadow-sm">
    <CardHeader>
      <CardTitle>Loading task...</CardTitle>
    </CardHeader>
  </Card>

  <Card v-else-if="loadError" class="border-none shadow-sm">
    <CardHeader>
      <CardTitle>Failed to load task</CardTitle>
    </CardHeader>
    <CardContent class="text-sm text-destructive">
      {{ loadError }}
    </CardContent>
  </Card>

  <div v-else-if="task" class="space-y-4">
    <div class="flex flex-wrap items-start justify-between gap-4">
      <div>
        <p class="text-sm font-semibold uppercase tracking-[0.18em] text-muted-foreground">
          Task Detail
        </p>
        <h1 class="mt-2 text-3xl font-semibold tracking-tight">{{ task.inputText }}</h1>
      </div>
      <div class="flex items-center gap-2">
        <RiskBadge :level="task.plan.riskLevel" />
        <TaskStatusBadge :status="task.status" />
      </div>
    </div>

    <Card class="border-none shadow-sm">
      <CardContent class="grid gap-4 pt-6 md:grid-cols-3">
        <div>
          <p class="text-sm font-medium text-foreground">Task ID</p>
          <p class="text-sm text-muted-foreground">{{ task.id }}</p>
        </div>
        <div>
          <p class="text-sm font-medium text-foreground">Target</p>
          <p class="text-sm text-muted-foreground">{{ task.target.join(", ") }}</p>
        </div>
        <div>
          <p class="text-sm font-medium text-foreground">Created</p>
          <p class="text-sm text-muted-foreground">{{ formatDateTime(task.createdAt) }}</p>
        </div>
      </CardContent>
    </Card>

    <Tabs default-value="plan">
      <TabsList class="rounded-2xl">
        <TabsTrigger value="plan">Plan</TabsTrigger>
        <TabsTrigger value="executions">Executions</TabsTrigger>
        <TabsTrigger value="audit">Audit</TabsTrigger>
      </TabsList>

      <TabsContent value="plan">
        <Card class="border-none shadow-sm">
          <CardHeader>
            <CardTitle>Structured plan</CardTitle>
          </CardHeader>
          <CardContent class="space-y-3">
            <div
              v-for="step in task.plan.steps"
              :key="step.id"
              class="rounded-2xl border border-border p-4"
            >
              <div class="flex items-center justify-between gap-3">
                <p class="font-medium text-foreground">{{ step.action }}</p>
                <RiskBadge :level="step.risk" />
              </div>
              <p class="mt-2 text-sm text-muted-foreground">
                {{ Object.entries(step.args).map(([key, value]) => `${key}=${value}`).join(" · ") }}
              </p>
            </div>
          </CardContent>
        </Card>
      </TabsContent>

      <TabsContent value="executions">
        <Card class="border-none shadow-sm">
          <CardHeader>
            <CardTitle>Execution details</CardTitle>
          </CardHeader>
          <CardContent>
            <ScrollArea class="h-80 rounded-2xl border border-border">
              <div class="space-y-3 p-4">
                <div
                  v-for="execution in task.executions"
                  :key="execution.id"
                  class="rounded-2xl border border-border bg-muted/40 p-4"
                >
                  <div class="flex items-center justify-between gap-3">
                    <p class="font-medium text-foreground">{{ execution.nodeId }}</p>
                    <TaskStatusBadge :status="execution.status" />
                  </div>
                  <p class="mt-3 whitespace-pre-wrap font-mono text-xs text-muted-foreground">
                    {{ execution.stdoutTail || execution.stderrTail || "No output yet." }}
                  </p>
                </div>
              </div>
            </ScrollArea>
          </CardContent>
        </Card>
      </TabsContent>

      <TabsContent value="audit">
        <Card class="border-none shadow-sm">
          <CardHeader>
            <CardTitle>Audit timeline</CardTitle>
          </CardHeader>
          <CardContent>
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>Action</TableHead>
                  <TableHead>Actor</TableHead>
                  <TableHead>Description</TableHead>
                  <TableHead>Time</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                <TableRow v-for="audit in taskAudits" :key="audit.id">
                  <TableCell class="font-medium">{{ audit.action }}</TableCell>
                  <TableCell>{{ audit.actorId }}</TableCell>
                  <TableCell>{{ audit.description }}</TableCell>
                  <TableCell>{{ formatDateTime(audit.createdAt) }}</TableCell>
                </TableRow>
              </TableBody>
            </Table>
          </CardContent>
        </Card>
      </TabsContent>
    </Tabs>
  </div>

  <Card v-else class="border-none shadow-sm">
    <CardHeader>
      <CardTitle>Task not found</CardTitle>
    </CardHeader>
  </Card>
</template>
