<script setup lang="ts">
import MarkdownRender from 'markstream-vue'
import { Copy } from 'lucide-vue-next'
import { useI18n } from 'vue-i18n'

import { Button } from '@/components/ui/button'
import type { SummaryRow as Row } from '@/shared/types/console'

defineProps<{
  row: Row
}>()

const { t } = useI18n()

function copySummary(markdown: string) {
  globalThis.navigator?.clipboard?.writeText(markdown)
}
</script>

<template>
  <div class="px-1 py-0.5">
    <MarkdownRender
      :content="row.markdown"
      class="prose prose-stone max-w-none text-[15px] leading-7 [&_p]:my-0 [&_p+p]:mt-2 [&_strong]:font-semibold [&_ul]:my-2 [&_li]:my-0"
    />

    <div v-if="row.nextSteps.length" class="mt-3">
      <p class="text-[11px] font-semibold tracking-[0.14em] text-muted-foreground">{{ t('console.summary.nextSteps') }}</p>
      <ul class="mt-1.5 space-y-1 text-sm leading-6 text-muted-foreground">
        <li v-for="step in row.nextSteps" :key="step">• {{ step }}</li>
      </ul>
    </div>

    <div class="mt-2 flex">
      <Button
        variant="ghost"
        size="icon-sm"
        class="size-7 rounded-md text-muted-foreground/80 hover:text-foreground"
        @click="copySummary(row.markdown)"
      >
        <Copy class="size-3.5" />
      </Button>
    </div>
  </div>
</template>
