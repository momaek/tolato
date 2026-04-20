<script setup lang="ts">
import { ref, computed, watch } from 'vue'
import { Bot, Check, Pencil } from 'lucide-vue-next'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Button } from '@/components/ui/button'
import { updateConversation, getNodes, getLLMSettings, getLLMModels } from '@/services/api'
import type { NodeListItem } from '@/types/api'

const props = defineProps<{
  conversationId?: string
  title: string
  model: string
  defaultNodeId?: string
}>()

const emit = defineEmits<{
  (e: 'update:model', value: string): void
  (e: 'update:defaultNodeId', value: string | undefined): void
}>()

const isEditing = ref(false)
const editTitle = ref(props.title)
const selectedModel = ref(props.model || '')
const selectedNode = ref(props.defaultNodeId || 'all')
const nodes = ref<NodeListItem[]>([])
const models = ref<string[]>([])
const defaultModel = ref('')

watch(() => props.title, (v) => { editTitle.value = v })
watch(() => props.model, (v) => { selectedModel.value = v || defaultModel.value })
watch(() => props.defaultNodeId, (v) => { selectedNode.value = v || 'all' })

const validNodes = computed(() => nodes.value.filter(x => x.id))

// Ensure the currently selected model is always renderable in the dropdown,
// even if it's not in the fetched list (e.g. stale config or fetch failed).
const modelOptions = computed(() => {
  const set = new Set(models.value)
  if (selectedModel.value) set.add(selectedModel.value)
  if (defaultModel.value) set.add(defaultModel.value)
  return Array.from(set)
})

// Load nodes for selector
getNodes().then((n) => { nodes.value = Array.isArray(n) ? n : [] }).catch(() => {})

// Load configured default model + available models from the LLM API.
getLLMSettings().then((s) => {
  defaultModel.value = s.default_model || ''
  if (!selectedModel.value && defaultModel.value) {
    selectedModel.value = defaultModel.value
    emit('update:model', defaultModel.value)
  }
}).catch(() => {})

// Read the cached model list (populated by SettingsView's verify action).
// Avoids hitting the upstream LLM API on every chat page open.
getLLMModels().then((list) => { models.value = list }).catch(() => {})

async function saveTitle() {
  if (props.conversationId && editTitle.value !== props.title) {
    await updateConversation(props.conversationId, { title: editTitle.value }).catch(() => {})
  }
  isEditing.value = false
}

function onModelChange(val: any) {
  const v = String(val)
  selectedModel.value = v
  emit('update:model', v)
  if (props.conversationId) {
    updateConversation(props.conversationId, { model: v }).catch(() => {})
  }
}

function onNodeChange(val: any) {
  const v = String(val)
  selectedNode.value = v
  emit('update:defaultNodeId', v === 'all' ? undefined : v)
}
</script>

<template>
  <div class="flex items-center gap-3 border-b px-5 py-3">
    <div class="flex items-center gap-2">
      <Bot class="h-4 w-4" style="color: var(--muted-foreground)" />
      <template v-if="isEditing">
        <Input
          v-model="editTitle"
          class="h-7 w-48 text-sm"
          @keyup.enter="saveTitle"
          @blur="saveTitle"
        />
        <Button size="icon-sm" variant="ghost" @click="saveTitle">
          <Check class="h-3 w-3" />
        </Button>
      </template>
      <template v-else>
        <span class="text-sm font-medium cursor-pointer" @click="isEditing = true">
          {{ title || $t('chat.newConversation') }}
        </span>
        <Pencil
          class="h-3 w-3 cursor-pointer opacity-50 hover:opacity-100"
          @click="isEditing = true"
        />
      </template>
    </div>
    <div class="flex-1" />
    <Select :model-value="selectedModel" @update:model-value="onModelChange">
      <SelectTrigger class="w-[160px]">
        <SelectValue :placeholder="$t('chat.selectModel')" />
      </SelectTrigger>
      <SelectContent>
        <SelectItem v-for="m in modelOptions" :key="m" :value="m">{{ m }}</SelectItem>
      </SelectContent>
    </Select>
    <Select :model-value="selectedNode" @update:model-value="onNodeChange">
      <SelectTrigger class="w-[160px]">
        <SelectValue :placeholder="$t('chat.defaultNode')" />
      </SelectTrigger>
      <SelectContent>
        <SelectItem value="all">{{ $t('chat.allNodes') }}</SelectItem>
        <SelectItem v-for="n in validNodes" :key="n.id" :value="n.id">
          {{ n.alias || n.name }}
        </SelectItem>
      </SelectContent>
    </Select>
  </div>
</template>
