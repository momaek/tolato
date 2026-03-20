import { z } from "zod"

export const nodeStatusSchema = z.enum(["online", "stale", "offline"])
export const riskLevelSchema = z.enum(["low", "medium", "high", "forbidden"])
export const taskStatusSchema = z.enum([
  "planned",
  "waiting_approval",
  "approved",
  "queued",
  "dispatched",
  "running",
  "success",
  "failed",
  "partial_failed",
  "timeout",
  "cancelled",
])
export const approvalStatusSchema = z.enum([
  "not_required",
  "pending",
  "approved",
  "rejected",
  "cancelled",
])
export const consoleModeSchema = z.enum(["ai_agent", "direct_shell"])

export const sessionSchema = z.object({
  id: z.string(),
  name: z.string(),
  role: z.string(),
})

export const nodeSummarySchema = z.object({
  id: z.string(),
  hostname: z.string(),
  region: z.string(),
  os: z.string(),
  version: z.string(),
  tags: z.array(z.string()),
  status: nodeStatusSchema,
  busy: z.boolean(),
  lastSeen: z.string(),
  metrics: z.object({
    cpu: z.number(),
    memory: z.number(),
    disk: z.number(),
  }),
})

export const taskStepSchema = z.object({
  id: z.string(),
  action: z.string(),
  args: z.record(z.string(), z.unknown()),
  risk: riskLevelSchema,
  timeoutSec: z.number().optional(),
  broadcastAllowed: z.boolean().optional(),
})

export const taskPlanSchema = z.object({
  targetNodes: z.array(z.string()),
  summary: z.string(),
  estimatedImpact: z.string(),
  riskLevel: riskLevelSchema,
  requiresApproval: z.boolean(),
  steps: z.array(taskStepSchema),
  metadata: z.record(z.string(), z.unknown()).optional(),
})

export const taskAggregateSchema = z.object({
  total: z.number(),
  success: z.number(),
  failed: z.number(),
  offlineSkipped: z.number(),
  running: z.number(),
})

export const taskExecutionSchema = z.object({
  id: z.string(),
  taskId: z.string(),
  nodeId: z.string(),
  status: taskStatusSchema,
  startedAt: z.string().optional(),
  finishedAt: z.string().optional(),
  exitCode: z.number().nullable(),
  stdoutTail: z.string(),
  stderrTail: z.string(),
  streamSummary: z.string(),
})

export const auditEventSchema = z.object({
  id: z.string(),
  taskId: z.string(),
  actorId: z.string(),
  action: z.string(),
  description: z.string(),
  createdAt: z.string(),
})

export const taskDetailSchema = z.object({
  id: z.string(),
  mode: consoleModeSchema,
  inputText: z.string(),
  target: z.array(z.string()),
  createdAt: z.string(),
  updatedAt: z.string().optional(),
  status: taskStatusSchema,
  approvalStatus: approvalStatusSchema,
  riskLevel: riskLevelSchema.optional(),
  statusReason: z.string().optional(),
  plan: taskPlanSchema,
  aggregate: taskAggregateSchema,
  summary: z.string(),
  executions: z.array(taskExecutionSchema),
})

export const connectionEventSchema = z.object({
  type: z.enum(["connection.ready", "connection.synced", "connection.disconnected", "connection.error"]),
  timestamp: z.string(),
  message: z.string().optional(),
})

export const nodeUpdatedEventSchema = z.object({
  type: z.literal("node.updated"),
  node: nodeSummarySchema,
})

export const taskStatusEventSchema = z.object({
  type: z.literal("task.status"),
  taskId: z.string(),
  status: taskStatusSchema,
  timestamp: z.string(),
})

export const taskLogEventSchema = z.object({
  type: z.literal("task.log"),
  taskId: z.string(),
  executionId: z.string(),
  nodeId: z.string(),
  stream: z.enum(["stdout", "stderr"]),
  chunk: z.string(),
  timestamp: z.string(),
})

export const taskResultEventSchema = z.object({
  type: z.literal("task.result"),
  taskId: z.string(),
  execution: taskExecutionSchema,
  timestamp: z.string(),
})

export const uiWsEventSchema = z.discriminatedUnion("type", [
  connectionEventSchema,
  nodeUpdatedEventSchema,
  taskStatusEventSchema,
  taskLogEventSchema,
  taskResultEventSchema,
])

export type ApprovalStatus = z.infer<typeof approvalStatusSchema>
export type AuditEvent = z.infer<typeof auditEventSchema>
export type ConsoleMode = z.infer<typeof consoleModeSchema>
export type NodeStatus = z.infer<typeof nodeStatusSchema>
export type NodeSummary = z.infer<typeof nodeSummarySchema>
export type RiskLevel = z.infer<typeof riskLevelSchema>
export type SessionInfo = z.infer<typeof sessionSchema>
export type TaskAggregate = z.infer<typeof taskAggregateSchema>
export type TaskDetail = z.infer<typeof taskDetailSchema>
export type TaskExecution = z.infer<typeof taskExecutionSchema>
export type TaskPlan = z.infer<typeof taskPlanSchema>
export type TaskStep = z.infer<typeof taskStepSchema>
export type TaskStatus = z.infer<typeof taskStatusSchema>
export type UiWsEvent = z.infer<typeof uiWsEventSchema>
