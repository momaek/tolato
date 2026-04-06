<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { MessageSquare } from 'lucide-vue-next'
import { Button } from '@/components/ui/button'

const { t } = useI18n()

const emit = defineEmits<{
  (e: 'quick-action', text: string): void
}>()

const quickActions = [
  { key: 'chat.emptyState.checkNginx', command: 'Check nginx status' },
  { key: 'chat.emptyState.viewDisk', command: 'View disk usage' },
  { key: 'chat.emptyState.updatePackages', command: 'Update packages' },
]
</script>

<template>
  <div class="flex flex-1 flex-col items-center justify-center gap-6 px-4">
    <div
      class="flex h-16 w-16 items-center justify-center rounded-2xl"
      style="background-color: var(--secondary)"
    >
      <MessageSquare class="h-8 w-8" style="color: var(--primary)" />
    </div>

    <h2 class="text-xl font-medium" style="color: var(--foreground)">
      {{ $t('chat.emptyState.greeting') }}
    </h2>

    <div class="flex flex-wrap justify-center gap-2">
      <Button
        v-for="action in quickActions"
        :key="action.key"
        variant="secondary"
        class="rounded-full"
        @click="emit('quick-action', action.command)"
      >
        {{ t(action.key) }}
      </Button>
    </div>
  </div>
</template>
