<script setup lang="ts">
defineProps<{
  eyebrow?: string
  title: string
  description: string
}>()
</script>

<template>
  <div class="flex items-start justify-between gap-4">
    <div class="space-y-3">
      <p v-if="eyebrow" class="text-xs font-semibold uppercase tracking-[0.3em] text-muted-foreground">
        {{ eyebrow }}
      </p>
      <div class="space-y-1">
        <h1 class="text-3xl font-semibold tracking-tight text-foreground">
          {{ title }}
        </h1>
        <p class="max-w-3xl text-sm leading-6 text-muted-foreground">
          {{ description }}
        </p>
      </div>
    </div>
    <div class="flex shrink-0 items-center gap-2">
      <slot name="actions" />
    </div>
  </div>
</template>
