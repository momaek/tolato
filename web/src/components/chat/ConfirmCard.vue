<script setup lang="ts">
import { AlertTriangle, Check, X } from 'lucide-vue-next'
import { Button } from '@/components/ui/button'

const props = defineProps<{
  id: string
  tool: string
  args: Record<string, unknown>
}>()

const emit = defineEmits<{
  (e: 'confirm', id: string, approved: boolean): void
}>()

const commandStr = props.tool === 'execute_command'
  ? (props.args?.command as string || '')
  : JSON.stringify(props.args || {})
</script>

<template>
  <div
    class="my-2 rounded-lg border-l-[3px] p-4"
    :style="{
      borderLeftColor: 'var(--color-warning-foreground)',
      backgroundColor: 'var(--color-warning)',
    }"
  >
    <div class="flex items-center gap-2 mb-3">
      <AlertTriangle class="h-4 w-4" style="color: var(--color-warning-foreground)" />
      <span class="text-sm font-medium" style="color: var(--color-warning-foreground)">
        {{ $t('chat.sensitiveOperation') }}
      </span>
    </div>

    <div class="mb-3 rounded p-2.5 text-xs font-mono" style="background-color: var(--secondary)">
      <div style="color: var(--muted-foreground)">{{ tool }}</div>
      <div class="mt-1" style="color: var(--foreground)">{{ commandStr }}</div>
    </div>

    <div class="flex gap-2">
      <Button size="sm" @click="emit('confirm', id, true)">
        <Check class="mr-1 h-3 w-3" />
        {{ $t('chat.approve') }}
      </Button>
      <Button size="sm" variant="outline" @click="emit('confirm', id, false)">
        <X class="mr-1 h-3 w-3" />
        {{ $t('chat.reject') }}
      </Button>
    </div>
  </div>
</template>
