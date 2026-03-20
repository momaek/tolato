<script setup lang="ts">
import { onMounted } from "vue"
import { useConsoleStore } from "@/entities/console/store"
import ConsoleWorkspace from "@/widgets/console/ConsoleWorkspace.vue"

const consoleStore = useConsoleStore()

onMounted(() => {
  consoleStore.setMode("direct_shell")
})
</script>

<template>
  <div class="space-y-4">
    <div>
      <p class="text-sm font-semibold uppercase tracking-[0.18em] text-muted-foreground">
        Console
      </p>
      <h1 class="mt-2 text-3xl font-semibold tracking-tight">Direct shell</h1>
    </div>

    <p class="max-w-3xl text-sm text-muted-foreground">
      命令风格输入仍会先被后端解析为 allowlist action，不提供裸 shell 和交互式 TTY。
    </p>

    <ConsoleWorkspace />
  </div>
</template>
