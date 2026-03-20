import { defineStore } from "pinia"
import type { TaskDetail, TaskStatus, UiWsEvent } from "@/shared/api/contracts"

export const useTasksStore = defineStore("tasks", {
  state: () => ({
    byId: {} as Record<string, TaskDetail>,
    order: [] as string[],
    activeTaskId: null as string | null,
  }),
  getters: {
    items: (state) => state.order.map((id) => state.byId[id]).filter(Boolean),
    activeTask(state) {
      return state.activeTaskId ? state.byId[state.activeTaskId] ?? null : null
    },
  },
  actions: {
    setTasks(tasks: TaskDetail[]) {
      this.byId = Object.fromEntries(tasks.map((task) => [task.id, task]))
      this.order = tasks.map((task) => task.id)
      this.activeTaskId ??= tasks[0]?.id ?? null
    },
    upsertTask(task: TaskDetail) {
      this.byId[task.id] = task
      if (!this.order.includes(task.id)) {
        this.order.unshift(task.id)
      }
      this.activeTaskId = task.id
    },
    setActiveTask(taskId: string | null) {
      this.activeTaskId = taskId
    },
    updateTaskStatus(taskId: string, status: TaskStatus) {
      const task = this.byId[taskId]

      if (!task) {
        return
      }

      this.byId[taskId] = {
        ...task,
        status,
      }
    },
    upsertExecution(taskId: string, execution: TaskDetail["executions"][number]) {
      const task = this.byId[taskId]
      if (!task) {
        return
      }

      const executions = [...task.executions]
      const index = executions.findIndex((item) => item.id === execution.id)
      if (index === -1) {
        executions.unshift(execution)
      } else {
        executions[index] = {
          ...executions[index],
          ...execution,
        }
      }

      this.byId[taskId] = {
        ...task,
        executions,
        aggregate: computeAggregate(task.target.length || task.plan.targetNodes.length, executions),
      }
    },
    appendTaskLog(taskId: string, executionId: string, nodeId: string, stream: "stdout" | "stderr", chunk: string, timestamp: string) {
      const task = this.byId[taskId]

      if (!task) {
        return
      }

      const executions = [...task.executions]
      const index = executions.findIndex((item) => item.id === executionId)
      const nextChunk = chunk.trim()

      if (index === -1) {
        executions.unshift({
          id: executionId,
          taskId,
          nodeId,
          status: "running",
          startedAt: timestamp,
          exitCode: null,
          stdoutTail: stream === "stdout" ? nextChunk : "",
          stderrTail: stream === "stderr" ? nextChunk : "",
          streamSummary: `live ${stream}`,
        })
      } else {
        const current = executions[index]
        executions[index] = {
          ...current,
          status: "running",
          startedAt: current.startedAt ?? timestamp,
          stdoutTail: stream === "stdout" ? appendTail(current.stdoutTail, nextChunk) : current.stdoutTail,
          stderrTail: stream === "stderr" ? appendTail(current.stderrTail, nextChunk) : current.stderrTail,
          streamSummary: `live ${stream}`,
        }
      }

      this.byId[taskId] = {
        ...task,
        status: "running",
        executions,
        aggregate: computeAggregate(task.target.length || task.plan.targetNodes.length, executions),
      }
    },
    consumeWsEvent(event: UiWsEvent) {
      if (event.type === "task.status") {
        this.updateTaskStatus(event.taskId, event.status)
      }

      if (event.type === "task.log") {
        this.appendTaskLog(event.taskId, event.executionId, event.nodeId, event.stream, event.chunk, event.timestamp)
      }

      if (event.type === "task.result") {
        this.upsertExecution(event.taskId, event.execution)
      }
    },
  },
})

function appendTail(existing: string, chunk: string) {
  const maxTailLength = 4096
  const next = [existing, chunk].filter(Boolean).join("\n")
  return next.length > maxTailLength ? next.slice(-maxTailLength) : next
}

function computeAggregate(total: number, executions: TaskDetail["executions"]) {
  const runningStates: TaskStatus[] = ["approved", "queued", "dispatched", "running"]
  const success = executions.filter((item) => item.status === "success").length
  const failed = executions.filter((item) => ["failed", "partial_failed", "timeout", "cancelled"].includes(item.status)).length
  const running = executions.filter((item) => runningStates.includes(item.status)).length

  return {
    total,
    success,
    failed,
    offlineSkipped: Math.max(total - success - failed - running, 0),
    running,
  }
}
