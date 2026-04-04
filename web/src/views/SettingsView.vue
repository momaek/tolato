<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { CheckCircle, AlertCircle, Loader2 } from 'lucide-vue-next'
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
  getLLMSettings,
  updateLLMSettings,
  verifyLLM,
  getSecuritySettings,
  updateSecuritySettings,
  getAgentSettings,
  updateAgentSettings,
  getChatSettings,
  updateChatSettings,
} from '@/services/api'
import type {
  LLMSettings,
  SecuritySettings,
  AgentSettings,
  ChatSettings,
  VerifyLLMResponse,
} from '@/types/api'

const activeTab = ref('llm')

const tabs = [
  { id: 'llm', label: 'LLM Config' },
  { id: 'security', label: 'Security' },
  { id: 'agent', label: 'Node Agent' },
  { id: 'chat', label: 'Conversation' },
]

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
  } catch {
    // TODO: toast error
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
    verifyResult.value = { success: false, error: 'Connection failed' }
  } finally {
    verifying.value = false
  }
}

async function saveLLM() {
  llmSaving.value = true
  try {
    await updateLLMSettings(llm.value)
  } catch {
    // TODO: toast
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
    // TODO: toast
  } finally {
    secSaving.value = false
  }
}

async function saveAgent() {
  agentSaving.value = true
  try {
    await updateAgentSettings(agent.value)
  } catch {
    // TODO: toast
  } finally {
    agentSaving.value = false
  }
}

async function saveChat() {
  chatSaving.value = true
  try {
    await updateChatSettings(chat.value)
  } catch {
    // TODO: toast
  } finally {
    chatSaving.value = false
  }
}
</script>

<template>
  <div class="flex h-full" style="background-color: var(--background)">
    <!-- Left tabs -->
    <div class="flex w-[220px] flex-col border-r px-3 py-6">
      <h1 class="mb-4 px-3 text-lg font-semibold">Settings</h1>
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
          <h2 class="text-base font-semibold">LLM Configuration</h2>
          <p class="mt-1 text-sm" style="color: var(--muted-foreground)">
            Configure the AI model provider for your assistant.
          </p>
        </div>

        <Separator />

        <div class="space-y-4">
          <div class="space-y-2">
            <label class="text-sm font-medium">API Base URL</label>
            <Input v-model="llm.api_base_url" placeholder="https://api.openai.com/v1" />
          </div>

          <div class="space-y-2">
            <label class="text-sm font-medium">API Key</label>
            <div class="flex gap-2">
              <Input v-model="llm.api_key" type="password" placeholder="sk-..." class="flex-1" />
              <Button variant="outline" :disabled="verifying" @click="handleVerifyLLM">
                <Loader2 v-if="verifying" class="mr-2 h-4 w-4 animate-spin" />
                Verify
              </Button>
            </div>
            <div v-if="verifyResult" class="flex items-center gap-2 text-sm">
              <template v-if="verifyResult.success">
                <CheckCircle class="h-4 w-4" style="color: var(--color-success-foreground)" />
                <span style="color: var(--color-success-foreground)">Connection verified</span>
              </template>
              <template v-else>
                <AlertCircle class="h-4 w-4" style="color: var(--color-error-foreground)" />
                <span style="color: var(--color-error-foreground)">{{ verifyResult.error }}</span>
              </template>
            </div>
          </div>

          <div class="space-y-2">
            <label class="text-sm font-medium">Default Model</label>
            <Select v-model="llm.default_model">
              <SelectTrigger>
                <SelectValue placeholder="Select a model" />
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
              <label class="text-sm font-medium">Max Rounds</label>
              <Input v-model.number="llm.max_rounds" type="number" :min="1" :max="50" />
            </div>
            <div class="space-y-2">
              <label class="text-sm font-medium">Temperature</label>
              <Input v-model.number="llm.temperature" type="number" :min="0" :max="2" step="0.1" />
            </div>
          </div>
        </div>

        <Button :disabled="llmSaving" @click="saveLLM">
          {{ llmSaving ? 'Saving...' : 'Save Changes' }}
        </Button>
      </div>

      <!-- Security -->
      <div v-if="activeTab === 'security'" class="max-w-2xl space-y-6">
        <div>
          <h2 class="text-base font-semibold">Security Settings</h2>
          <p class="mt-1 text-sm" style="color: var(--muted-foreground)">
            Configure confirmation requirements and command restrictions.
          </p>
        </div>

        <Separator />

        <div class="space-y-6">
          <div class="flex items-center justify-between">
            <div>
              <label class="text-sm font-medium">Require Confirmation</label>
              <p class="text-sm" style="color: var(--muted-foreground)">
                Ask for confirmation before executing sensitive commands.
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
            <label class="text-sm font-medium">Sensitive Keywords</label>
            <div class="flex gap-2">
              <Input
                v-model="keywordInput"
                placeholder="Add keyword..."
                class="flex-1"
                @keyup.enter="addKeyword"
              />
              <Button variant="outline" @click="addKeyword">Add</Button>
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
            <label class="text-sm font-medium">Command Blacklist</label>
            <div class="flex gap-2">
              <Input
                v-model="blacklistInput"
                placeholder="Add command..."
                class="flex-1"
                @keyup.enter="addBlacklist"
              />
              <Button variant="outline" @click="addBlacklist">Add</Button>
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
          {{ secSaving ? 'Saving...' : 'Save Changes' }}
        </Button>
      </div>

      <!-- Node Agent -->
      <div v-if="activeTab === 'agent'" class="max-w-2xl space-y-6">
        <div>
          <h2 class="text-base font-semibold">Node Agent Settings</h2>
          <p class="mt-1 text-sm" style="color: var(--muted-foreground)">
            Configure agent behavior on managed nodes.
          </p>
        </div>

        <Separator />

        <div class="space-y-4">
          <div class="space-y-2">
            <label class="text-sm font-medium">Heartbeat Interval (seconds)</label>
            <Input v-model.number="agent.heartbeat_interval" type="number" :min="5" :max="300" />
          </div>

          <div class="space-y-2">
            <label class="text-sm font-medium">Command Timeout (seconds)</label>
            <Input v-model.number="agent.command_timeout" type="number" :min="5" :max="600" />
          </div>

          <div class="space-y-2">
            <label class="text-sm font-medium">Output Max Lines</label>
            <Input v-model.number="agent.output_max_lines" type="number" :min="100" :max="10000" />
          </div>
        </div>

        <Button :disabled="agentSaving" @click="saveAgent">
          {{ agentSaving ? 'Saving...' : 'Save Changes' }}
        </Button>
      </div>

      <!-- Conversation -->
      <div v-if="activeTab === 'chat'" class="max-w-2xl space-y-6">
        <div>
          <h2 class="text-base font-semibold">Conversation Settings</h2>
          <p class="mt-1 text-sm" style="color: var(--muted-foreground)">
            Configure chat behavior and system prompt.
          </p>
        </div>

        <Separator />

        <div class="space-y-4">
          <div class="space-y-2">
            <label class="text-sm font-medium">Context Rounds</label>
            <Input v-model.number="chat.context_rounds" type="number" :min="1" :max="50" />
            <p class="text-xs" style="color: var(--muted-foreground)">
              Number of previous message rounds to include as context.
            </p>
          </div>

          <div class="space-y-2">
            <label class="text-sm font-medium">Output Truncate Lines</label>
            <Input v-model.number="chat.output_truncate_lines" type="number" :min="50" :max="5000" />
          </div>

          <div class="space-y-2">
            <label class="text-sm font-medium">Custom System Prompt</label>
            <Textarea
              v-model="chat.custom_system_prompt"
              placeholder="Enter a custom system prompt for the AI assistant..."
              :rows="6"
            />
          </div>
        </div>

        <Button :disabled="chatSaving" @click="saveChat">
          {{ chatSaving ? 'Saving...' : 'Save Changes' }}
        </Button>
      </div>
    </div>
  </div>
</template>
