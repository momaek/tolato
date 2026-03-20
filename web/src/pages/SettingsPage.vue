<script setup lang="ts">
import { storeToRefs } from "pinia"
import { useConnectionStore } from "@/entities/connection/store"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/shared/ui"
import { formatDateTime } from "@/shared/lib/format"

const connectionStore = useConnectionStore()
const { state, lastSyncAt, message } = storeToRefs(connectionStore)
const mockMode = import.meta.env.VITE_USE_MOCK ?? "false"
</script>

<template>
  <div class="space-y-4">
    <div>
      <p class="text-sm font-semibold uppercase tracking-[0.18em] text-muted-foreground">
        System
      </p>
      <h1 class="mt-2 text-3xl font-semibold tracking-tight">Settings</h1>
    </div>

    <div class="grid gap-4 xl:grid-cols-3">
      <Card class="border-none shadow-sm">
        <CardHeader>
          <CardTitle>Frontend mode</CardTitle>
          <CardDescription>
            当前默认直连真实后端，可通过环境变量切回 mock contracts。
          </CardDescription>
        </CardHeader>
        <CardContent class="text-sm text-muted-foreground">
          VITE_USE_MOCK={{ mockMode }}
        </CardContent>
      </Card>

      <Card class="border-none shadow-sm">
        <CardHeader>
          <CardTitle>Connection state</CardTitle>
        </CardHeader>
        <CardContent class="space-y-2 text-sm text-muted-foreground">
          <p>Status · {{ state }}</p>
          <p>Last sync · {{ formatDateTime(lastSyncAt) }}</p>
          <p v-if="message">Message · {{ message }}</p>
        </CardContent>
      </Card>

      <Card class="border-none shadow-sm">
        <CardHeader>
          <CardTitle>Design system</CardTitle>
          <CardDescription>
            shadcn-vue vendor layer 已全量落库，业务组件从 shared/ui 导入。
          </CardDescription>
        </CardHeader>
      </Card>
    </div>
  </div>
</template>
