<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { Zap } from 'lucide-vue-next'

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
  <div class="flex flex-1 items-center justify-center px-6">
    <div class="flex max-w-[540px] flex-col items-center gap-5 text-center">
      <div
        class="flex h-14 w-14 items-center justify-center rounded-[18px]"
        :style="{
          backgroundColor: 'color-mix(in oklab, var(--primary) 12%, transparent)',
          color: 'var(--primary)',
        }"
      >
        <Zap class="h-7 w-7" />
      </div>

      <div class="flex flex-col gap-1.5">
        <h2
          class="text-[22px] font-medium leading-tight"
          style="color: var(--foreground); letter-spacing: -0.015em"
        >
          {{ $t('chat.emptyState.greeting') }}
        </h2>
        <p class="text-sm" style="color: var(--muted-foreground)">
          {{ $t('chat.emptyState.subtitle') }}
        </p>
      </div>

      <div class="flex flex-wrap justify-center gap-2">
        <button
          v-for="action in quickActions"
          :key="action.key"
          type="button"
          class="rounded-full px-3.5 py-2 text-[13px] transition-colors hover:opacity-80"
          :style="{
            backgroundColor: 'var(--secondary)',
            color: 'var(--foreground)',
          }"
          @click="emit('quick-action', action.command)"
        >
          {{ t(action.key) }}
        </button>
      </div>
    </div>
  </div>
</template>
