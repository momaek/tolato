<script setup lang="ts">
import { storeToRefs } from "pinia"
import { RouterLink } from "vue-router"
import RiskBadge from "@/features/tasks/RiskBadge.vue"
import TaskStatusBadge from "@/features/tasks/TaskStatusBadge.vue"
import { useTasksStore } from "@/entities/tasks/store"
import { Card, CardContent, CardHeader, CardTitle, Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/shared/ui"
import { formatDateTime } from "@/shared/lib/format"

const tasksStore = useTasksStore()
const { items } = storeToRefs(tasksStore)
const isMockMode = import.meta.env.VITE_USE_MOCK !== "false"
</script>

<template>
  <div class="space-y-4">
    <div>
      <p class="text-sm font-semibold uppercase tracking-[0.18em] text-muted-foreground">
        Operations
      </p>
      <h1 class="mt-2 text-3xl font-semibold tracking-tight">Tasks</h1>
    </div>

    <Card class="border-none shadow-sm">
      <CardHeader>
        <CardTitle>Recent tasks</CardTitle>
      </CardHeader>
      <CardContent>
        <div v-if="items.length === 0" class="rounded-2xl border border-dashed border-border p-6 text-sm text-muted-foreground">
          {{ isMockMode ? "No tasks in mock state." : "当前后端还没有 GET /api/v1/tasks，列表页暂时无法直接加载历史任务。可从控制台新建任务，或通过 /tasks/:id 查看单任务详情。" }}
        </div>
        <Table v-else>
          <TableHeader>
            <TableRow>
              <TableHead>Task</TableHead>
              <TableHead>Risk</TableHead>
              <TableHead>Status</TableHead>
              <TableHead>Target</TableHead>
              <TableHead>Created</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            <TableRow v-for="task in items" :key="task.id">
              <TableCell>
                <RouterLink class="font-medium text-foreground underline-offset-4 hover:underline" :to="`/tasks/${task.id}`">
                  {{ task.inputText }}
                </RouterLink>
              </TableCell>
              <TableCell>
                <RiskBadge :level="task.plan.riskLevel" />
              </TableCell>
              <TableCell>
                <TaskStatusBadge :status="task.status" />
              </TableCell>
              <TableCell>{{ task.target.join(", ") }}</TableCell>
              <TableCell>{{ formatDateTime(task.createdAt) }}</TableCell>
            </TableRow>
          </TableBody>
        </Table>
      </CardContent>
    </Card>
  </div>
</template>
