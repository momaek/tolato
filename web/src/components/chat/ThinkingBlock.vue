<script setup lang="ts">
import { ref } from 'vue'
import { Brain, ChevronRight } from 'lucide-vue-next'
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from '@/components/ui/collapsible'

defineProps<{
  reasoning: string
}>()

const isOpen = ref(false)
</script>

<template>
  <Collapsible v-model:open="isOpen" class="mb-2">
    <CollapsibleTrigger
      class="flex items-center gap-1.5 text-xs cursor-pointer opacity-70 hover:opacity-100"
      style="color: var(--muted-foreground)"
    >
      <Brain class="h-3.5 w-3.5" />
      <span>{{ $t('chat.thinking') }}</span>
      <ChevronRight
        class="h-3 w-3 transition-transform"
        :class="{ 'rotate-90': isOpen }"
      />
    </CollapsibleTrigger>
    <CollapsibleContent>
      <div
        class="mt-2 rounded-lg p-3 text-xs whitespace-pre-wrap leading-relaxed"
        style="background-color: var(--secondary); color: var(--muted-foreground)"
      >
        {{ reasoning }}
      </div>
    </CollapsibleContent>
  </Collapsible>
</template>
