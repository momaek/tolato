<script setup lang="ts">
import { ref, computed, watch } from 'vue'
import { useI18n } from 'vue-i18n'
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

const { t } = useI18n()

const selectedModel = ref(props.model || '')
const selectedNode = ref(props.defaultNodeId || 'all')
const nodes = ref<NodeListItem[]>([])
const models = ref<string[]>([])
const defaultModel = ref('')

watch(() => props.model, (v) => { selectedModel.value = v || defaultModel.value })
watch(() => props.defaultNodeId, (v) => { selectedNode.value = v || 'all' })

const validNodes = computed(() => {
  const list = nodes.value.filter(x => x.id)
  return [...list].sort((a, b) => {
    const ao = a.status === 'online' ? 0 : 1
    const bo = b.status === 'online' ? 0 : 1
    if (ao !== bo) return ao - bo
    return (a.alias || a.name || '').localeCompare(b.alias || b.name || '')
  })
})

function flagEmoji(code?: string): string {
  if (!code || code.length !== 2) return ''
  const A = 0x1f1e6
  const c0 = code.toUpperCase().charCodeAt(0)
  const c1 = code.toUpperCase().charCodeAt(1)
  if (c0 < 65 || c0 > 90 || c1 < 65 || c1 > 90) return ''
  return String.fromCodePoint(A + c0 - 65, A + c1 - 65)
}

type ExpiryInfo = { text: string; color: string } | null

function expiryInfo(node: NodeListItem): ExpiryInfo {
  const raw = node.extra?.expires_at as string | undefined
  if (!raw) return null
  const target = new Date(raw)
  if (isNaN(target.getTime())) return null
  const startOfDay = (d: Date) => new Date(d.getFullYear(), d.getMonth(), d.getDate())
  const days = Math.round((startOfDay(target).getTime() - startOfDay(new Date()).getTime()) / 86_400_000)
  if (days < 0) return { text: t('nodes.expiredAgo', { days: -days }), color: 'var(--color-error-foreground)' }
  if (days === 0) return { text: t('nodes.expiresToday'), color: 'var(--color-error-foreground)' }
  if (days <= 7) return { text: t('nodes.expiresIn', { days }), color: 'var(--color-error-foreground)' }
  if (days <= 30) return { text: t('nodes.expiresIn', { days }), color: 'var(--color-warning-foreground)' }
  return null
}

function nodeSecondary(node: NodeListItem): string {
  const parts: string[] = []
  const flag = flagEmoji(node.country_code)
  const region = [flag, node.city].filter(Boolean).join(' ')
  if (region) parts.push(region)
  if (node.ip) parts.push(node.ip)
  return parts.join(' · ')
}

const selectedNodeLabel = computed(() => {
  if (selectedNode.value === 'all') return t('chat.allNodes')
  const n = validNodes.value.find(x => x.id === selectedNode.value)
  if (!n) return t('chat.defaultNode')
  const flag = flagEmoji(n.country_code)
  const name = n.alias || n.name
  return flag ? `${flag} ${name}` : name
})

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
        <span class="truncate">{{ selectedNodeLabel }}</span>
      </SelectTrigger>
      <SelectContent class="min-w-[260px]">
        <SelectItem value="all">{{ $t('chat.allNodes') }}</SelectItem>
        <SelectItem
          v-for="n in validNodes"
          :key="n.id"
          :value="n.id"
          class="py-2"
        >
          <div class="flex w-full min-w-0 items-start gap-2">
            <span
              class="mt-1 h-2 w-2 shrink-0 rounded-full"
              :style="{
                backgroundColor: n.status === 'online' ? 'var(--color-success-foreground)' : 'var(--muted-foreground)',
              }"
              :title="n.status === 'online' ? $t('common.online') : $t('common.offline')"
            />
            <div class="flex min-w-0 flex-1 flex-col">
              <div class="flex items-center gap-2">
                <span class="truncate text-sm">{{ n.alias || n.name }}</span>
                <span
                  v-if="expiryInfo(n)"
                  class="shrink-0 text-[10px]"
                  :style="{ color: expiryInfo(n)!.color }"
                >
                  {{ expiryInfo(n)!.text }}
                </span>
              </div>
              <span
                v-if="nodeSecondary(n)"
                class="truncate text-xs"
                style="color: var(--muted-foreground)"
              >
                {{ nodeSecondary(n) }}
              </span>
            </div>
          </div>
        </SelectItem>
      </SelectContent>
    </Select>
  </div>
</template>
