<script setup lang="ts">
import type { UserMessageRow as Row } from '@/shared/types/console'

defineProps<{
  row: Row
}>()
</script>

<template>
  <div class="flex justify-end">
    <div class="max-w-[min(62%,42rem)] rounded-[0.95rem] bg-muted/75 px-4 py-2.5 text-right shadow-sm shadow-black/[0.03]">
      <p class="text-[14px] font-medium leading-6 text-foreground">{{ row.text }}</p>
    </div>
  </div>
</template>
