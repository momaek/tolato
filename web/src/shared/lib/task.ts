import type { NodeStatus, RiskLevel, TaskStatus } from "@/shared/api/contracts"

export const taskStatusLabelMap: Record<TaskStatus, string> = {
  planned: "Planned",
  waiting_approval: "Waiting Approval",
  approved: "Approved",
  queued: "Queued",
  dispatched: "Dispatched",
  running: "Running",
  success: "Success",
  failed: "Failed",
  partial_failed: "Partial Failed",
  timeout: "Timeout",
  cancelled: "Cancelled",
}

export const taskStatusClassMap: Record<TaskStatus, string> = {
  planned: "bg-muted text-muted-foreground border-border",
  waiting_approval: "bg-amber-100 text-amber-900 border-amber-200",
  approved: "bg-sky-100 text-sky-900 border-sky-200",
  queued: "bg-cyan-100 text-cyan-900 border-cyan-200",
  dispatched: "bg-cyan-100 text-cyan-900 border-cyan-200",
  running: "bg-blue-100 text-blue-900 border-blue-200",
  success: "bg-emerald-100 text-emerald-900 border-emerald-200",
  failed: "bg-red-100 text-red-900 border-red-200",
  partial_failed: "bg-orange-100 text-orange-900 border-orange-200",
  timeout: "bg-orange-100 text-orange-900 border-orange-200",
  cancelled: "bg-stone-100 text-stone-800 border-stone-200",
}

export const riskLevelLabelMap: Record<RiskLevel, string> = {
  low: "Low Risk",
  medium: "Medium Risk",
  high: "High Risk",
  forbidden: "Forbidden",
}

export const riskLevelClassMap: Record<RiskLevel, string> = {
  low: "bg-emerald-100 text-emerald-900 border-emerald-200",
  medium: "bg-amber-100 text-amber-900 border-amber-200",
  high: "bg-red-100 text-red-900 border-red-200",
  forbidden: "bg-red-600 text-white border-red-700",
}

export const nodeStatusLabelMap: Record<NodeStatus, string> = {
  online: "Online",
  stale: "Stale",
  offline: "Offline",
}

export const nodeStatusClassMap: Record<NodeStatus, string> = {
  online: "bg-emerald-100 text-emerald-900 border-emerald-200",
  stale: "bg-amber-100 text-amber-900 border-amber-200",
  offline: "bg-red-100 text-red-900 border-red-200",
}
