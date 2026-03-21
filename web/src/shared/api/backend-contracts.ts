import { z } from "zod"
import {
  approvalStatusSchema,
  auditEventSchema,
  consoleModeSchema,
  nodeStatusSchema,
  nodeSummarySchema,
  riskLevelSchema,
  sessionSchema,
  taskDetailSchema,
  taskExecutionSchema,
  taskPlanSchema,
  taskStatusSchema,
  type ApprovalStatus,
  type AuditEvent,
  type ConsoleMode,
  type NodeStatus,
  type NodeSummary,
  type RiskLevel,
  type SessionInfo,
  type TaskAggregate,
  type TaskDetail,
  type TaskExecution,
  type TaskPlan,
  type TaskStatus,
} from "@/shared/api/contracts"

const rawUserSchema = z.object({
  id: z.string(),
  name: z.string().optional(),
  username: z.string().optional(),
  role: z.string(),
})

export const rawMeResponseSchema = z.object({
  user: rawUserSchema,
})

export const rawLoginResponseSchema = rawMeResponseSchema.extend({
  token: z.string(),
})

const rawNodeSchema = z.object({
  id: z.string(),
  hostname: z.string(),
  region: z.string(),
  os: z.string(),
  version: z.string(),
  tags: z.array(z.string()).default([]),
  status: z.string(),
  last_seen_at: z.string(),
  auth_secret_version: z.number(),
  created_at: z.string(),
  updated_at: z.string(),
  busy: z.boolean().optional().default(false),
  metrics: z.object({
    cpu: z.number().optional().default(0),
    memory: z.number().optional().default(0),
    disk: z.number().optional().default(0),
  }).optional().default({
    cpu: 0,
    memory: 0,
    disk: 0,
  }),
})

export const rawNodesResponseSchema = z.object({
  nodes: z.array(rawNodeSchema),
})

const rawTaskAggregateSchema = z.object({
  total: z.number(),
  success: z.number(),
  failed: z.number(),
  offline_skipped: z.number(),
  running: z.number(),
})

const rawPlanStepSchema = z.object({
  action: z.string(),
  args: z.record(z.string(), z.unknown()).default({}),
  risk: z.string(),
  timeout_sec: z.number().optional(),
  broadcast_allowed: z.boolean().optional(),
})

const rawPlanSchema = z.object({
  target_nodes: z.array(z.string()).default([]),
  summary: z.string(),
  estimated_impact: z.string(),
  risk_level: z.string(),
  requires_approval: z.boolean(),
  steps: z.array(rawPlanStepSchema).default([]),
  metadata: z.record(z.string(), z.unknown()).optional(),
})

const rawTaskSchema = z.object({
  id: z.string(),
  parent_task_id: z.string().nullable().optional(),
  mode: z.string(),
  initiator_id: z.string(),
  initiator_role: z.string().optional(),
  target: z.array(z.string()).default([]),
  input_text: z.string(),
  plan: rawPlanSchema,
  risk_level: z.string(),
  approval_status: z.string(),
  required_approval_role: z.string().optional().default(""),
  approver_id: z.string().optional().default(""),
  final_status: z.string(),
  status_reason: z.string().optional().default(""),
  result_summary: z.string().optional().default(""),
  failure_node_ids: z.array(z.string()).optional().default([]),
  summary_source: z.string().optional().default(""),
  created_at: z.string(),
  updated_at: z.string(),
})

const rawTaskDetailEnvelopeSchema = z.object({
  task: rawTaskSchema,
  aggregate: rawTaskAggregateSchema.optional(),
  summary: z.string().optional(),
})

export const rawTaskDetailResponseSchema = rawTaskDetailEnvelopeSchema

export const rawTasksResponseSchema = z.object({
  tasks: z.array(rawTaskDetailEnvelopeSchema),
})

const rawExecutionSchema = z.object({
  id: z.string(),
  task_id: z.string(),
  node_id: z.string(),
  status: z.string(),
  attempt: z.number(),
  started_at: z.string().optional().nullable(),
  finished_at: z.string().optional().nullable(),
  exit_code: z.number().optional().nullable(),
  stdout_tail: z.string().default(""),
  stderr_tail: z.string().default(""),
  status_reason: z.string().default(""),
})

export const rawExecutionsResponseSchema = z.object({
  executions: z.array(rawExecutionSchema),
})

const rawAuditEventSchema = z.object({
  id: z.string(),
  task_id: z.string(),
  actor_id: z.string(),
  event_type: z.string(),
  payload: z.record(z.string(), z.unknown()).default({}),
  created_at: z.string(),
})

export const rawAuditsResponseSchema = z.object({
  events: z.array(rawAuditEventSchema),
})

export const rawTaskMutationResponseSchema = z.object({
  task_id: z.string(),
  status: z.string(),
  message: z.string(),
})

export const rawPlanTaskResponseSchema = z.object({
  task_id: z.string(),
  status: z.string(),
  plan: rawPlanSchema,
})

export const rawWelcomeWsEventSchema = z.object({
  type: z.literal("welcome"),
  message: z.string(),
})

function enumOrFallback<T extends string>(
  schema: z.ZodEnum<[T, ...T[]]>,
  value: string,
  fallback: T,
): T {
  const parsed = schema.safeParse(value)
  return parsed.success ? parsed.data : fallback
}

function normalizeNodeStatus(value: string): NodeStatus {
  return enumOrFallback(nodeStatusSchema, value as NodeStatus, "stale")
}

function normalizeRiskLevel(value: string): RiskLevel {
  return enumOrFallback(riskLevelSchema, value as RiskLevel, "low")
}

function normalizeTaskStatus(value: string): TaskStatus {
  return enumOrFallback(taskStatusSchema, value as TaskStatus, "planned")
}

function normalizeApprovalStatus(value: string): ApprovalStatus {
  return enumOrFallback(approvalStatusSchema, value as ApprovalStatus, "pending")
}

function normalizeConsoleMode(value: string): ConsoleMode {
  if (value === "manual_command") {
    return "direct_shell"
  }

  return enumOrFallback(consoleModeSchema, value as ConsoleMode, "ai_agent")
}

function stringifyUnknown(value: unknown) {
  if (typeof value === "string" || typeof value === "number" || typeof value === "boolean") {
    return value
  }

  return JSON.stringify(value)
}

function buildAggregate(executions: TaskExecution[], targetCount: number): TaskAggregate {
  const runningStatuses = new Set<TaskStatus>(["approved", "queued", "dispatched", "running"])
  const success = executions.filter((item) => item.status === "success").length
  const failed = executions.filter((item) =>
    ["failed", "partial_failed", "timeout", "cancelled"].includes(item.status),
  ).length
  const running = executions.filter((item) => runningStatuses.has(item.status)).length

  return {
    total: targetCount,
    success,
    failed,
    offlineSkipped: Math.max(targetCount - executions.length, 0),
    running,
  }
}

function mapAggregate(input: z.infer<typeof rawTaskAggregateSchema>): TaskAggregate {
  return {
    total: input.total,
    success: input.success,
    failed: input.failed,
    offlineSkipped: input.offline_skipped,
    running: input.running,
  }
}

export function mapMeResponse(input: unknown): SessionInfo {
  const parsed = rawMeResponseSchema.parse(input)

  return sessionSchema.parse({
    id: parsed.user.id,
    name: parsed.user.name ?? parsed.user.id,
    username: parsed.user.username,
    role: parsed.user.role,
  })
}

export function mapLoginResponse(input: unknown): { session: SessionInfo; token: string } {
  const parsed = rawLoginResponseSchema.parse(input)

  return {
    session: sessionSchema.parse({
      id: parsed.user.id,
      name: parsed.user.name ?? parsed.user.id,
      username: parsed.user.username,
      role: parsed.user.role,
    }),
    token: parsed.token,
  }
}

export function mapNodesResponse(input: unknown): NodeSummary[] {
  const parsed = rawNodesResponseSchema.parse(input)

  return nodeSummarySchema.array().parse(
    parsed.nodes.map((node) => ({
      id: node.id,
      hostname: node.hostname,
      region: node.region,
      os: node.os,
      version: node.version,
      tags: node.tags,
      status: normalizeNodeStatus(node.status),
      busy: node.busy,
      lastSeen: node.last_seen_at,
      metrics: {
        cpu: node.metrics.cpu,
        memory: node.metrics.memory,
        disk: node.metrics.disk,
      },
    })),
  )
}

export function mapPlan(input: z.infer<typeof rawPlanSchema>): TaskPlan {
  return taskPlanSchema.parse({
    targetNodes: input.target_nodes,
    summary: input.summary,
    estimatedImpact: input.estimated_impact,
    riskLevel: normalizeRiskLevel(input.risk_level),
    requiresApproval: input.requires_approval,
    metadata: input.metadata,
    steps: input.steps.map((step, index) => ({
      id: `${step.action}_${index + 1}`,
      action: step.action,
      args: Object.fromEntries(
        Object.entries(step.args).map(([key, value]) => [key, stringifyUnknown(value)]),
      ),
      risk: normalizeRiskLevel(step.risk),
      timeoutSec: step.timeout_sec,
      broadcastAllowed: step.broadcast_allowed,
    })),
  })
}

export function mapExecutionsResponse(input: unknown): TaskExecution[] {
  const parsed = rawExecutionsResponseSchema.parse(input)

  return taskExecutionSchema.array().parse(
    parsed.executions.map((execution) => ({
      id: execution.id,
      taskId: execution.task_id,
      nodeId: execution.node_id,
      status: normalizeTaskStatus(execution.status),
      startedAt: execution.started_at ?? undefined,
      finishedAt: execution.finished_at ?? undefined,
      exitCode: execution.exit_code ?? null,
      stdoutTail: execution.stdout_tail,
      stderrTail: execution.stderr_tail,
      streamSummary:
        execution.status_reason ||
        execution.stdout_tail ||
        execution.stderr_tail ||
        execution.status,
    })),
  )
}

export function mapTaskDetailResponse(input: unknown, executions: TaskExecution[] = []): TaskDetail {
  const parsed = rawTaskDetailResponseSchema.parse(input)
  const plan = mapPlan(parsed.task.plan)
  const aggregate = parsed.aggregate
    ? mapAggregate(parsed.aggregate)
    : buildAggregate(executions, parsed.task.target.length || plan.targetNodes.length)
  const summary = parsed.summary ?? (parsed.task.status_reason || plan.summary)

  return taskDetailSchema.parse({
    id: parsed.task.id,
    mode: normalizeConsoleMode(parsed.task.mode),
    inputText: parsed.task.input_text,
    target: parsed.task.target,
    createdAt: parsed.task.created_at,
    updatedAt: parsed.task.updated_at,
    status: normalizeTaskStatus(parsed.task.final_status),
    approvalStatus: normalizeApprovalStatus(parsed.task.approval_status),
    requiredApprovalRole: parsed.task.required_approval_role || undefined,
    riskLevel: normalizeRiskLevel(parsed.task.risk_level),
    statusReason: parsed.task.status_reason,
    plan,
    aggregate,
    summary,
    resultSummary: parsed.task.result_summary || summary,
    failureNodeIds: parsed.task.failure_node_ids,
    summarySource: parsed.task.summary_source || undefined,
    executions,
  })
}

export function mapTasksResponse(input: unknown): TaskDetail[] {
  const parsed = rawTasksResponseSchema.parse(input)

  return taskDetailSchema.array().parse(
    parsed.tasks.map((item) => mapTaskDetailResponse(item, [])),
  )
}

export function mapAuditEventsResponse(input: unknown): AuditEvent[] {
  const parsed = rawAuditsResponseSchema.parse(input)

  return auditEventSchema.array().parse(
    parsed.events.map((event) => ({
      id: event.id,
      taskId: event.task_id,
      actorId: event.actor_id,
      action: event.event_type,
      description:
        typeof event.payload.description === "string"
          ? event.payload.description
          : typeof event.payload.message === "string"
            ? event.payload.message
            : typeof event.payload.reason === "string"
              ? event.payload.reason
              : event.event_type,
      createdAt: event.created_at,
    })),
  )
}
