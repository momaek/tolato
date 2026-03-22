<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'

import { Button } from '@/components/ui/button'

const emit = defineEmits<{
  select: [text: string]
}>()

const { t } = useI18n()

const chips = computed(() => [
  t('console.composer.quickActions.tokyoReload'),
  t('console.composer.quickActions.dockerStatus'),
  t('console.composer.quickActions.systemLoad'),
  t('console.composer.quickActions.networkCheck'),
])
</script>

<template>
  <div class="flex flex-wrap gap-2">
    <Button
      v-for="chip in chips"
      :key="chip"
      variant="outline"
      size="sm"
      class="rounded-2xl border-border/70 bg-background/70 px-3.5 text-foreground shadow-sm hover:-translate-y-0.5 hover:bg-accent/70"
      @click="emit('select', chip)"
    >
      {{ chip }}
    </Button>
  </div>
</template>
