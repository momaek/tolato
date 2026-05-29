<script setup lang="ts">
import MarkdownRender from 'markstream-vue'
import 'markstream-vue/index.css'
// Side-effect import: registers our lightweight shiki code block once.
import '@/lib/registerCodeBlock'

defineProps<{
  content: string
}>()
</script>

<template>
  <div class="max-w-none text-sm leading-relaxed">
    <MarkdownRender :content="content" />
  </div>
</template>
