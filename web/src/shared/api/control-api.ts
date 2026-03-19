import {
  auditEventSchema,
  nodeSummarySchema,
  sessionSchema,
  taskDetailSchema,
  type ConsoleMode,
  type AuditEvent,
  type NodeSummary,
  type SessionInfo,
  type TaskDetail,
} from "@/shared/api/contracts"
import {
  mapAuditEventsResponse,
  mapExecutionsResponse,
  mapMeResponse,
  mapNodesResponse,
  mapPlan,
  mapTaskDetailResponse,
  rawPlanTaskResponseSchema,
  rawTaskMutationResponseSchema,
} from "@/shared/api/backend-contracts"
import { mockAudits, mockNodes, mockSession, mockTasks } from "@/shared/api/mock-data"

export interface ControlApiAdapter {
  getSession(): Promise<SessionInfo>
  getNodes(): Promise<NodeSummary[]>
  getTasks(): Promise<TaskDetail[]>
  getTask(taskId: string): Promise<TaskDetail | null>
  getAudits(taskId?: string): Promise<AuditEvent[]>
  planTask(input: { mode: ConsoleMode; target: string[]; inputText: string }): Promise<TaskDetail | null>
  approveTask(taskId: string): Promise<TaskDetail | null>
  rejectTask(taskId: string): Promise<TaskDetail | null>
  cancelTask(taskId: string): Promise<TaskDetail | null>
}

class MockControlApi implements ControlApiAdapter {
  async getSession() {
    return sessionSchema.parse(mockSession)
  }

  async getNodes() {
    return nodeSummarySchema.array().parse(mockNodes)
  }

  async getTasks() {
    return taskDetailSchema.array().parse(mockTasks)
  }

  async getTask(taskId: string) {
    const task = mockTasks.find((item) => item.id === taskId) ?? null
    return task ? taskDetailSchema.parse(task) : null
  }

  async getAudits() {
    return auditEventSchema.array().parse(mockAudits)
  }

  async planTask() {
    const task = mockTasks[0] ?? null
    return task ? taskDetailSchema.parse(task) : null
  }

  async approveTask(taskId: string) {
    const task = mockTasks.find((item) => item.id === taskId) ?? null
    if (!task) return null
    return taskDetailSchema.parse({
      ...task,
      approvalStatus: "approved",
      status: "approved",
    })
  }

  async rejectTask(taskId: string) {
    const task = mockTasks.find((item) => item.id === taskId) ?? null
    if (!task) return null
    return taskDetailSchema.parse({
      ...task,
      approvalStatus: "rejected",
      status: "cancelled",
      summary: "Task rejected by operator.",
    })
  }

  async cancelTask(taskId: string) {
    const task = mockTasks.find((item) => item.id === taskId) ?? null
    if (!task) return null
    return taskDetailSchema.parse({
      ...task,
      approvalStatus: task.approvalStatus === "pending" ? "cancelled" : task.approvalStatus,
      status: "cancelled",
      summary: "Task cancelled by operator.",
    })
  }
}

class RealControlApi implements ControlApiAdapter {
  private readonly baseUrl: string

  constructor(baseUrl: string) {
    this.baseUrl = baseUrl
  }

  private async readJson<T>(path: string, parser: (value: unknown) => T) {
    const response = await fetch(new URL(path, this.baseUrl))

    if (!response.ok) {
      throw new Error(`HTTP ${response.status} for ${path}`)
    }

    return parser(await response.json())
  }

  private async readJsonOrNull<T>(path: string, parser: (value: unknown) => T) {
    const response = await fetch(new URL(path, this.baseUrl))

    if (response.status === 404) {
      return null
    }

    if (!response.ok) {
      throw new Error(`HTTP ${response.status} for ${path}`)
    }

    return parser(await response.json())
  }

  private async mutateTask(path: string) {
    const response = await fetch(new URL(path, this.baseUrl), {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
      },
    })

    if (!response.ok) {
      throw new Error(`HTTP ${response.status} for ${path}`)
    }

    const payload = rawTaskMutationResponseSchema.parse(await response.json())
    return this.getTask(payload.task_id)
  }

  async getSession() {
    return this.readJson("/api/v1/me", mapMeResponse)
  }

  async getNodes() {
    return this.readJson("/api/v1/nodes", mapNodesResponse)
  }

  async getTasks() {
    const tasks = await this.readJsonOrNull("/api/v1/tasks", taskDetailSchema.array().parse)
    return tasks ?? []
  }

  async getTask(taskId: string) {
    try {
      const [taskResponse, executionsResponse] = await Promise.all([
        this.readJson(`/api/v1/tasks/${taskId}`, (value) => value),
        this.readJson(`/api/v1/tasks/${taskId}/executions`, (value) => value),
      ])

      const executions = mapExecutionsResponse(executionsResponse)
      return mapTaskDetailResponse(taskResponse, executions)
    } catch {
      return null
    }
  }

  async getAudits(taskId?: string) {
    const query = taskId ? `?task_id=${encodeURIComponent(taskId)}` : ""
    return this.readJson(`/api/v1/audits${query}`, mapAuditEventsResponse)
  }

  async planTask(input: { mode: ConsoleMode; target: string[]; inputText: string }) {
    const response = await fetch(new URL("/api/v1/tasks/plan", this.baseUrl), {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
      },
      body: JSON.stringify({
        mode: input.mode === "direct_shell" ? "manual_command" : input.mode,
        target: input.target,
        input_text: input.inputText,
      }),
    })

    if (!response.ok) {
      throw new Error(`HTTP ${response.status} for /api/v1/tasks/plan`)
    }

    const payload = rawPlanTaskResponseSchema.parse(await response.json())
    const task = await this.getTask(payload.task_id)

    if (task) {
      return task
    }

    return taskDetailSchema.parse({
      id: payload.task_id,
      mode: input.mode,
      inputText: input.inputText,
      target: input.target,
      createdAt: new Date().toISOString(),
      status: payload.status,
      approvalStatus: payload.status === "waiting_approval" ? "pending" : "not_required",
      plan: mapPlan(payload.plan),
      aggregate: {
        total: input.target.length,
        success: 0,
        failed: 0,
        offlineSkipped: 0,
        running: 0,
      },
      summary: payload.plan.summary,
      executions: [],
    })
  }

  async approveTask(taskId: string) {
    return this.mutateTask(`/api/v1/tasks/${taskId}/approve`)
  }

  async rejectTask(taskId: string) {
    return this.mutateTask(`/api/v1/tasks/${taskId}/reject`)
  }

  async cancelTask(taskId: string) {
    return this.mutateTask(`/api/v1/tasks/${taskId}/cancel`)
  }
}

const useMock = import.meta.env.VITE_USE_MOCK !== "false"
const apiBaseUrl = import.meta.env.VITE_API_BASE_URL ?? "http://localhost:8080"

export const controlApi: ControlApiAdapter = useMock
  ? new MockControlApi()
  : new RealControlApi(apiBaseUrl)
