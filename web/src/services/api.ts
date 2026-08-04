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
  NotifySettings,
  NotifyPreset,
  AuditLogQuery,
  AuditLogItem,
  PaginatedResponse,
  PaginationQuery,
  NodeCommandItem,
  ToolCallOutputResponse,
  UserItem,
  CreateUserRequest,
  UpdateUserRequest,
  ChangePasswordRequest,
  APIKeyListItem,
  CreateAPIKeyRequest,
  CreateAPIKeyResponse,
  OIDCSettings,
  OIDCSettingsResponse,
  OIDCStatusResponse,
  VerifyOIDCResponse,
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

export async function getCurrentUser(): Promise<UserItem> {
  const res = await api.get<UserItem>('/auth/me')
  return res.data
}

export async function changeOwnPassword(data: ChangePasswordRequest): Promise<void> {
  await api.put('/auth/password', data)
}

// --- Single sign-on ---

/** Unauthenticated: drives the SSO button on the login page. */
export async function getOIDCStatus(): Promise<OIDCStatusResponse> {
  const res = await api.get<OIDCStatusResponse>('/auth/oidc/status')
  return res.data
}

export async function getOIDCSettings(): Promise<OIDCSettingsResponse> {
  const res = await api.get<OIDCSettingsResponse>('/settings/oidc')
  return res.data
}

export async function updateOIDCSettings(data: OIDCSettings): Promise<void> {
  await api.put('/settings/oidc', data)
}

export async function verifyOIDC(data: OIDCSettings): Promise<VerifyOIDCResponse> {
  const res = await api.post<VerifyOIDCResponse>('/settings/oidc/verify', data)
  return res.data
}

// --- User management (admin only) ---

export async function getUsers(): Promise<UserItem[]> {
  const res = await api.get<{ items: UserItem[] }>('/users')
  return res.data.items ?? []
}

export async function createUser(data: CreateUserRequest): Promise<UserItem> {
  const res = await api.post<UserItem>('/users', data)
  return res.data
}

export async function updateUser(id: string, data: UpdateUserRequest): Promise<UserItem> {
  const res = await api.put<UserItem>(`/users/${id}`, data)
  return res.data
}

export async function deleteUser(id: string): Promise<void> {
  await api.delete(`/users/${id}`)
}

export interface VersionInfo {
  current: string
  latest: string
  has_update: boolean
  release_url: string
  self_node?: string
}

export async function getVersionInfo(): Promise<VersionInfo> {
  const res = await api.get<VersionInfo>('/version')
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

// Fetch the full, untruncated stdout/stderr for one tool call. Used by
// ToolCallCard when the conversation payload only delivered a head preview.
export async function getToolCallOutput(
  conversationId: string,
  toolCallId: string
): Promise<ToolCallOutputResponse> {
  const res = await api.get<ToolCallOutputResponse>(
    `/conversations/${conversationId}/tool-calls/${toolCallId}/output`
  )
  return res.data
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

export interface UpdateAgentResponse {
  message: string
  old_version: string
  new_version: string
}

export async function updateNodeAgent(id: string): Promise<UpdateAgentResponse> {
  // The agent downloads + verifies + swaps the binary then restarts, so this
  // can take a while; give it a generous client timeout.
  const res = await api.post<UpdateAgentResponse>(`/nodes/${id}/update`, null, {
    timeout: 6 * 60 * 1000,
  })
  return res.data
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

export async function getNotifySettings(): Promise<NotifySettings> {
  const res = await api.get<NotifySettings>('/settings/notify')
  return res.data
}

export async function updateNotifySettings(data: NotifySettings): Promise<void> {
  await api.put('/settings/notify', data)
}

export async function getNotifyPresets(): Promise<NotifyPreset[]> {
  const res = await api.get<{ presets: NotifyPreset[] }>('/settings/notify/presets')
  return res.data.presets || []
}

export async function testNotifyChannel(name: string): Promise<void> {
  await api.post('/settings/notify/test', { name })
}

// --- Audit Logs ---

export async function getAuditLogs(query: AuditLogQuery): Promise<PaginatedResponse<AuditLogItem>> {
  const res = await api.get<PaginatedResponse<AuditLogItem>>('/audit-logs', { params: query })
  return res.data
}

// --- API Keys ---

export async function getAPIKeys(): Promise<APIKeyListItem[]> {
  const res = await api.get<APIKeyListItem[]>('/api-keys')
  return res.data
}

export async function createAPIKey(data: CreateAPIKeyRequest): Promise<CreateAPIKeyResponse> {
  const res = await api.post<CreateAPIKeyResponse>('/api-keys', data)
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
