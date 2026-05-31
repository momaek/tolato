<script setup lang="ts">
import { ref, computed, watchEffect } from 'vue'
import { useRoute } from 'vue-router'
import { Terminal, CheckCircle, XCircle, Loader2, ChevronRight } from 'lucide-vue-next'
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from '@/components/ui/collapsible'
import { getToolCallOutput } from '@/services/api'
import type { ToolCallItem, ToolCallOutputResponse } from '@/types/api'

const props = defineProps<{
  toolCall: ToolCallItem
}>()

const isOpen = ref(false)

const route = useRoute()
const conversationId = computed(() => route.params.conversationId as string | undefined)

// Full output fetched lazily when the user clicks "view all". Stays null while
// the head preview from the conversation GET is enough (or for live results,
// which arrive complete via WebSocket).
const fullOutput = ref<ToolCallOutputResponse | null>(null)
const loadingFull = ref(false)
const loadFailed = ref(false)

const countLines = (s?: string | null) => (s ? s.split('\n').length : 0)

const stdoutText = computed(() => fullOutput.value?.stdout ?? props.toolCall.result?.stdout)
const stderrText = computed(() => fullOutput.value?.stderr ?? props.toolCall.result?.stderr)

// Preview is partial and the full output hasn't been fetched yet.
const isTruncated = computed(() => !!props.toolCall.result?.truncated && !fullOutput.value)

const totalLines = computed(
  () => (props.toolCall.result?.stdout_lines ?? 0) + (props.toolCall.result?.stderr_lines ?? 0)
)
const shownLines = computed(
  () => countLines(props.toolCall.result?.stdout) + countLines(props.toolCall.result?.stderr)
)

async function loadFull() {
  if (loadingFull.value || !conversationId.value) return
  loadingFull.value = true
  loadFailed.value = false
  try {
    fullOutput.value = await getToolCallOutput(conversationId.value, props.toolCall.id)
  } catch {
    loadFailed.value = true
  } finally {
    loadingFull.value = false
  }
}

const status = computed(() => {
  if (!props.toolCall.result) return 'executing'
  const r = props.toolCall.result
  if (r.exit_code !== undefined && r.exit_code !== null && r.exit_code !== 0) return 'error'
  if (r.data && typeof r.data === 'object' && 'error' in (r.data as any)) return 'error'
  return 'success'
})

const isError = computed(() => status.value === 'error')

// Auto-expand errors reactively
watchEffect(() => {
  if (isError.value) {
    isOpen.value = true
  }
})

const commandStr = computed(() => {
  if (props.toolCall.tool === 'execute_command') {
    return props.toolCall.args?.command as string || ''
  }
  return JSON.stringify(props.toolCall.args || {})
})

const editPreview = computed(() => {
  if (props.toolCall.tool !== 'edit_node_info') return ''
  const args = props.toolCall.args || {}
  const parts: string[] = []
  if (typeof args.alias === 'string') parts.push(`alias=${args.alias || '∅'}`)
  if (args.extra && typeof args.extra === 'object') {
    parts.push(...Object.keys(args.extra as object).map((k) => `${k}=${(args.extra as Record<string, unknown>)[k]}`))
  }
  return parts.join(' · ')
})

const borderClass = 'border-l-[3px]'

const borderColor = computed(() => {
  if (status.value === 'executing') return 'var(--primary)'
  if (status.value === 'error') return 'var(--color-error-foreground)'
  return 'var(--border)'
})
</script>

<template>
  <Collapsible v-model:open="isOpen" class="my-2">
    <CollapsibleTrigger
      class="flex w-full items-center gap-2 rounded-lg p-3 text-xs cursor-pointer"
      :class="borderClass"
      :style="{
        borderLeftColor: borderColor,
        backgroundColor: 'var(--card)',
      }"
    >
      <Loader2 v-if="status === 'executing'" class="h-3.5 w-3.5 animate-spin" style="color: var(--primary)" />
      <CheckCircle v-else-if="status === 'success'" class="h-3.5 w-3.5" style="color: var(--color-success-foreground)" />
      <XCircle v-else class="h-3.5 w-3.5" style="color: var(--color-error-foreground)" />

      <Terminal class="h-3.5 w-3.5" style="color: var(--muted-foreground)" />
      <span class="font-mono" style="color: var(--foreground)">{{ toolCall.tool }}</span>

      <span v-if="toolCall.tool === 'execute_command'" class="truncate font-mono opacity-60">
        {{ commandStr }}
      </span>
      <span v-else-if="toolCall.tool === 'edit_node_info'" class="truncate opacity-60">
        {{ editPreview }}
      </span>

      <template v-if="toolCall.result?.duration_ms">
        <span class="ml-auto opacity-50">{{ toolCall.result.duration_ms }}ms</span>
      </template>

      <ChevronRight class="h-3 w-3 transition-transform ml-1" :class="{ 'rotate-90': isOpen }" />
    </CollapsibleTrigger>

    <CollapsibleContent>
      <div class="mt-1 rounded-lg text-xs font-mono" style="background-color: var(--secondary)">
        <template v-if="toolCall.result">
          <div class="overflow-auto p-3" style="max-height: min(400px, 50vh)">
            <div v-if="stdoutText" class="whitespace-pre-wrap break-all" style="color: var(--foreground)">
              {{ stdoutText }}
            </div>
            <div v-if="stderrText" class="whitespace-pre-wrap break-all mt-2" style="color: var(--color-error-foreground)">
              {{ stderrText }}
            </div>
            <div v-if="toolCall.result.data" class="whitespace-pre-wrap break-all" style="color: var(--foreground)">
              {{ typeof toolCall.result.data === 'string' ? toolCall.result.data : JSON.stringify(toolCall.result.data, null, 2) }}
            </div>
            <div
              v-if="!stdoutText && !stderrText && !toolCall.result.data"
              class="opacity-50"
              style="color: var(--muted-foreground)"
            >
              {{ $t('chat.noOutput') }}
            </div>
          </div>

          <!-- Truncated preview: offer to fetch the full output on demand. -->
          <div
            v-if="isTruncated"
            class="flex items-center gap-2 border-t px-3 py-2"
            style="border-color: var(--border); color: var(--muted-foreground)"
          >
            <span>{{ $t('chat.outputTruncated', { shown: shownLines, total: totalLines }) }}</span>
            <button
              v-if="!loadingFull"
              type="button"
              class="underline hover:opacity-80 cursor-pointer"
              style="color: var(--primary)"
              @click="loadFull"
            >
              {{ loadFailed ? $t('chat.retry') : $t('chat.viewFullOutput') }}
            </button>
            <span v-else class="flex items-center gap-1">
              <Loader2 class="h-3 w-3 animate-spin" />
              {{ $t('chat.loadingOutput') }}
            </span>
            <span v-if="loadFailed && !loadingFull" style="color: var(--color-error-foreground)">
              {{ $t('chat.loadOutputFailed') }}
            </span>
          </div>
        </template>
        <div v-else class="flex items-center gap-2 p-3" style="color: var(--muted-foreground)">
          <Loader2 class="h-3 w-3 animate-spin" />
          {{ $t('chat.executing') }}
        </div>
      </div>
    </CollapsibleContent>
  </Collapsible>
</template>
