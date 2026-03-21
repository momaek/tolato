<script setup lang="ts">
import { onMounted } from "vue"
import { useConsoleStore } from "@/entities/console/store"
import { Card, CardContent, CardHeader, CardTitle } from "@/shared/ui"
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

    <Card class="border-none shadow-sm">
      <CardHeader>
        <CardTitle>Supported commands</CardTitle>
      </CardHeader>
      <CardContent class="grid gap-4 lg:grid-cols-2">
        <div>
          <p class="text-sm font-medium text-foreground">Examples</p>
          <div class="mt-2 space-y-2 font-mono text-sm text-muted-foreground">
            <p>systemctl status nginx</p>
            <p>systemctl restart nginx</p>
            <p>systemctl reload nginx</p>
            <p>tail -n 100 /var/log/nginx/error.log</p>
            <p>docker ps</p>
            <p>df -h /</p>
            <p>free -m</p>
            <p>uptime</p>
          </div>
        </div>

        <div>
          <p class="text-sm font-medium text-foreground">Policy</p>
          <div class="mt-2 space-y-2 text-sm text-muted-foreground">
            <p>仅允许受控命令，不支持管道、重定向、分号、命令替换。</p>
            <p>服务名受 allowlist 约束，目前支持 nginx、docker、redis、postgresql、mysql、mariadb、caddy、myapp、php-fpm。</p>
            <p>日志路径仅允许 `/var/log/*`。</p>
          </div>
        </div>
      </CardContent>
    </Card>

    <ConsoleWorkspace />
  </div>
</template>
