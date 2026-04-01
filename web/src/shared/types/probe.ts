export type NodeRole = 'entry' | 'relay' | 'landing'
export type AlertType = 'latency' | 'packet_loss' | 'tcp' | 'bandwidth' | 'offline'
export type LinkStatusType = 'ok' | 'warn' | 'alert' | 'unknown'

export interface ProbeNode {
  id: string
  name: string
  role: NodeRole
  last_seen: string
}

export interface ProbeLink {
  id: string
  source_id: string
  target_id: string
}

export interface LinkStatus {
  id: string
  source_id: string
  target_id: string
  source_name: string
  target_name: string
  latency_avg: number | null
  packet_loss: number | null
  tcp_connect_time: number | null
  bandwidth_mbps: number | null
  status: LinkStatusType
  last_updated: string | null
}

export interface MetricRow {
  id: number
  link_id: string
  timestamp: string
  latency_min: number
  latency_avg: number
  latency_max: number
  packet_loss: number
  tcp_connect_time: number
  bandwidth_mbps: number | null
}

export interface ProbeAlert {
  id: number
  link_id: string
  type: AlertType
  message: string
  triggered_at: string
  resolved_at: string | null
}
