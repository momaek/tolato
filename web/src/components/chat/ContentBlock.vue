<script setup lang="ts">
import MarkdownRender from 'markstream-vue'
import 'markstream-vue/index.css'

defineProps<{
  content: string
}>()
</script>

<template>
  <div class="prose prose-invert max-w-none text-sm leading-relaxed">
    <MarkdownRender :text="content" />
  </div>
</template>
