<script setup lang="ts">
import { computed, ref } from "vue"
import { storeToRefs } from "pinia"
import ConsoleSidebar from "@/widgets/console/ConsoleSidebar.vue"
import RiskBadge from "@/features/tasks/RiskBadge.vue"
import TaskStatusBadge from "@/features/tasks/TaskStatusBadge.vue"
import { useConsoleStore } from "@/entities/console/store"
import { useNodesStore } from "@/entities/nodes/store"
import { useTasksStore } from "@/entities/tasks/store"
import { controlApi } from "@/shared/api/control-api"
import {
  Button,
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
  ScrollArea,
  Separator,
  Textarea,
} from "@/shared/ui"
import { formatDateTime } from "@/shared/lib/format"

const consoleStore = useConsoleStore()
const nodesStore = useNodesStore()
const tasksStore = useTasksStore()

const { composerText, targetNodeId } = storeToRefs(consoleStore)
const { activeTask } = storeToRefs(tasksStore)

const isPlanDialogOpen = ref(false)
const isSubmitting = ref(false)
const actionError = ref("")

const targetLabel = computed(() => {
  if (targetNodeId.value === "all") {
    return "All nodes (broadcast)"
  }

  return nodesStore.byId[targetNodeId.value]?.hostname ?? targetNodeId.value
})

const quickActions = [
  "磁盘告警",
  "Docker 状态",
  "Nginx 自愈",
  "系统负载",
  "网络检查",
]

const usesApproval = computed(() => activeTask.value?.plan.requiresApproval ?? false)

const requestTarget = computed(() => {
  return targetNodeId.value === "all" ? ["all"] : [targetNodeId.value]
})

async function runTaskMutation(action: "plan" | "approve" | "reject" | "cancel") {
  isSubmitting.value = true
  actionError.value = ""

  try {
    let task = null

    if (action === "plan") {
      task = await controlApi.planTask({
        mode: consoleStore.mode,
        target: requestTarget.value,
        inputText: composerText.value,
      })
    }

    if (action === "approve" && activeTask.value) {
      task = await controlApi.approveTask(activeTask.value.id)
    }

    if (action === "reject" && activeTask.value) {
      task = await controlApi.rejectTask(activeTask.value.id)
    }

    if (action === "cancel" && activeTask.value) {
      task = await controlApi.cancelTask(activeTask.value.id)
    }

    if (task) {
      tasksStore.upsertTask(task)
    }
  } catch (error) {
    actionError.value = error instanceof Error ? error.message : "Unexpected request error"
  } finally {
    isSubmitting.value = false
  }
}
</script>

<template>
  <div class="grid gap-4 xl:grid-cols-[320px,1fr]">
    <ConsoleSidebar />

    <div class="flex min-h-[720px] flex-col gap-4">
      <Card class="border-none shadow-sm">
        <CardContent class="flex flex-wrap items-center justify-between gap-4 pt-6">
          <div class="space-y-1">
            <p class="text-sm text-muted-foreground">Target</p>
            <h2 class="text-2xl font-semibold text-foreground">{{ targetLabel }}</h2>
          </div>
          <div class="flex items-center gap-3">
            <TaskStatusBadge v-if="activeTask" :status="activeTask.status" />
            <Button size="sm" variant="secondary">AI Agent</Button>
            <Button size="sm" variant="outline" disabled>Direct shell</Button>
          </div>
        </CardContent>
      </Card>

      <Card class="border-none shadow-sm">
        <CardHeader>
          <p class="text-xs font-semibold uppercase tracking-[0.18em] text-muted-foreground">
            System
          </p>
          <CardTitle>Control server ready</CardTitle>
          <CardDescription>
            当前目标 {{ targetLabel }}。广播模式只自动执行低风险只读计划。
          </CardDescription>
        </CardHeader>
      </Card>

      <Card v-if="activeTask" class="border-none shadow-sm">
        <CardHeader class="space-y-4">
          <div class="flex items-center justify-between gap-3">
            <div>
              <p class="text-xs font-semibold uppercase tracking-[0.18em] text-muted-foreground">
                Plan
              </p>
              <CardTitle>Plan Preview</CardTitle>
            </div>
            <div class="flex items-center gap-2">
              <RiskBadge :level="activeTask.plan.riskLevel" />
              <TaskStatusBadge :status="activeTask.status" />
            </div>
          </div>
          <CardDescription>{{ activeTask.plan.summary }}</CardDescription>
        </CardHeader>
        <CardContent class="space-y-4">
          <div class="rounded-2xl border border-border bg-muted/50 p-4">
            <p class="text-sm font-medium text-foreground">Input</p>
            <p class="mt-1 text-sm text-muted-foreground">{{ activeTask.inputText }}</p>
          </div>

          <div class="space-y-3">
            <div
              v-for="step in activeTask.plan.steps"
              :key="step.id"
              class="flex items-center justify-between gap-4 rounded-2xl border border-border px-4 py-3"
            >
              <div>
                <p class="font-medium text-foreground">{{ step.action }}</p>
                <p class="text-sm text-muted-foreground">
                  {{ Object.entries(step.args).map(([key, value]) => `${key}=${value}`).join(" · ") }}
                </p>
              </div>
              <RiskBadge :level="step.risk" />
            </div>
          </div>

          <div class="flex items-center justify-between gap-4">
            <p class="text-sm text-muted-foreground">
              {{ activeTask.plan.estimatedImpact }}
            </p>
            <Button variant="outline" @click="isPlanDialogOpen = true">
              查看完整计划
            </Button>
          </div>
        </CardContent>
      </Card>

      <Card v-if="activeTask && usesApproval" class="border-none shadow-sm">
        <CardHeader>
          <CardTitle>Approval Gate</CardTitle>
          <CardDescription>
            该计划包含写操作，必须审批后才能进入执行队列。
          </CardDescription>
        </CardHeader>
        <CardContent class="flex flex-wrap gap-3">
          <Button :disabled="isSubmitting" @click="runTaskMutation('approve')">Approve</Button>
          <Button :disabled="isSubmitting" variant="outline" @click="runTaskMutation('reject')">Reject</Button>
          <Button :disabled="isSubmitting" variant="ghost" @click="runTaskMutation('cancel')">Cancel</Button>
        </CardContent>
      </Card>

      <Card v-if="activeTask" class="border-none shadow-sm">
        <CardHeader>
          <div class="flex items-center justify-between gap-4">
            <div>
              <CardTitle>Execution Stream</CardTitle>
              <CardDescription>
                状态会通过 WebSocket 持续刷新。最后同步 {{ formatDateTime(activeTask.createdAt) }}
              </CardDescription>
            </div>
            <TaskStatusBadge :status="activeTask.status" />
          </div>
        </CardHeader>
        <CardContent>
          <ScrollArea class="h-72 rounded-2xl border border-border">
            <div class="space-y-3 p-4">
              <div
                v-for="execution in activeTask.executions"
                :key="execution.id"
                class="rounded-2xl border border-border bg-muted/40 p-4"
              >
                <div class="flex items-start justify-between gap-4">
                  <div>
                    <p class="font-medium text-foreground">{{ execution.nodeId }}</p>
                    <p class="text-sm text-muted-foreground">{{ execution.streamSummary }}</p>
                  </div>
                  <TaskStatusBadge :status="execution.status" />
                </div>

                <Separator class="my-3" />

                <p class="whitespace-pre-wrap font-mono text-xs text-muted-foreground">
                  {{ execution.stdoutTail || execution.stderrTail || "Waiting for stream output..." }}
                </p>
              </div>
            </div>
          </ScrollArea>
        </CardContent>
      </Card>

      <div v-if="activeTask" class="grid gap-4 lg:grid-cols-[280px,1fr]">
        <Card class="border-none shadow-sm">
          <CardHeader>
            <CardTitle>Aggregate</CardTitle>
          </CardHeader>
          <CardContent class="space-y-2 text-sm text-muted-foreground">
            <p>Total · {{ activeTask.aggregate.total }}</p>
            <p>Success · {{ activeTask.aggregate.success }}</p>
            <p>Running · {{ activeTask.aggregate.running }}</p>
            <p>Offline skipped · {{ activeTask.aggregate.offlineSkipped }}</p>
          </CardContent>
        </Card>

        <Card class="border-none shadow-sm">
          <CardHeader>
            <CardTitle>AI Summary</CardTitle>
          </CardHeader>
          <CardContent>
            <p class="text-sm leading-7 text-muted-foreground">
              {{ activeTask.summary }}
            </p>
          </CardContent>
        </Card>
      </div>

      <Card class="border-none shadow-sm">
        <CardHeader>
          <CardTitle>Composer</CardTitle>
          <CardDescription>
            描述你的任务，系统会先生成计划。高风险操作不会直接执行。
          </CardDescription>
        </CardHeader>
        <CardContent class="space-y-4">
          <p v-if="actionError" class="text-sm text-destructive">
            {{ actionError }}
          </p>
          <div class="flex gap-3">
            <Textarea
              class="min-h-28 rounded-2xl"
              :model-value="composerText"
              @update:model-value="consoleStore.setComposerText(String($event))"
            />
            <Button
              class="h-auto min-w-36 rounded-2xl"
              :disabled="isSubmitting"
              @click="runTaskMutation('plan')"
            >
              {{ isSubmitting ? "处理中" : "生成计划" }}
            </Button>
          </div>
          <div class="flex flex-wrap gap-2">
            <Button
              v-for="chip in quickActions"
              :key="chip"
              size="sm"
              variant="outline"
              @click="consoleStore.setComposerText(chip)"
            >
              {{ chip }}
            </Button>
          </div>
        </CardContent>
      </Card>
    </div>
  </div>

  <Dialog v-model:open="isPlanDialogOpen">
    <DialogContent class="max-w-3xl rounded-3xl">
      <DialogHeader>
        <DialogTitle>Full Plan</DialogTitle>
        <DialogDescription>
          审批和执行前的完整结构化计划。
        </DialogDescription>
      </DialogHeader>

      <div v-if="activeTask" class="space-y-4">
        <div class="grid gap-3 md:grid-cols-2">
          <div class="rounded-2xl border border-border p-4">
            <p class="text-sm font-medium text-foreground">Task ID</p>
            <p class="mt-1 text-sm text-muted-foreground">{{ activeTask.id }}</p>
          </div>
          <div class="rounded-2xl border border-border p-4">
            <p class="text-sm font-medium text-foreground">Target</p>
            <p class="mt-1 text-sm text-muted-foreground">
              {{ activeTask.plan.targetNodes.join(", ") }}
            </p>
          </div>
        </div>

        <div class="space-y-3">
          <div
            v-for="step in activeTask.plan.steps"
            :key="step.id"
            class="rounded-2xl border border-border p-4"
          >
            <div class="flex items-center justify-between gap-4">
              <p class="font-medium text-foreground">{{ step.action }}</p>
              <RiskBadge :level="step.risk" />
            </div>
            <p class="mt-2 text-sm text-muted-foreground">
              {{ JSON.stringify(step.args, null, 2) }}
            </p>
          </div>
        </div>
      </div>
    </DialogContent>
  </Dialog>
</template>
