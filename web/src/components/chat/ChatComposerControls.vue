<script setup lang="ts">
import { ref, computed, watch } from 'vue'
import { Brain, Server } from 'lucide-vue-next'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { updateConversation, getNodes, getLLMSettings, getLLMModels } from '@/services/api'
import type { NodeListItem } from '@/types/api'

const props = defineProps<{
  conversationId?: string
  model: string
  defaultNodeId?: string
}>()

const emit = defineEmits<{
  (e: 'update:model', value: string): void
  (e: 'update:defaultNodeId', value: string | undefined): void
}>()

const selectedModel = ref(props.model || '')
const selectedNode = ref(props.defaultNodeId || 'all')
const nodes = ref<NodeListItem[]>([])
const models = ref<string[]>([])
const defaultModel = ref('')

watch(() => props.model, (v) => { selectedModel.value = v || defaultModel.value })
watch(() => props.defaultNodeId, (v) => { selectedNode.value = v || 'all' })

const validNodes = computed(() => nodes.value.filter(x => x.id))

const modelOptions = computed(() => {
  const set = new Set(models.value)
  if (selectedModel.value) set.add(selectedModel.value)
  if (defaultModel.value) set.add(defaultModel.value)
  return Array.from(set)
})

getNodes().then((n) => { nodes.value = Array.isArray(n) ? n : [] }).catch(() => {})

getLLMSettings().then((s) => {
  defaultModel.value = s.default_model || ''
  if (!selectedModel.value && defaultModel.value) {
    selectedModel.value = defaultModel.value
    emit('update:model', defaultModel.value)
  }
}).catch(() => {})

getLLMModels().then((list) => { models.value = list }).catch(() => {})

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

const pickerClass = 'h-7 gap-1.5 rounded-md border-transparent bg-transparent px-2 text-xs font-normal hover:bg-[color-mix(in_oklab,var(--foreground)_8%,transparent)]'
</script>

<template>
  <div class="flex items-center gap-1.5">
    <Select :model-value="selectedModel" @update:model-value="onModelChange">
      <SelectTrigger size="sm" :class="pickerClass">
        <Brain class="h-3.5 w-3.5" style="color: var(--muted-foreground)" />
        <SelectValue :placeholder="$t('chat.selectModel')" />
      </SelectTrigger>
      <SelectContent>
        <SelectItem v-for="m in modelOptions" :key="m" :value="m">{{ m }}</SelectItem>
      </SelectContent>
    </Select>

    <Select :model-value="selectedNode" @update:model-value="onNodeChange">
      <SelectTrigger size="sm" :class="pickerClass">
        <Server class="h-3.5 w-3.5" style="color: var(--muted-foreground)" />
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
