<script setup lang="ts">
import { ref, onMounted, computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { toast } from 'vue-sonner'
import { CheckCircle, AlertCircle, Loader2, Copy, Check, Key, Trash2 } from 'lucide-vue-next'
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
  getAPIKeys,
  createAPIKey,
  deleteAPIKey,
} from '@/services/api'
import type {
  LLMSettings,
  SecuritySettings,
  AgentSettings,
  ChatSettings,
  VerifyLLMResponse,
} from '@/types/api'

const { t } = useI18n()
const activeTab = ref('llm')

const tabs = computed(() => [
  { id: 'llm', label: t('settings.tabs.llm') },
  { id: 'security', label: t('settings.tabs.security') },
  { id: 'agent', label: t('settings.tabs.agent') },
  { id: 'chat', label: t('settings.tabs.chat') },
  { id: 'api_keys', label: t('settings.tabs.apiKeys') },
])

// LLM
const llm = ref<LLMSettings>({
  api_base_url: '',
  api_key: '',
  default_model: '',
  max_rounds: 10,
  temperature: 0.7,
})
const verifyResult = ref<VerifyLLMResponse | null>(null)
const verifying = ref(false)
const availableModels = ref<string[]>([])
const llmSaving = ref(false)

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

// API Keys
const apiKeys = ref<any[]>([])
const showCreateKeyDialog = ref(false)
const newKeyName = ref('')
const newKeyPermission = ref('standard')
const createdKey = ref<string | null>(null)
const keyCopied = ref(false)

onMounted(async () => {
  try {
    const [llmData, secData, agentData, chatData] = await Promise.all([
      getLLMSettings(),
      getSecuritySettings(),
      getAgentSettings(),
      getChatSettings(),
    ])
    llm.value = llmData
    security.value = secData
    agent.value = agentData
    chat.value = chatData
    apiKeys.value = await getAPIKeys().catch(() => [])
  } catch {
    toast.error(t('settings.failedToLoad'))
  }
})

async function handleVerifyLLM() {
  verifying.value = true
  verifyResult.value = null
  try {
    const res = await verifyLLM()
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
                  v-for="model in availableModels.length ? availableModels : ['gpt-4o', 'gpt-4o-mini', 'claude-3.5-sonnet']"
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
