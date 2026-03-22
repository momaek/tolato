<script setup lang="ts">
import { computed, onMounted, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import { Separator } from '@/components/ui/separator'
import { useNodeDetailStore } from '@/entities/node/model/node-detail.store'
import NodeOverview from '@/widgets/node-overview/NodeOverview.vue'

const route = useRoute()
const router = useRouter()
const nodeDetailStore = useNodeDetailStore()
const { t } = useI18n()

async function loadNode(id: string) {
  await nodeDetailStore.fetch(id)
}

onMounted(async () => {
  if (typeof route.params.id === 'string') {
    await loadNode(route.params.id)
  }
})

watch(
  () => route.params.id,
  async id => {
    if (typeof id === 'string') {
      await loadNode(id)
    }
  },
)

const node = computed(() => nodeDetailStore.item)
</script>

<template>
  <div class="min-h-screen px-4 py-6 md:px-6 xl:px-8">
    <div class="mx-auto flex max-w-7xl flex-col gap-6">
      <section class="glass-panel border-border/70 rounded-[2rem] border p-6">
        <div class="flex flex-col gap-4 lg:flex-row lg:items-end lg:justify-between">
          <div class="space-y-3">
            <div class="flex flex-wrap items-center gap-2">
              <Badge variant="secondary">{{ t('pages.nodeDetail.badge') }}</Badge>
              <Badge variant="outline">{{ t('pages.nodeDetail.badgeSecondary') }}</Badge>
            </div>
            <h1 class="text-3xl font-semibold tracking-tight md:text-4xl">{{ t('pages.nodeDetail.title') }}</h1>
            <p class="max-w-3xl text-sm leading-6 text-muted-foreground md:text-base">{{ t('pages.nodeDetail.description') }}</p>
          </div>

          <div class="flex flex-wrap gap-2">
            <Button variant="outline" @click="router.push({ name: 'nodes' })">{{ t('common.nav.nodes') }}</Button>
            <Button @click="router.push({ name: 'console' })">{{ t('common.buttons.openConsole') }}</Button>
          </div>
        </div>
      </section>

      <Card v-if="nodeDetailStore.error" class="border-brand-danger/25 bg-brand-danger/5">
        <CardContent class="flex items-center justify-between gap-3 p-4 text-sm text-brand-danger">
          <span>{{ nodeDetailStore.error }}</span>
          <Button
            size="sm"
            variant="outline"
            @click="typeof route.params.id === 'string' && loadNode(route.params.id)"
          >
            {{ t('common.buttons.retry') }}
          </Button>
        </CardContent>
      </Card>

      <NodeOverview v-if="node" :node="node" />

      <div class="grid gap-6 xl:grid-cols-[1.1fr_0.9fr]">
        <Card class="glass-panel border-border/70 rounded-2xl">
          <CardContent class="space-y-4 p-6">
            <div class="text-sm font-medium">{{ t('pages.nodeDetail.riskSignals') }}</div>
            <div class="space-y-3">
              <div
                v-for="signal in node?.riskSignals ?? []"
                :key="signal"
                class="rounded-xl border border-border/70 bg-background/70 p-4 text-sm text-muted-foreground"
              >
                {{ signal }}
              </div>
            </div>
          </CardContent>
        </Card>

        <Card class="glass-panel border-border/70 rounded-2xl">
          <CardContent class="space-y-4 p-6">
            <div class="text-sm font-medium">{{ t('pages.nodeDetail.recentTasks') }}</div>
            <div class="space-y-3">
              <div
                v-for="task in node?.recentTasks ?? []"
                :key="task.id"
                class="rounded-xl border border-border/70 bg-background/70 p-4"
              >
                <div class="flex items-center justify-between gap-3">
                  <div class="font-medium">{{ task.title }}</div>
                  <Badge :variant="task.status === 'success' ? 'default' : task.status === 'failed' ? 'destructive' : 'outline'">
                    {{ task.status }}
                  </Badge>
                </div>
                <div class="mt-2 text-sm text-muted-foreground">{{ task.createdAt }}</div>
              </div>
            </div>
          </CardContent>
        </Card>
      </div>

      <Separator />

      <div v-if="!node" class="rounded-2xl border border-dashed border-border/70 p-8 text-sm text-muted-foreground">
        {{ t('common.states.loadingNodeDetail') }}
      </div>
    </div>
  </div>
</template>
