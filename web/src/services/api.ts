import axios from 'axios'
import type {
  LoginRequest,
  LoginResponse,
  ConversationSummary,
  ConversationDetail,
  CreateConversationRequest,
  UpdateConversationRequest,
  NodeListItem,
  NodeDetail,
  CreateNodeRequest,
  CreateNodeResponse,
  UpdateNodeRequest,
  LLMSettings,
  VerifyLLMResponse,
  SecuritySettings,
  AgentSettings,
  ChatSettings,
  WebFetchSettings,
  VerifyWebFetchResponse,
  AuditLogQuery,
  AuditLogItem,
  PaginatedResponse,
  PaginationQuery,
  NodeCommandItem,
} from '@/types/api'
import router from '@/router'

const api = axios.create({
  baseURL: '/api',
  timeout: 30000,
  headers: {
    'Content-Type': 'application/json',
  },
})

// Request interceptor: attach JWT
api.interceptors.request.use((config) => {
  const token = localStorage.getItem('token')
  if (token) {
    config.headers.Authorization = `Bearer ${token}`
  }
  return config
})

// Response interceptor: handle 401
api.interceptors.response.use(
  (response) => response,
  (error) => {
    if (error.response?.status === 401) {
      localStorage.removeItem('token')
      router.push('/login')
    }
    return Promise.reject(error)
  }
)

// --- Auth ---

export async function login(data: LoginRequest): Promise<LoginResponse> {
  const res = await api.post<LoginResponse>('/auth/login', data)
  return res.data
}

// --- Conversations ---

export async function getConversations(): Promise<ConversationSummary[]> {
  const res = await api.get<PaginatedResponse<ConversationSummary>>('/conversations')
  return res.data.items ?? []
}

export async function getConversation(id: string): Promise<ConversationDetail> {
  const res = await api.get<ConversationDetail>(`/conversations/${id}`)
  return res.data
}

export async function createConversation(data: CreateConversationRequest): Promise<ConversationSummary> {
  const res = await api.post<ConversationSummary>('/conversations', data)
  return res.data
}

export async function updateConversation(id: string, data: UpdateConversationRequest): Promise<void> {
  await api.put(`/conversations/${id}`, data)
}

export async function deleteConversation(id: string): Promise<void> {
  await api.delete(`/conversations/${id}`)
}

export async function deleteMessage(conversationId: string, messageId: string): Promise<void> {
  await api.delete(`/conversations/${conversationId}/messages/${messageId}`)
}

// --- Nodes ---

export async function getNodes(): Promise<NodeListItem[]> {
  const res = await api.get<PaginatedResponse<NodeListItem>>('/nodes')
  return res.data.items ?? []
}

export async function getNode(id: string): Promise<NodeDetail> {
  const res = await api.get<NodeDetail>(`/nodes/${id}`)
  return res.data
}

export async function createNode(data: CreateNodeRequest): Promise<CreateNodeResponse> {
  const res = await api.post<CreateNodeResponse>('/nodes', data)
  return res.data
}

export async function updateNode(id: string, data: UpdateNodeRequest): Promise<void> {
  await api.put(`/nodes/${id}`, data)
}

export async function deleteNode(id: string): Promise<void> {
  await api.delete(`/nodes/${id}`)
}

// --- Settings ---

export async function getLLMSettings(): Promise<LLMSettings> {
  const res = await api.get<LLMSettings>('/settings/llm')
  return res.data
}

export async function updateLLMSettings(data: Partial<LLMSettings>): Promise<void> {
  await api.put('/settings/llm', data)
}

export async function verifyLLM(
  payload?: { api_base_url?: string; api_key?: string }
): Promise<VerifyLLMResponse> {
  const res = await api.post<VerifyLLMResponse>('/settings/llm/verify', payload ?? {})
  return res.data
}

export async function getLLMModels(): Promise<string[]> {
  const res = await api.get<{ models: string[] }>('/settings/llm/models')
  return res.data.models || []
}

export async function getSecuritySettings(): Promise<SecuritySettings> {
  const res = await api.get<SecuritySettings>('/settings/security')
  return res.data
}

export async function updateSecuritySettings(data: Partial<SecuritySettings>): Promise<void> {
  await api.put('/settings/security', data)
}

export async function getAgentSettings(): Promise<AgentSettings> {
  const res = await api.get<AgentSettings>('/settings/agent')
  return res.data
}

export async function updateAgentSettings(data: Partial<AgentSettings>): Promise<void> {
  await api.put('/settings/agent', data)
}

export async function getChatSettings(): Promise<ChatSettings> {
  const res = await api.get<ChatSettings>('/settings/chat')
  return res.data
}

export async function updateChatSettings(data: Partial<ChatSettings>): Promise<void> {
  await api.put('/settings/chat', data)
}

export async function getWebFetchSettings(): Promise<WebFetchSettings> {
  const res = await api.get<WebFetchSettings>('/settings/webfetch')
  return res.data
}

export async function updateWebFetchSettings(data: Partial<WebFetchSettings>): Promise<void> {
  await api.put('/settings/webfetch', data)
}

export async function verifyWebFetch(
  payload?: { mode?: string; jina_api_key?: string }
): Promise<VerifyWebFetchResponse> {
  const res = await api.post<VerifyWebFetchResponse>('/settings/webfetch/verify', payload ?? {})
  return res.data
}

// --- Audit Logs ---

export async function getAuditLogs(query: AuditLogQuery): Promise<PaginatedResponse<AuditLogItem>> {
  const res = await api.get<PaginatedResponse<AuditLogItem>>('/audit-logs', { params: query })
  return res.data
}

// --- API Keys ---

export async function getAPIKeys(): Promise<any[]> {
  const res = await api.get('/api-keys')
  return res.data
}

export async function createAPIKey(data: { name: string; permission: string }): Promise<any> {
  const res = await api.post('/api-keys', data)
  return res.data
}

export async function deleteAPIKey(id: string): Promise<void> {
  await api.delete(`/api-keys/${id}`)
}

// --- Node Commands ---

export async function getNodeCommands(
  nodeId: string,
  query: PaginationQuery = {}
): Promise<PaginatedResponse<NodeCommandItem>> {
  const res = await api.get<PaginatedResponse<NodeCommandItem>>(`/nodes/${nodeId}/commands`, { params: query })
  return res.data
}

export default api
