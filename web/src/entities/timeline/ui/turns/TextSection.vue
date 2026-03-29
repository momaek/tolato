<script setup lang="ts">
import MarkdownRender from 'markstream-vue'
import { Copy } from 'lucide-vue-next'

import { Button } from '@/components/ui/button'
import type { TextBlock } from '@/shared/types/console'

const props = defineProps<{
  block: TextBlock
  streaming: boolean
}>()

function copyText() {
  globalThis.navigator?.clipboard?.writeText(props.block.text)
}
</script>

<template>
  <div class="max-w-none">
    <MarkdownRender
      :content="block.text"
      class="prose prose-stone max-w-none text-[15px] leading-7 [&_p]:my-0 [&_p+p]:mt-2 [&_ul]:my-2 [&_li]:my-0"
    />
    <span
      v-if="streaming"
      class="ml-0.5 inline-block h-[1em] w-[0.5ch] translate-y-[2px] animate-pulse rounded-[1px] bg-foreground/70 align-baseline"
    />

    <div v-if="!streaming && block.text" class="mt-2 flex">
      <Button
        variant="ghost"
        size="icon-sm"
        class="size-7 rounded-md text-muted-foreground/80 hover:text-foreground"
        @click="copyText"
      >
        <Copy class="size-3.5" />
      </Button>
    </div>
  </div>
</template>
