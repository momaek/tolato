export type NodeStatus = 'online' | 'busy' | 'stale' | 'offline'

export interface NodeMetrics {
  cpu: number
  memory: number
  disk: number
}

export interface NodeTaskSummary {
  id: string
  title: string
  status: 'success' | 'failed' | 'running' | 'waiting_approval'
  createdAt: string
}

export interface NodeSummary {
  id: string
  hostname: string
  region: string
  os: string
  version: string
  ipAddress: string
  provider: string
  tags: string[]
  status: NodeStatus
  busy: boolean
  lastSeen: string
  metrics: NodeMetrics
}

export interface NodeDetail extends NodeSummary {
  kernel: string
  uptime: string
  agentVersion: string
  riskSignals: string[]
  recentTasks: NodeTaskSummary[]
}
