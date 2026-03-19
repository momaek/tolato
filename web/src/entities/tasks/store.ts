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
    consumeWsEvent(event: UiWsEvent) {
      if (event.type === "task.status") {
        this.updateTaskStatus(event.taskId, event.status)
      }
    },
  },
})
