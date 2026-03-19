<script setup lang="ts">
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/shared/ui"
</script>

<template>
  <div class="space-y-4">
    <div>
      <p class="text-sm font-semibold uppercase tracking-[0.18em] text-muted-foreground">
        Console
      </p>
      <h1 class="mt-2 text-3xl font-semibold tracking-tight">Direct shell</h1>
    </div>

    <Card class="max-w-3xl border-none shadow-sm">
      <CardHeader>
        <CardTitle>受限命令模式预留</CardTitle>
        <CardDescription>
          MVP 不提供裸 shell，也不提供交互式终端。后续版本会以受限命令模式开放，仍经过策略校验与审计。
        </CardDescription>
      </CardHeader>
      <CardContent class="space-y-3 text-sm text-muted-foreground">
        <p>1. 非裸 shell 直通。</p>
        <p>2. 非交互式 TTY。</p>
        <p>3. 仍遵循计划、审批、审计边界。</p>
      </CardContent>
    </Card>
  </div>
</template>
