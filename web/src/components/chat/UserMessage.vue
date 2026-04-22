<script setup lang="ts">
defineProps<{
  content: string
}>()
</script>

<template>
  <div class="flex justify-end">
    <div
      class="max-w-[70%] whitespace-pre-wrap rounded-[999px] px-4 py-2.5 text-sm leading-relaxed"
      style="background-color: var(--secondary); color: var(--foreground)"
    >
      {{ content }}
    </div>
  </div>
</template>
