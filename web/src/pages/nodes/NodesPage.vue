<script setup lang="ts">
import { computed, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import NodesFilterBar from '@/features/nodes-filter/NodesFilterBar.vue'
import { useNodesStore } from '@/entities/node/model/nodes.store'
import NodesTable from '@/widgets/nodes-table/NodesTable.vue'
import { formatRelativeMinutes } from '@/shared/lib/format'

const router = useRouter()
const nodesStore = useNodesStore()
const { t } = useI18n()

onMounted(async () => {
  if (!nodesStore.items.length) {
    await nodesStore.fetchAll()
  }
})

const stats = computed(() => {
  const total = nodesStore.items.length
  const online = nodesStore.items.filter(
    (node) => node.status === 'online',
  ).length
  const busy = nodesStore.items.filter((node) => node.busy).length
  const offline = nodesStore.items.filter(
    (node) => node.status === 'offline',
  ).length

  return { total, online, busy, offline }
})

const visible = computed(() => nodesStore.filteredItems.length)
const regions = computed(() =>
  Array.from(new Set(nodesStore.items.map((node) => node.region))).sort(),
)
const tags = computed(() =>
  Array.from(new Set(nodesStore.items.flatMap((node) => node.tags))).sort(),
)

function openConsole(nodeId: string) {
  void router.push({
    name: 'console',
    query: {
      nodeId,
      prefill: `Inspect node ${nodeId}`,
    },
  })
}
</script>

<template>
  <div class="min-h-screen px-4 py-6 md:px-6 xl:px-8">
    <div class="mx-auto flex max-w-7xl flex-col gap-6">
      <section class="glass-panel border-border/70 rounded-[2rem] border p-6">
        <div
          class="flex flex-col gap-6 xl:flex-row xl:items-end xl:justify-between"
        >
          <div class="max-w-3xl space-y-3">
            <div class="flex flex-wrap items-center gap-2">
              <Badge>{{ t('pages.nodes.badge') }}</Badge>
              <Badge variant="secondary">{{
                t('pages.nodes.badgeSecondary')
              }}</Badge>
            </div>
            <h1 class="text-3xl font-semibold tracking-tight md:text-4xl">
              {{ t('pages.nodes.title') }}
            </h1>
            <p
              class="max-w-2xl text-sm leading-6 text-muted-foreground md:text-base"
            >
              {{ t('pages.nodes.description') }}
            </p>
          </div>

          <div class="grid gap-3 sm:grid-cols-2 xl:min-w-[420px]">
            <Card class="bg-background/70">
              <CardContent class="p-4">
                <div
                  class="text-xs uppercase tracking-[0.2em] text-muted-foreground"
                >
                  {{ t('common.labels.total') }}
                </div>
                <div class="mt-2 text-3xl font-semibold">{{ stats.total }}</div>
              </CardContent>
            </Card>
            <Card class="bg-background/70">
              <CardContent class="p-4">
                <div
                  class="text-xs uppercase tracking-[0.2em] text-muted-foreground"
                >
                  {{ t('pages.nodes.statBusy') }}
                </div>
                <div class="mt-2 text-3xl font-semibold">{{ stats.busy }}</div>
              </CardContent>
            </Card>
            <Card class="bg-background/70">
              <CardContent class="p-4">
                <div
                  class="text-xs uppercase tracking-[0.2em] text-muted-foreground"
                >
                  {{ t('pages.nodes.statOnline') }}
                </div>
                <div class="mt-2 text-3xl font-semibold">
                  {{ stats.online }}
                </div>
              </CardContent>
            </Card>
            <Card class="bg-background/70">
              <CardContent class="p-4">
                <div
                  class="text-xs uppercase tracking-[0.2em] text-muted-foreground"
                >
                  {{ t('pages.nodes.statOffline') }}
                </div>
                <div class="mt-2 text-3xl font-semibold">
                  {{ stats.offline }}
                </div>
              </CardContent>
            </Card>
          </div>
        </div>
      </section>

      <Card
        v-if="nodesStore.error"
        class="border-brand-danger/25 bg-brand-danger/5"
      >
        <CardContent
          class="flex items-center justify-between gap-3 p-4 text-sm text-brand-danger"
        >
          <span>{{ nodesStore.error }}</span>
          <Button size="sm" variant="outline" @click="nodesStore.fetchAll()">{{
            t('common.buttons.retry')
          }}</Button>
        </CardContent>
      </Card>

      <NodesFilterBar
        v-if="stats.total > 0"
        v-model:search="nodesStore.search"
        :status="nodesStore.status"
        :region="nodesStore.region"
        :tag="nodesStore.tag"
        :busy-only="nodesStore.busyOnly"
        :regions="regions"
        :tags="tags"
        :total="stats.total"
        :visible="visible"
        @update:status="(value) => (nodesStore.status = value)"
        @update:region="(value) => (nodesStore.region = value)"
        @update:tag="(value) => (nodesStore.tag = value)"
        @update:busy-only="(value) => (nodesStore.busyOnly = value)"
        @reset="
          () => {
            nodesStore.search = ''
            nodesStore.status = 'all'
            nodesStore.region = 'all'
            nodesStore.tag = 'all'
            nodesStore.busyOnly = false
          }
        "
      />

      <Card
        v-if="stats.total === 0"
        class="border-dashed border-primary/25 bg-primary/5"
      >
        <CardContent class="space-y-4 p-6">
          <div class="space-y-2">
            <p class="text-lg font-semibold">
              {{ t('nodeOnboarding.nodes.title') }}
            </p>
            <p class="text-sm leading-6 text-muted-foreground">
              {{ t('nodeOnboarding.nodes.description') }}
            </p>
          </div>
          <div
            class="rounded-xl border border-border/70 bg-background/80 p-4 text-sm text-foreground"
          >
            <code
              >go run ./cmd/tolato-nodeagent -server
              ws://127.0.0.1:8080/ws/agent -node-id demo-node -auth-token
              agent-dev-token</code
            >
          </div>
          <p class="text-sm text-muted-foreground">
            {{ t('nodeOnboarding.nodes.hint') }}
          </p>
          <div class="flex flex-wrap gap-2">
            <Button @click="nodesStore.fetchAll()">{{
              t('common.buttons.retry')
            }}</Button>
            <Button
              variant="outline"
              @click="router.push({ name: 'console' })"
              >{{ t('common.buttons.goToConsole') }}</Button
            >
          </div>
        </CardContent>
      </Card>

      <NodesTable
        v-else
        :nodes="nodesStore.filteredItems"
        @open-console="openConsole"
      />

      <div class="grid gap-4 lg:grid-cols-3">
        <Card class="glass-panel border-border/70 rounded-2xl lg:col-span-2">
          <CardContent class="space-y-4 p-6">
            <div class="text-sm font-medium">
              {{ t('pages.nodes.statusSnapshot') }}
            </div>
            <div class="grid gap-3 md:grid-cols-3">
              <div
                class="rounded-xl border border-border/70 bg-background/70 p-4"
              >
                <div
                  class="text-xs uppercase tracking-[0.2em] text-muted-foreground"
                >
                  {{ t('common.labels.latestUpdate') }}
                </div>
                <div class="mt-2 text-sm">
                  {{
                    formatRelativeMinutes(
                      nodesStore.items[0]?.lastSeen ?? new Date().toISOString(),
                    )
                  }}
                </div>
              </div>
              <div
                class="rounded-xl border border-border/70 bg-background/70 p-4"
              >
                <div
                  class="text-xs uppercase tracking-[0.2em] text-muted-foreground"
                >
                  {{ t('common.labels.filterMode') }}
                </div>
                <div class="mt-2 text-sm">{{ nodesStore.status }}</div>
              </div>
              <div
                class="rounded-xl border border-border/70 bg-background/70 p-4"
              >
                <div
                  class="text-xs uppercase tracking-[0.2em] text-muted-foreground"
                >
                  {{ t('common.labels.search') }}
                </div>
                <div class="mt-2 text-sm">
                  {{ nodesStore.search || t('common.labels.allNodes') }}
                </div>
              </div>
            </div>
          </CardContent>
        </Card>

        <Card class="glass-panel border-border/70 rounded-2xl">
          <CardContent class="space-y-3 p-6">
            <div class="text-sm font-medium">
              {{ t('pages.nodes.consoleShortcut') }}
            </div>
            <p class="text-sm text-muted-foreground">
              {{ t('pages.nodes.consoleShortcutDescription') }}
            </p>
            <Button class="w-full" @click="router.push({ name: 'console' })">{{
              t('common.buttons.goToConsole')
            }}</Button>
          </CardContent>
        </Card>
      </div>
    </div>
  </div>
</template>
