<script setup lang="ts">
import MarkdownRender from 'markstream-vue'
import { Copy } from 'lucide-vue-next'

import { Button } from '@/components/ui/button'
import type { AssistantTextRow as Row } from '@/shared/types/console'

const props = defineProps<{
  row: Row
}>()

function copyAssistantText(markdown: string) {
  globalThis.navigator?.clipboard?.writeText(markdown)
}
</script>

<template>
  <div class="max-w-none px-1 py-0.5">
    <MarkdownRender
      :content="props.row.markdown"
      class="prose prose-stone max-w-none text-[15px] leading-7 [&_p]:my-0 [&_p+p]:mt-2 [&_ul]:my-2 [&_li]:my-0"
    />

    <div class="mt-2 flex">
      <Button
        variant="ghost"
        size="icon-sm"
        class="size-7 rounded-md text-muted-foreground/80 hover:text-foreground"
        @click="copyAssistantText(props.row.markdown)"
      >
        <Copy class="size-3.5" />
      </Button>
    </div>
  </div>
</template>
