<script setup lang="ts">
defineProps<{
  content: string
}>()
</script>

<template>
  <div class="flex justify-end px-5 py-3">
    <div
      class="max-w-[70%] rounded-2xl px-4 py-2.5 text-sm whitespace-pre-wrap"
      style="background-color: var(--primary); color: var(--primary-foreground)"
    >
      {{ content }}
    </div>
  </div>
</template>
