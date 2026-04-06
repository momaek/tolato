<script setup lang="ts">
import { Loader2 } from 'lucide-vue-next'
</script>

<template>
  <div class="flex items-center gap-2 px-5 py-2">
    <Loader2 class="h-3.5 w-3.5 animate-spin" style="color: var(--primary)" />
    <span class="text-xs" style="color: var(--muted-foreground)">{{ $t('chat.aiThinking') }}</span>
  </div>
</template>
