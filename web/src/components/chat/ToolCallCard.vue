<script setup lang="ts">
import { ref, computed, watchEffect } from 'vue'
import { Terminal, CheckCircle, XCircle, Loader2, ChevronRight } from 'lucide-vue-next'
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from '@/components/ui/collapsible'
import type { ToolCallItem } from '@/types/api'

const props = defineProps<{
  toolCall: ToolCallItem
}>()

const isOpen = ref(false)

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

const borderClass = 'border-l-[3px]'

const borderColor = computed(() => {
  if (status.value === 'executing') return 'var(--color-warning-foreground)'
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
      <Loader2 v-if="status === 'executing'" class="h-3.5 w-3.5 animate-spin" style="color: var(--color-warning-foreground)" />
      <CheckCircle v-else-if="status === 'success'" class="h-3.5 w-3.5" style="color: var(--color-success-foreground)" />
      <XCircle v-else class="h-3.5 w-3.5" style="color: var(--color-error-foreground)" />

      <Terminal class="h-3.5 w-3.5" style="color: var(--muted-foreground)" />
      <span class="font-mono" style="color: var(--foreground)">{{ toolCall.tool }}</span>

      <span v-if="toolCall.tool === 'execute_command'" class="truncate font-mono opacity-60">
        {{ commandStr }}
      </span>

      <template v-if="toolCall.result?.duration_ms">
        <span class="ml-auto opacity-50">{{ toolCall.result.duration_ms }}ms</span>
      </template>

      <ChevronRight class="h-3 w-3 transition-transform ml-1" :class="{ 'rotate-90': isOpen }" />
    </CollapsibleTrigger>

    <CollapsibleContent>
      <div class="mt-1 rounded-lg p-3 text-xs font-mono" style="background-color: var(--secondary)">
        <template v-if="toolCall.result">
          <div v-if="toolCall.result.stdout" class="whitespace-pre-wrap break-all" style="color: var(--foreground)">
            {{ toolCall.result.stdout }}
          </div>
          <div v-if="toolCall.result.stderr" class="whitespace-pre-wrap break-all mt-2" style="color: var(--color-error-foreground)">
            {{ toolCall.result.stderr }}
          </div>
          <div v-if="toolCall.result.data" class="whitespace-pre-wrap break-all" style="color: var(--foreground)">
            {{ typeof toolCall.result.data === 'string' ? toolCall.result.data : JSON.stringify(toolCall.result.data, null, 2) }}
          </div>
        </template>
        <div v-else class="flex items-center gap-2" style="color: var(--muted-foreground)">
          <Loader2 class="h-3 w-3 animate-spin" />
          {{ $t('chat.executing') }}
        </div>
      </div>
    </CollapsibleContent>
  </Collapsible>
</template>
