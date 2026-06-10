<script setup lang="ts">
import { ref, onMounted, computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { toast } from 'vue-sonner'
import { CheckCircle, AlertCircle, Loader2, Copy, Check, Key, Trash2, Plus, Send } from 'lucide-vue-next'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Textarea } from '@/components/ui/textarea'
import { Badge } from '@/components/ui/badge'
import { Separator } from '@/components/ui/separator'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogFooter,
} from '@/components/ui/dialog'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import {
  getLLMSettings,
  updateLLMSettings,
  verifyLLM,
  getSecuritySettings,
  updateSecuritySettings,
  getAgentSettings,
  updateAgentSettings,
  getChatSettings,
  updateChatSettings,
  getWebFetchSettings,
  updateWebFetchSettings,
  verifyWebFetch,
  getNotifySettings,
  updateNotifySettings,
  getNotifyPresets,
  testNotifyChannel,
  getAPIKeys,
  createAPIKey,
  deleteAPIKey,
} from '@/services/api'
import type {
  LLMSettings,
  SecuritySettings,
  AgentSettings,
  ChatSettings,
  WebFetchSettings,
  VerifyLLMResponse,
  VerifyWebFetchResponse,
  NotifySettings,
  NotifyPreset,
  NotifyChannel,
  NotifyTokenSource,
} from '@/types/api'

const { t } = useI18n()
const activeTab = ref('llm')

const tabs = computed(() => [
  { id: 'llm', label: t('settings.tabs.llm') },
  { id: 'security', label: t('settings.tabs.security') },
  { id: 'agent', label: t('settings.tabs.agent') },
  { id: 'chat', label: t('settings.tabs.chat') },
  { id: 'web_fetch', label: t('settings.tabs.webFetch') },
  { id: 'notify', label: t('settings.tabs.notify') },
  { id: 'api_keys', label: t('settings.tabs.apiKeys') },
])

// LLM
const llm = ref<LLMSettings>({
  api_base_url: '',
  api_key: '',
  default_model: '',
  max_rounds: 10,
  temperature: 0.7,
  interleaved_thinking: false,
})
const verifyResult = ref<VerifyLLMResponse | null>(null)
const verifying = ref(false)
const availableModels = ref<string[]>([])
const llmSaving = ref(false)

const modelOptions = computed(() => {
  const base = availableModels.value.length
    ? availableModels.value
    : ['gpt-4o', 'gpt-4o-mini', 'claude-3.5-sonnet']
  // Always include the currently-saved model so the Select can display it,
  // even before the user verifies and fetches the full model list.
  const current = llm.value.default_model
  return current && !base.includes(current) ? [current, ...base] : base
})

// Security
const security = ref<SecuritySettings>({
  confirm_enabled: true,
  sensitive_keywords: [],
  command_blacklist: [],
})
const keywordInput = ref('')
const blacklistInput = ref('')
const secSaving = ref(false)

// Agent
const agent = ref<AgentSettings>({
  heartbeat_interval: 30,
  command_timeout: 60,
  output_max_lines: 1000,
})
const agentSaving = ref(false)

// Chat
const chat = ref<ChatSettings>({
  context_rounds: 10,
  output_truncate_lines: 200,
  custom_system_prompt: '',
})
const chatSaving = ref(false)

// Web Fetch
const webFetch = ref<WebFetchSettings>({
  mode: 'jina',
  jina_api_key: '',
  timeout_sec: 30,
  max_kb: 1024,
})
const webFetchSaving = ref(false)
const webFetchVerifying = ref(false)
const webFetchVerifyResult = ref<VerifyWebFetchResponse | null>(null)

// Notifications
type EditChannel = NotifyChannel & { _headers_json?: string }
type EditSource = NotifyTokenSource & { _headers_json?: string }
const notify = ref<NotifySettings>({
  offline_threshold_seconds: 90,
  recover_notify: true,
  startup_grace_seconds: 90,
  token_sources: [],
  channels: [],
})
const notifyPresets = ref<NotifyPreset[]>([])
const notifySaving = ref(false)
const notifyTesting = ref<string | null>(null)

function presetFor(name: string): NotifyPreset | undefined {
  return notifyPresets.value.find((p) => p.preset === name)
}

function isSecretField(key: string): boolean {
  const k = key.toLowerCase()
  return k.includes('secret') || k.includes('token') || k.includes('password') || k.includes('key')
}

// Params for the channel's referenced token source (creating the entry lazily so
// v-model writes persist). Shared across channels using the same preset.
function sourceParams(ch: NotifyChannel): Record<string, string> {
  if (!ch.token_source) return {}
  let src = notify.value.token_sources.find((s) => s.name === ch.token_source)
  if (!src) {
    const tpl = presetFor(ch.preset)?.token_source
    src = tpl
      ? JSON.parse(JSON.stringify(tpl))
      : { name: ch.token_source, request: { url: '' }, extract: { token_path: '' }, params: {} }
    src!.params = src!.params || {}
    notify.value.token_sources.push(src!)
  }
  if (!src!.params) src!.params = {}
  return src!.params
}

function ensureChannelParams(ch: NotifyChannel) {
  if (!ch.params) ch.params = {}
  const preset = presetFor(ch.preset)
  for (const k of preset?.params || []) {
    if (!(k in ch.params)) ch.params[k] = ''
  }
  if (preset?.source_params?.length) {
    ch.token_source = ch.preset
    const sp = sourceParams(ch)
    for (const k of preset.source_params) {
      if (!(k in sp)) sp[k] = ''
    }
  }
}

function addChannel() {
  const ch: EditChannel = {
    name: `channel-${notify.value.channels.length + 1}`,
    enabled: true,
    preset: 'telegram',
    params: {},
  }
  ensureChannelParams(ch)
  notify.value.channels.push(ch)
}

function onPresetChange(ch: EditChannel) {
  ch.params = {}
  ch.token_source = ''
  if (ch.preset === 'custom') {
    ch.webhook = ch.webhook || { method: 'POST', url: '', headers: {}, body_template: '' }
    ch._headers_json = JSON.stringify(ch.webhook.headers || {}, null, 2)
  } else {
    ch.webhook = undefined
    ensureChannelParams(ch)
  }
}

function removeChannel(idx: number) {
  notify.value.channels.splice(idx, 1)
}

// Serialize the custom-webhook headers textarea into the webhook object.
function syncHeaders(ch: EditChannel) {
  if (!ch.webhook) return
  try {
    ch.webhook.headers = ch._headers_json ? JSON.parse(ch._headers_json) : {}
  } catch {
    // leave previous value; validation happens on save
  }
}

// --- Custom-webhook token exchange (the independently configurable token step) ---

// A custom channel uses a token step when it references a token source.
function customTokenEnabled(ch: NotifyChannel): boolean {
  return !!ch.token_source
}

function customSource(ch: NotifyChannel): EditSource | undefined {
  if (!ch.token_source) return undefined
  return notify.value.token_sources.find((s) => s.name === ch.token_source) as EditSource | undefined
}

function toggleCustomToken(ch: EditChannel) {
  if (customTokenEnabled(ch)) {
    ch.token_source = ''
    return
  }
  // Stable, channel-scoped source name so renaming the channel doesn't orphan it.
  let n = 1
  while (notify.value.token_sources.some((s) => s.name === `custom-src-${n}`)) n++
  const name = `custom-src-${n}`
  const src: EditSource = {
    name,
    request: { method: 'POST', url: '', headers: {}, body: '' },
    params: {},
    extract: { token_path: '', expires_path: '', ttl_fallback_seconds: 3600 },
    invalidate_on: { json_path: '', values: [] },
    _headers_json: '{}',
  }
  notify.value.token_sources.push(src)
  ch.token_source = name
}

function syncSourceHeaders(src: EditSource) {
  try {
    src.request.headers = src._headers_json ? JSON.parse(src._headers_json) : {}
  } catch {
    // keep previous; validated on save
  }
}

// invalidate_on.values edited as a comma-separated string; numbers stay numbers.
function invalidateValuesText(src: NotifyTokenSource): string {
  return (src.invalidate_on?.values || []).join(',')
}
function setInvalidateValues(src: NotifyTokenSource, text: string) {
  if (!src.invalidate_on) src.invalidate_on = { json_path: '', values: [] }
  src.invalidate_on.values = text
    .split(',')
    .map((s) => s.trim())
    .filter(Boolean)
    .map((v) => {
      const n = Number(v)
      return isNaN(n) ? v : n
    })
}

function prepareNotifyPayload(): NotifySettings {
  // Parse custom headers and drop token sources no channel references.
  const channels = notify.value.channels.map((c) => {
    const ch = c as EditChannel
    if (ch.preset === 'custom' && ch.webhook) {
      try {
        ch.webhook.headers = ch._headers_json ? JSON.parse(ch._headers_json) : {}
      } catch {
        ch.webhook.headers = ch.webhook.headers || {}
      }
    }
    const { _headers_json, ...rest } = ch
    return rest as NotifyChannel
  })
  const usedSources = new Set(channels.map((c) => c.token_source).filter(Boolean))
  const token_sources = notify.value.token_sources
    .filter((s) => usedSources.has(s.name))
    .map((s) => {
      const src = s as EditSource
      if (src._headers_json !== undefined) {
        try {
          src.request.headers = src._headers_json ? JSON.parse(src._headers_json) : {}
        } catch {
          src.request.headers = src.request.headers || {}
        }
      }
      const { _headers_json, ...rest } = src
      return rest as NotifyTokenSource
    })
  return { ...notify.value, channels, token_sources }
}

async function loadNotify() {
  const [data, presets] = await Promise.all([getNotifySettings(), getNotifyPresets()])
  notifyPresets.value = presets
  // Hydrate ephemeral editor fields for custom channels.
  for (const c of data.channels) {
    const ch = c as EditChannel
    if (ch.preset === 'custom') {
      ch._headers_json = JSON.stringify(ch.webhook?.headers || {}, null, 2)
    }
  }
  // Hydrate token-source header editors (custom token steps).
  for (const s of data.token_sources) {
    const src = s as EditSource
    src._headers_json = JSON.stringify(src.request?.headers || {}, null, 2)
  }
  notify.value = data
}

async function saveNotify() {
  notifySaving.value = true
  try {
    await updateNotifySettings(prepareNotifyPayload())
    await loadNotify() // re-mask secrets
  } catch {
    toast.error(t('settings.saveFailed'))
  } finally {
    notifySaving.value = false
  }
}

async function testChannel(ch: NotifyChannel) {
  notifyTesting.value = ch.name
  try {
    // Test runs against the saved config, so persist first.
    await updateNotifySettings(prepareNotifyPayload())
    await testNotifyChannel(ch.name)
    toast.success(t('settings.notify.testSent'))
    await loadNotify()
  } catch (e: any) {
    const msg = e?.response?.data?.message || t('settings.notify.testFailed')
    toast.error(msg)
  } finally {
    notifyTesting.value = null
  }
}

// API Keys
const apiKeys = ref<any[]>([])
const showCreateKeyDialog = ref(false)
const newKeyName = ref('')
const newKeyPermission = ref('standard')
const createdKey = ref<string | null>(null)
const keyCopied = ref(false)

onMounted(async () => {
  try {
    const [llmData, secData, agentData, chatData, webFetchData] = await Promise.all([
      getLLMSettings(),
      getSecuritySettings(),
      getAgentSettings(),
      getChatSettings(),
      getWebFetchSettings(),
    ])
    llm.value = llmData
    security.value = secData
    agent.value = agentData
    chat.value = chatData
    webFetch.value = webFetchData
    apiKeys.value = await getAPIKeys().catch(() => [])
    await loadNotify().catch(() => {})
  } catch {
    toast.error(t('settings.failedToLoad'))
  }
})

async function handleVerifyLLM() {
  verifying.value = true
  verifyResult.value = null
  try {
    // GET returns api_key masked as "abcd****wxyz" — only send it if the user
    // edited the field, otherwise let the backend use the stored value.
    const isMasked = /\*{4}/.test(llm.value.api_key || '')
    const res = await verifyLLM({
      api_base_url: llm.value.api_base_url,
      api_key: isMasked ? undefined : llm.value.api_key,
    })
    verifyResult.value = res
    if (res.models) {
      availableModels.value = res.models
    }
  } catch {
    verifyResult.value = { success: false, error: t('settings.llm.connectionFailed') }
  } finally {
    verifying.value = false
  }
}

async function saveLLM() {
  llmSaving.value = true
  try {
    await updateLLMSettings(llm.value)
  } catch {
    toast.error(t('settings.saveFailed'))
  } finally {
    llmSaving.value = false
  }
}

function addKeyword() {
  const val = keywordInput.value.trim()
  if (val && !security.value.sensitive_keywords.includes(val)) {
    security.value.sensitive_keywords.push(val)
    keywordInput.value = ''
  }
}

function removeKeyword(kw: string) {
  security.value.sensitive_keywords = security.value.sensitive_keywords.filter((k) => k !== kw)
}

function addBlacklist() {
  const val = blacklistInput.value.trim()
  if (val && !security.value.command_blacklist.includes(val)) {
    security.value.command_blacklist.push(val)
    blacklistInput.value = ''
  }
}

function removeBlacklist(cmd: string) {
  security.value.command_blacklist = security.value.command_blacklist.filter((c) => c !== cmd)
}

async function saveSecurity() {
  secSaving.value = true
  try {
    await updateSecuritySettings(security.value)
  } catch {
    toast.error(t('settings.saveFailed'))
  } finally {
    secSaving.value = false
  }
}

async function saveAgent() {
  agentSaving.value = true
  try {
    await updateAgentSettings(agent.value)
  } catch {
    toast.error(t('settings.saveFailed'))
  } finally {
    agentSaving.value = false
  }
}

async function saveChat() {
  chatSaving.value = true
  try {
    await updateChatSettings(chat.value)
  } catch {
    toast.error(t('settings.saveFailed'))
  } finally {
    chatSaving.value = false
  }
}

async function saveWebFetch() {
  webFetchSaving.value = true
  try {
    await updateWebFetchSettings(webFetch.value)
  } catch {
    toast.error(t('settings.saveFailed'))
  } finally {
    webFetchSaving.value = false
  }
}

async function handleVerifyWebFetch() {
  webFetchVerifying.value = true
  webFetchVerifyResult.value = null
  try {
    // GET returns jina_api_key masked as "jina_****abcd" — only send it if the
    // user edited the field, otherwise let the backend use the stored value.
    const isMasked = /\*{4}/.test(webFetch.value.jina_api_key || '')
    const res = await verifyWebFetch({
      mode: webFetch.value.mode,
      jina_api_key: isMasked ? undefined : webFetch.value.jina_api_key,
    })
    webFetchVerifyResult.value = res
  } catch {
    webFetchVerifyResult.value = { success: false, error: t('settings.webFetch.connectionFailed') }
  } finally {
    webFetchVerifying.value = false
  }
}

async function handleCreateKey() {
  if (!newKeyName.value.trim()) return
  try {
    const res = await createAPIKey({
      name: newKeyName.value.trim(),
      permission: newKeyPermission.value,
    })
    createdKey.value = res.key
    apiKeys.value = await getAPIKeys()
    newKeyName.value = ''
    newKeyPermission.value = 'standard'
  } catch {
    toast.error(t('settings.saveFailed'))
  }
}

async function handleRevokeKey(id: string) {
  try {
    await deleteAPIKey(id)
    apiKeys.value = await getAPIKeys()
  } catch {
    toast.error(t('settings.saveFailed'))
  }
}

function copyKey() {
  if (createdKey.value) {
    navigator.clipboard.writeText(createdKey.value)
    keyCopied.value = true
    setTimeout(() => { keyCopied.value = false }, 2000)
  }
}

function closeCreateDialog() {
  showCreateKeyDialog.value = false
  createdKey.value = null
}
</script>

<template>
  <div class="flex h-full" style="background-color: var(--background)">
    <!-- Left tabs -->
    <div class="flex w-[220px] flex-col border-r px-3 py-6">
      <h1 class="mb-4 px-3 text-lg font-semibold">{{ $t('settings.title') }}</h1>
      <nav class="flex flex-col gap-1">
        <button
          v-for="tab in tabs"
          :key="tab.id"
          class="rounded-lg px-3 py-2 text-left text-sm font-medium transition-colors"
          :style="{
            backgroundColor: activeTab === tab.id ? 'var(--secondary)' : 'transparent',
            color: activeTab === tab.id ? 'var(--foreground)' : 'var(--muted-foreground)',
          }"
          @click="activeTab = tab.id"
        >
          {{ tab.label }}
        </button>
      </nav>
    </div>

    <!-- Right content -->
    <div class="flex-1 overflow-auto p-6">
      <!-- LLM Config -->
      <div v-if="activeTab === 'llm'" class="max-w-2xl space-y-6">
        <div>
          <h2 class="text-base font-semibold">{{ $t('settings.llm.title') }}</h2>
          <p class="mt-1 text-sm" style="color: var(--muted-foreground)">
            {{ $t('settings.llm.description') }}
          </p>
        </div>

        <Separator />

        <div class="space-y-4">
          <div class="space-y-2">
            <label class="text-sm font-medium">{{ $t('settings.llm.apiBaseUrl') }}</label>
            <Input v-model="llm.api_base_url" placeholder="https://api.openai.com/v1" />
          </div>

          <div class="space-y-2">
            <label class="text-sm font-medium">{{ $t('settings.llm.apiKey') }}</label>
            <div class="flex gap-2">
              <Input v-model="llm.api_key" type="password" placeholder="sk-..." class="flex-1" />
              <Button variant="outline" :disabled="verifying" @click="handleVerifyLLM">
                <Loader2 v-if="verifying" class="mr-2 h-4 w-4 animate-spin" />
                {{ $t('common.verify') }}
              </Button>
            </div>
            <div v-if="verifyResult" class="flex items-center gap-2 text-sm">
              <template v-if="verifyResult.success">
                <CheckCircle class="h-4 w-4" style="color: var(--color-success-foreground)" />
                <span style="color: var(--color-success-foreground)">{{ $t('settings.llm.connectionVerified') }}</span>
              </template>
              <template v-else>
                <AlertCircle class="h-4 w-4" style="color: var(--color-error-foreground)" />
                <span style="color: var(--color-error-foreground)">{{ verifyResult.error }}</span>
              </template>
            </div>
          </div>

          <div class="space-y-2">
            <label class="text-sm font-medium">{{ $t('settings.llm.defaultModel') }}</label>
            <Select v-model="llm.default_model">
              <SelectTrigger>
                <SelectValue :placeholder="$t('settings.llm.selectModel')" />
              </SelectTrigger>
              <SelectContent>
                <SelectItem
                  v-for="model in modelOptions"
                  :key="model"
                  :value="model"
                >
                  {{ model }}
                </SelectItem>
              </SelectContent>
            </Select>
          </div>

          <div class="grid grid-cols-2 gap-4">
            <div class="space-y-2">
              <label class="text-sm font-medium">{{ $t('settings.llm.maxRounds') }}</label>
              <Input v-model.number="llm.max_rounds" type="number" :min="1" :max="50" />
            </div>
            <div class="space-y-2">
              <label class="text-sm font-medium">{{ $t('settings.llm.temperature') }}</label>
              <Input v-model.number="llm.temperature" type="number" :min="0" :max="2" step="0.1" />
            </div>
          </div>

          <div class="flex items-center justify-between">
            <div>
              <label class="text-sm font-medium">{{ $t('settings.llm.interleavedThinking') }}</label>
              <p class="text-sm" style="color: var(--muted-foreground)">
                {{ $t('settings.llm.interleavedThinkingDescription') }}
              </p>
            </div>
            <button
              class="relative h-6 w-11 shrink-0 rounded-full transition-colors"
              :style="{
                backgroundColor: llm.interleaved_thinking ? 'var(--primary)' : 'var(--secondary)',
              }"
              @click="llm.interleaved_thinking = !llm.interleaved_thinking"
            >
              <span
                class="absolute top-0.5 block h-5 w-5 rounded-full bg-white transition-transform"
                :class="llm.interleaved_thinking ? 'translate-x-5' : 'translate-x-0.5'"
              />
            </button>
          </div>
        </div>

        <Button :disabled="llmSaving" @click="saveLLM">
          {{ llmSaving ? $t('common.saving') : $t('common.save') }}
        </Button>
      </div>

      <!-- Security -->
      <div v-if="activeTab === 'security'" class="max-w-2xl space-y-6">
        <div>
          <h2 class="text-base font-semibold">{{ $t('settings.security.title') }}</h2>
          <p class="mt-1 text-sm" style="color: var(--muted-foreground)">
            {{ $t('settings.security.description') }}
          </p>
        </div>

        <Separator />

        <div class="space-y-6">
          <div class="flex items-center justify-between">
            <div>
              <label class="text-sm font-medium">{{ $t('settings.security.requireConfirmation') }}</label>
              <p class="text-sm" style="color: var(--muted-foreground)">
                {{ $t('settings.security.confirmDescription') }}
              </p>
            </div>
            <button
              class="relative h-6 w-11 rounded-full transition-colors"
              :style="{
                backgroundColor: security.confirm_enabled ? 'var(--primary)' : 'var(--secondary)',
              }"
              @click="security.confirm_enabled = !security.confirm_enabled"
            >
              <span
                class="absolute top-0.5 block h-5 w-5 rounded-full bg-white transition-transform"
                :class="security.confirm_enabled ? 'translate-x-5' : 'translate-x-0.5'"
              />
            </button>
          </div>

          <div class="space-y-2">
            <label class="text-sm font-medium">{{ $t('settings.security.sensitiveKeywords') }}</label>
            <div class="flex gap-2">
              <Input
                v-model="keywordInput"
                :placeholder="$t('settings.security.addKeyword')"
                class="flex-1"
                @keyup.enter="addKeyword"
              />
              <Button variant="outline" @click="addKeyword">{{ $t('common.add') }}</Button>
            </div>
            <div class="flex flex-wrap gap-2">
              <Badge
                v-for="kw in security.sensitive_keywords"
                :key="kw"
                variant="secondary"
                class="cursor-pointer"
                @click="removeKeyword(kw)"
              >
                {{ kw }} &times;
              </Badge>
            </div>
          </div>

          <div class="space-y-2">
            <label class="text-sm font-medium">{{ $t('settings.security.commandBlacklist') }}</label>
            <div class="flex gap-2">
              <Input
                v-model="blacklistInput"
                :placeholder="$t('settings.security.addCommand')"
                class="flex-1"
                @keyup.enter="addBlacklist"
              />
              <Button variant="outline" @click="addBlacklist">{{ $t('common.add') }}</Button>
            </div>
            <div class="flex flex-wrap gap-2">
              <Badge
                v-for="cmd in security.command_blacklist"
                :key="cmd"
                variant="secondary"
                class="cursor-pointer font-mono"
                @click="removeBlacklist(cmd)"
              >
                {{ cmd }} &times;
              </Badge>
            </div>
          </div>
        </div>

        <Button :disabled="secSaving" @click="saveSecurity">
          {{ secSaving ? $t('common.saving') : $t('common.save') }}
        </Button>
      </div>

      <!-- Node Agent -->
      <div v-if="activeTab === 'agent'" class="max-w-2xl space-y-6">
        <div>
          <h2 class="text-base font-semibold">{{ $t('settings.agent.title') }}</h2>
          <p class="mt-1 text-sm" style="color: var(--muted-foreground)">
            {{ $t('settings.agent.description') }}
          </p>
        </div>

        <Separator />

        <div class="space-y-4">
          <div class="space-y-2">
            <label class="text-sm font-medium">{{ $t('settings.agent.heartbeatInterval') }}</label>
            <Input v-model.number="agent.heartbeat_interval" type="number" :min="5" :max="300" />
          </div>

          <div class="space-y-2">
            <label class="text-sm font-medium">{{ $t('settings.agent.commandTimeout') }}</label>
            <Input v-model.number="agent.command_timeout" type="number" :min="5" :max="600" />
          </div>

          <div class="space-y-2">
            <label class="text-sm font-medium">{{ $t('settings.agent.outputMaxLines') }}</label>
            <Input v-model.number="agent.output_max_lines" type="number" :min="100" :max="10000" />
          </div>
        </div>

        <Button :disabled="agentSaving" @click="saveAgent">
          {{ agentSaving ? $t('common.saving') : $t('common.save') }}
        </Button>
      </div>

      <!-- Conversation -->
      <div v-if="activeTab === 'chat'" class="max-w-2xl space-y-6">
        <div>
          <h2 class="text-base font-semibold">{{ $t('settings.conversation.title') }}</h2>
          <p class="mt-1 text-sm" style="color: var(--muted-foreground)">
            {{ $t('settings.conversation.description') }}
          </p>
        </div>

        <Separator />

        <div class="space-y-4">
          <div class="space-y-2">
            <label class="text-sm font-medium">{{ $t('settings.conversation.contextRounds') }}</label>
            <Input v-model.number="chat.context_rounds" type="number" :min="1" :max="50" />
            <p class="text-xs" style="color: var(--muted-foreground)">
              {{ $t('settings.conversation.contextRoundsHelp') }}
            </p>
          </div>

          <div class="space-y-2">
            <label class="text-sm font-medium">{{ $t('settings.conversation.outputTruncateLines') }}</label>
            <Input v-model.number="chat.output_truncate_lines" type="number" :min="50" :max="5000" />
          </div>

          <div class="space-y-2">
            <label class="text-sm font-medium">{{ $t('settings.conversation.customSystemPrompt') }}</label>
            <Textarea
              v-model="chat.custom_system_prompt"
              :placeholder="$t('settings.conversation.systemPromptPlaceholder')"
              :rows="6"
            />
          </div>
        </div>

        <Button :disabled="chatSaving" @click="saveChat">
          {{ chatSaving ? $t('common.saving') : $t('common.save') }}
        </Button>
      </div>

      <!-- Web Fetch -->
      <div v-if="activeTab === 'web_fetch'" class="max-w-2xl space-y-6">
        <div>
          <h2 class="text-base font-semibold">{{ $t('settings.webFetch.title') }}</h2>
          <p class="mt-1 text-sm" style="color: var(--muted-foreground)">
            {{ $t('settings.webFetch.description') }}
          </p>
        </div>

        <Separator />

        <div class="space-y-4">
          <div class="space-y-2">
            <label class="text-sm font-medium">{{ $t('settings.webFetch.mode') }}</label>
            <Select v-model="webFetch.mode">
              <SelectTrigger>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="jina">Jina Reader API</SelectItem>
                <SelectItem value="local" disabled>
                  {{ $t('settings.webFetch.modeLocalLabel') }}
                </SelectItem>
              </SelectContent>
            </Select>
            <p class="text-xs" style="color: var(--muted-foreground)">
              {{ $t('settings.webFetch.modeJinaHint') }}
            </p>
          </div>

          <template v-if="webFetch.mode === 'jina'">
            <div class="space-y-2">
              <label class="text-sm font-medium">{{ $t('settings.webFetch.jinaApiKey') }}</label>
              <div class="flex gap-2">
                <Input
                  v-model="webFetch.jina_api_key"
                  type="password"
                  placeholder="jina_..."
                  class="flex-1"
                />
                <Button variant="outline" :disabled="webFetchVerifying" @click="handleVerifyWebFetch">
                  <Loader2 v-if="webFetchVerifying" class="mr-2 h-4 w-4 animate-spin" />
                  {{ $t('common.verify') }}
                </Button>
              </div>
              <p class="text-xs" style="color: var(--muted-foreground)">
                {{ $t('settings.webFetch.jinaKeyHint') }}
                <a
                  href="https://jina.ai/reader/#apiform"
                  target="_blank"
                  rel="noopener"
                  class="underline"
                >jina.ai/reader</a>
              </p>
              <div v-if="webFetchVerifyResult" class="flex items-center gap-2 text-sm">
                <template v-if="webFetchVerifyResult.success">
                  <CheckCircle class="h-4 w-4" style="color: var(--color-success-foreground)" />
                  <span style="color: var(--color-success-foreground)">
                    {{ $t('settings.webFetch.connectionVerified') }}
                  </span>
                </template>
                <template v-else>
                  <AlertCircle class="h-4 w-4" style="color: var(--color-error-foreground)" />
                  <span style="color: var(--color-error-foreground)">
                    {{ webFetchVerifyResult.error }}
                  </span>
                </template>
              </div>
            </div>
          </template>

          <div class="grid grid-cols-2 gap-4">
            <div class="space-y-2">
              <label class="text-sm font-medium">{{ $t('settings.webFetch.timeoutSec') }}</label>
              <Input v-model.number="webFetch.timeout_sec" type="number" :min="3" :max="120" />
            </div>
            <div class="space-y-2">
              <label class="text-sm font-medium">{{ $t('settings.webFetch.maxKb') }}</label>
              <Input v-model.number="webFetch.max_kb" type="number" :min="64" :max="4096" />
            </div>
          </div>
        </div>

        <Button :disabled="webFetchSaving" @click="saveWebFetch">
          {{ webFetchSaving ? $t('common.saving') : $t('common.save') }}
        </Button>
      </div>

      <!-- Notifications -->
      <div v-if="activeTab === 'notify'" class="max-w-2xl space-y-6">
        <div>
          <h2 class="text-base font-semibold">{{ $t('settings.notify.title') }}</h2>
          <p class="mt-1 text-sm" style="color: var(--muted-foreground)">
            {{ $t('settings.notify.description') }}
          </p>
        </div>

        <Separator />

        <!-- Global options -->
        <div class="space-y-4">
          <div class="grid grid-cols-2 gap-4">
            <div class="space-y-2">
              <label class="text-sm font-medium">{{ $t('settings.notify.thresholdLabel') }}</label>
              <Input v-model.number="notify.offline_threshold_seconds" type="number" :min="0" :max="3600" />
              <p class="text-xs" style="color: var(--muted-foreground)">{{ $t('settings.notify.thresholdHelp') }}</p>
            </div>
            <div class="space-y-2">
              <label class="text-sm font-medium">{{ $t('settings.notify.graceLabel') }}</label>
              <Input v-model.number="notify.startup_grace_seconds" type="number" :min="0" :max="3600" />
              <p class="text-xs" style="color: var(--muted-foreground)">{{ $t('settings.notify.graceHelp') }}</p>
            </div>
          </div>

          <div class="flex items-center justify-between">
            <div>
              <label class="text-sm font-medium">{{ $t('settings.notify.recoverLabel') }}</label>
              <p class="text-sm" style="color: var(--muted-foreground)">{{ $t('settings.notify.recoverHelp') }}</p>
            </div>
            <button
              class="relative h-6 w-11 shrink-0 rounded-full transition-colors"
              :style="{ backgroundColor: notify.recover_notify ? 'var(--primary)' : 'var(--secondary)' }"
              @click="notify.recover_notify = !notify.recover_notify"
            >
              <span
                class="absolute top-0.5 block h-5 w-5 rounded-full bg-white transition-transform"
                :class="notify.recover_notify ? 'translate-x-5' : 'translate-x-0.5'"
              />
            </button>
          </div>
        </div>

        <Separator />

        <!-- Channels -->
        <div class="space-y-4">
          <div class="flex items-center justify-between">
            <div>
              <h3 class="text-sm font-semibold">{{ $t('settings.notify.channels') }}</h3>
              <p class="text-xs" style="color: var(--muted-foreground)">{{ $t('settings.notify.channelsHelp') }}</p>
            </div>
            <Button variant="outline" size="sm" @click="addChannel">
              <Plus class="mr-1 h-4 w-4" />
              {{ $t('settings.notify.addChannel') }}
            </Button>
          </div>

          <p v-if="notify.channels.length === 0" class="text-sm" style="color: var(--muted-foreground)">
            {{ $t('settings.notify.noChannels') }}
          </p>

          <div
            v-for="(ch, idx) in notify.channels"
            :key="idx"
            class="space-y-3 rounded-lg border p-4"
          >
            <!-- Header row: enable toggle + name + preset + remove -->
            <div class="flex items-center gap-2">
              <button
                class="relative h-5 w-9 shrink-0 rounded-full transition-colors"
                :style="{ backgroundColor: ch.enabled ? 'var(--primary)' : 'var(--secondary)' }"
                :title="$t('settings.notify.enabled')"
                @click="ch.enabled = !ch.enabled"
              >
                <span
                  class="absolute top-0.5 block h-4 w-4 rounded-full bg-white transition-transform"
                  :class="ch.enabled ? 'translate-x-4' : 'translate-x-0.5'"
                />
              </button>
              <Input v-model="ch.name" :placeholder="$t('settings.notify.channelNamePlaceholder')" class="flex-1" />
              <Select :model-value="ch.preset" @update:model-value="(v: any) => { ch.preset = v; onPresetChange(ch as any) }">
                <SelectTrigger class="w-40">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem v-for="p in notifyPresets" :key="p.preset" :value="p.preset">
                    {{ p.label }}
                  </SelectItem>
                </SelectContent>
              </Select>
              <Button size="icon-sm" variant="ghost" @click="removeChannel(idx)">
                <Trash2 class="h-3.5 w-3.5" style="color: var(--color-error-foreground)" />
              </Button>
            </div>

            <!-- Preset params -->
            <div v-if="ch.preset !== 'custom'" class="space-y-3">
              <div v-if="(presetFor(ch.preset)?.params || []).length" class="space-y-2">
                <label class="text-xs font-medium" style="color: var(--muted-foreground)">
                  {{ $t('settings.notify.credentials') }}
                </label>
                <div
                  v-for="k in presetFor(ch.preset)?.params || []"
                  :key="k"
                  class="flex items-center gap-2"
                >
                  <span class="w-28 shrink-0 text-xs font-mono">{{ k }}</span>
                  <Input
                    v-model="ch.params![k]"
                    :type="isSecretField(k) ? 'password' : 'text'"
                    class="flex-1"
                  />
                </div>
              </div>

              <!-- Token-exchange step params -->
              <div v-if="(presetFor(ch.preset)?.source_params || []).length" class="space-y-2">
                <label class="text-xs font-medium" style="color: var(--muted-foreground)">
                  {{ $t('settings.notify.tokenStep') }}
                </label>
                <p class="text-xs" style="color: var(--muted-foreground)">{{ $t('settings.notify.tokenStepHelp') }}</p>
                <div
                  v-for="k in presetFor(ch.preset)?.source_params || []"
                  :key="k"
                  class="flex items-center gap-2"
                >
                  <span class="w-28 shrink-0 text-xs font-mono">{{ k }}</span>
                  <Input
                    v-model="sourceParams(ch)[k]"
                    :type="isSecretField(k) ? 'password' : 'text'"
                    class="flex-1"
                  />
                </div>
              </div>
            </div>

            <!-- Custom webhook -->
            <div v-else class="space-y-2">
              <div class="flex gap-2">
                <Select v-model="ch.webhook!.method">
                  <SelectTrigger class="w-28">
                    <SelectValue placeholder="POST" />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="POST">POST</SelectItem>
                    <SelectItem value="GET">GET</SelectItem>
                    <SelectItem value="PUT">PUT</SelectItem>
                  </SelectContent>
                </Select>
                <Input v-model="ch.webhook!.url" :placeholder="$t('settings.notify.custom.url')" class="flex-1" />
              </div>
              <label class="text-xs font-medium" style="color: var(--muted-foreground)">
                {{ $t('settings.notify.custom.headers') }}
              </label>
              <Textarea
                :model-value="(ch as any)._headers_json"
                :rows="3"
                class="font-mono text-xs"
                placeholder='{"Content-Type":"application/json"}'
                @update:model-value="(v: any) => { (ch as any)._headers_json = v; syncHeaders(ch as any) }"
              />
              <label class="text-xs font-medium" style="color: var(--muted-foreground)">
                {{ $t('settings.notify.custom.body') }}
              </label>
              <Textarea
                v-model="ch.webhook!.body_template"
                :rows="3"
                class="font-mono text-xs"
                placeholder='{"text":"{{message}}"}'
              />
              <p class="text-xs" style="color: var(--muted-foreground)">{{ $t('settings.notify.custom.bodyHelp') }}</p>

              <!-- Optional token-exchange step -->
              <div class="mt-2 rounded-md border border-dashed p-3">
                <div class="flex items-center justify-between">
                  <div>
                    <label class="text-xs font-medium">{{ $t('settings.notify.tokenStep') }}</label>
                    <p class="text-xs" style="color: var(--muted-foreground)">{{ $t('settings.notify.tokenStepHelp') }}</p>
                  </div>
                  <button
                    class="relative h-5 w-9 shrink-0 rounded-full transition-colors"
                    :style="{ backgroundColor: customTokenEnabled(ch) ? 'var(--primary)' : 'var(--secondary)' }"
                    @click="toggleCustomToken(ch as any)"
                  >
                    <span
                      class="absolute top-0.5 block h-4 w-4 rounded-full bg-white transition-transform"
                      :class="customTokenEnabled(ch) ? 'translate-x-4' : 'translate-x-0.5'"
                    />
                  </button>
                </div>

                <div v-if="customSource(ch)" class="mt-3 space-y-2">
                  <div class="flex gap-2">
                    <Select v-model="customSource(ch)!.request.method">
                      <SelectTrigger class="w-28">
                        <SelectValue placeholder="POST" />
                      </SelectTrigger>
                      <SelectContent>
                        <SelectItem value="POST">POST</SelectItem>
                        <SelectItem value="GET">GET</SelectItem>
                      </SelectContent>
                    </Select>
                    <Input
                      v-model="customSource(ch)!.request.url"
                      :placeholder="$t('settings.notify.tokenSource.url')"
                      class="flex-1"
                    />
                  </div>
                  <label class="text-xs font-medium" style="color: var(--muted-foreground)">
                    {{ $t('settings.notify.custom.headers') }}
                  </label>
                  <Textarea
                    :model-value="(customSource(ch) as any)._headers_json"
                    :rows="2"
                    class="font-mono text-xs"
                    placeholder='{"Content-Type":"application/json"}'
                    @update:model-value="(v: any) => { (customSource(ch) as any)._headers_json = v; syncSourceHeaders(customSource(ch)!) }"
                  />
                  <label class="text-xs font-medium" style="color: var(--muted-foreground)">
                    {{ $t('settings.notify.tokenSource.body') }}
                  </label>
                  <Textarea
                    v-model="customSource(ch)!.request.body"
                    :rows="2"
                    class="font-mono text-xs"
                    placeholder='{"app_id":"...","app_secret":"..."}'
                  />
                  <div class="grid grid-cols-3 gap-2">
                    <div class="space-y-1">
                      <label class="text-xs" style="color: var(--muted-foreground)">{{ $t('settings.notify.tokenSource.tokenPath') }}</label>
                      <Input v-model="customSource(ch)!.extract.token_path" placeholder="access_token" />
                    </div>
                    <div class="space-y-1">
                      <label class="text-xs" style="color: var(--muted-foreground)">{{ $t('settings.notify.tokenSource.expiresPath') }}</label>
                      <Input v-model="customSource(ch)!.extract.expires_path" placeholder="expires_in" />
                    </div>
                    <div class="space-y-1">
                      <label class="text-xs" style="color: var(--muted-foreground)">{{ $t('settings.notify.tokenSource.ttl') }}</label>
                      <Input v-model.number="customSource(ch)!.extract.ttl_fallback_seconds" type="number" :min="0" />
                    </div>
                  </div>
                  <div class="grid grid-cols-2 gap-2">
                    <div class="space-y-1">
                      <label class="text-xs" style="color: var(--muted-foreground)">{{ $t('settings.notify.tokenSource.invalidatePath') }}</label>
                      <Input :model-value="customSource(ch)!.invalidate_on?.json_path || ''" placeholder="errcode" @update:model-value="(v: any) => { const s = customSource(ch)!; s.invalidate_on = s.invalidate_on || { json_path: '', values: [] }; s.invalidate_on.json_path = v }" />
                    </div>
                    <div class="space-y-1">
                      <label class="text-xs" style="color: var(--muted-foreground)">{{ $t('settings.notify.tokenSource.invalidateValues') }}</label>
                      <Input :model-value="invalidateValuesText(customSource(ch)!)" placeholder="42001,40014" @update:model-value="(v: any) => setInvalidateValues(customSource(ch)!, v)" />
                    </div>
                  </div>
                  <p class="text-xs" style="color: var(--muted-foreground)">{{ $t('settings.notify.tokenSource.help') }}</p>
                </div>
              </div>
            </div>

            <!-- Per-channel test -->
            <div class="flex justify-end">
              <Button variant="outline" size="sm" :disabled="notifyTesting === ch.name" @click="testChannel(ch)">
                <Loader2 v-if="notifyTesting === ch.name" class="mr-1 h-4 w-4 animate-spin" />
                <Send v-else class="mr-1 h-4 w-4" />
                {{ $t('settings.notify.test') }}
              </Button>
            </div>
          </div>
        </div>

        <Button :disabled="notifySaving" @click="saveNotify">
          {{ notifySaving ? $t('common.saving') : $t('common.save') }}
        </Button>
      </div>

      <!-- API Keys -->
      <div v-if="activeTab === 'api_keys'" class="max-w-3xl space-y-6">
        <div class="flex items-center justify-between">
          <div>
            <h2 class="text-base font-semibold">{{ $t('settings.apiKeys.title') }}</h2>
            <p class="mt-1 text-sm" style="color: var(--muted-foreground)">
              {{ $t('settings.apiKeys.description') }}
            </p>
          </div>
          <Button @click="showCreateKeyDialog = true">
            <Key class="mr-2 h-4 w-4" />
            {{ $t('settings.apiKeys.createKey') }}
          </Button>
        </div>

        <Separator />

        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>{{ $t('common.name') }}</TableHead>
              <TableHead>{{ $t('settings.apiKeys.key') }}</TableHead>
              <TableHead>{{ $t('settings.apiKeys.permission') }}</TableHead>
              <TableHead>{{ $t('common.status') }}</TableHead>
              <TableHead>{{ $t('settings.apiKeys.lastUsed') }}</TableHead>
              <TableHead>{{ $t('common.actions') }}</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            <TableRow v-for="key in apiKeys" :key="key.id">
              <TableCell class="font-medium">{{ key.name }}</TableCell>
              <TableCell class="font-mono text-xs">{{ key.key_prefix }}...</TableCell>
              <TableCell><Badge variant="secondary">{{ key.permission }}</Badge></TableCell>
              <TableCell>
                <Badge :variant="key.status === 'active' ? 'default' : 'secondary'">
                  {{ key.status }}
                </Badge>
              </TableCell>
              <TableCell class="text-xs" style="color: var(--muted-foreground)">
                {{ key.last_used_at ? new Date(key.last_used_at).toLocaleDateString() : $t('common.never') }}
              </TableCell>
              <TableCell>
                <Button
                  v-if="key.status === 'active'"
                  size="icon-sm"
                  variant="ghost"
                  @click="handleRevokeKey(key.id)"
                >
                  <Trash2 class="h-3.5 w-3.5" style="color: var(--color-error-foreground)" />
                </Button>
              </TableCell>
            </TableRow>
            <TableRow v-if="apiKeys.length === 0">
              <TableCell :colspan="6" class="text-center py-8 text-sm" style="color: var(--muted-foreground)">
                {{ $t('settings.apiKeys.noKeys') }}
              </TableCell>
            </TableRow>
          </TableBody>
        </Table>
      </div>

      <!-- Create API Key Dialog -->
      <Dialog :open="showCreateKeyDialog" @update:open="closeCreateDialog">
        <DialogContent class="max-w-md">
          <DialogHeader>
            <DialogTitle>{{ createdKey ? $t('settings.apiKeys.keyCreated') : $t('settings.apiKeys.createKey') }}</DialogTitle>
          </DialogHeader>

          <template v-if="!createdKey">
            <div class="space-y-4 py-4">
              <div class="space-y-2">
                <label class="text-sm font-medium">{{ $t('common.name') }}</label>
                <Input v-model="newKeyName" :placeholder="$t('settings.apiKeys.namePlaceholder')" />
              </div>
              <div class="space-y-2">
                <label class="text-sm font-medium">{{ $t('settings.apiKeys.permission') }}</label>
                <Select v-model="newKeyPermission">
                  <SelectTrigger>
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="readonly">{{ $t('settings.apiKeys.readonly') }}</SelectItem>
                    <SelectItem value="standard">{{ $t('settings.apiKeys.standard') }}</SelectItem>
                    <SelectItem value="admin">{{ $t('settings.apiKeys.admin') }}</SelectItem>
                  </SelectContent>
                </Select>
                <p class="text-xs" style="color: var(--muted-foreground)">
                  {{ $t('settings.apiKeys.permissionHelp') }}
                </p>
              </div>
            </div>
            <DialogFooter>
              <Button variant="outline" @click="closeCreateDialog">{{ $t('common.cancel') }}</Button>
              <Button :disabled="!newKeyName.trim()" @click="handleCreateKey">{{ $t('common.create') }}</Button>
            </DialogFooter>
          </template>

          <template v-else>
            <div class="space-y-4 py-4">
              <div
                class="rounded-lg p-4"
                style="background-color: var(--color-warning); border: 1px solid var(--color-warning-foreground)"
              >
                <p class="text-sm font-medium mb-2" style="color: var(--color-warning-foreground)">
                  {{ $t('settings.apiKeys.copyWarning') }}
                </p>
                <div class="flex items-center gap-2">
                  <code class="flex-1 rounded p-2 text-xs font-mono break-all" style="background-color: var(--secondary)">
                    {{ createdKey }}
                  </code>
                  <Button size="icon" variant="outline" @click="copyKey">
                    <Check v-if="keyCopied" class="h-4 w-4" style="color: var(--color-success-foreground)" />
                    <Copy v-else class="h-4 w-4" />
                  </Button>
                </div>
              </div>
            </div>
            <DialogFooter>
              <Button @click="closeCreateDialog">{{ $t('common.done') }}</Button>
            </DialogFooter>
          </template>
        </DialogContent>
      </Dialog>
    </div>
  </div>
</template>
