<script setup lang="ts">
import ConsoleWorkspace from "@/widgets/console/ConsoleWorkspace.vue"
</script>

<template>
  <div class="space-y-4">
    <div>
      <p class="text-sm font-semibold uppercase tracking-[0.18em] text-muted-foreground">
        Console
      </p>
      <h1 class="mt-2 text-3xl font-semibold tracking-tight">AI Agent</h1>
    </div>

    <ConsoleWorkspace />
  </div>
</template>
