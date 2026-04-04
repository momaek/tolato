<script setup lang="ts">
import { MessageSquare } from 'lucide-vue-next'
import { Button } from '@/components/ui/button'

const emit = defineEmits<{
  (e: 'quick-action', text: string): void
}>()

const quickActions = [
  'Check nginx status',
  'View disk usage',
  'Update packages',
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
      How can I help you manage your servers?
    </h2>

    <div class="flex flex-wrap justify-center gap-2">
      <Button
        v-for="action in quickActions"
        :key="action"
        variant="secondary"
        class="rounded-full"
        @click="emit('quick-action', action)"
      >
        {{ action }}
      </Button>
    </div>
  </div>
</template>
