<script setup lang="ts">
import { computed, onMounted, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute, useRouter } from 'vue-router'
import { toast } from 'vue-sonner'

import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import { useNodesStore } from '@/entities/node/model/nodes.store'
import { useConsoleSessionListStore } from '@/entities/session/model/session-list.store'
import { useConsoleSessionViewStore } from '@/entities/session/model/session-view.store'
import ConsoleComposer from '@/widgets/console-composer/ConsoleComposer.vue'
import ConsoleHeader from '@/widgets/console-header/ConsoleHeader.vue'
import ConsoleSidebar from '@/widgets/console-sidebar/ConsoleSidebar.vue'
import ConsoleTimeline from '@/widgets/console-timeline/ConsoleTimeline.vue'
import { toErrorMessage } from '@/shared/lib/errors'
import type { ApprovalRow, TargetCandidate } from '@/shared/types/console'

const route = useRoute()
const router = useRouter()
const { t } = useI18n()

const listStore = useConsoleSessionListStore()
const nodesStore = useNodesStore()
const viewStore = useConsoleSessionViewStore()

const snapshot = computed(() => viewStore.activeSnapshot)
const prefill = computed(() =>
  typeof route.query.prefill === 'string' ? route.query.prefill : '',
)
const hasNodes = computed(() => nodesStore.items.length > 0)
const hasActiveSession = computed(() => Boolean(listStore.activeSessionId))
const sessionGenerating = computed(
  () =>
    snapshot.value?.status === 'running' ||
    snapshot.value?.llmStreamState?.status === 'streaming' ||
    viewStore.isActiveMessageSubmitting,
)
const composerDisabled = computed(
  () =>
    Boolean(snapshot.value?.pendingActionType) ||
    sessionGenerating.value ||
    !hasNodes.value ||
    !hasActiveSession.value,
)

async function ensureActiveSession(sessionId?: string) {
  const fallback =
    sessionId || listStore.activeSessionId || listStore.sessions[0]?.id
  if (!fallback) {
    return
  }

  listStore.setActiveSession(fallback)
  await viewStore.switchSession(fallback)

  if (route.params.sessionId !== fallback) {
    await router.replace(`/console/${fallback}`)
  }
}

async function handleSelectSession(sessionId: string) {
  listStore.setActiveSession(sessionId)
  await router.push(`/console/${sessionId}`)
}

async function handleCreateSession() {
  try {
    const sessionId = await listStore.createSession()
    await ensureActiveSession(sessionId)
  } catch (error) {
    toast.error(toErrorMessage(error, 'Failed to create session'))
  }
}

async function handleDeleteSession(sessionId: string) {
  const session = listStore.sessions.find((item) => item.id === sessionId)
  const deletingActiveSession = sessionId === listStore.activeSessionId
  const confirmed = globalThis.confirm?.(
    `Delete session "${session?.title ?? sessionId}"?`,
  )
  if (confirmed === false) {
    return
  }

  try {
    viewStore.removeSession(sessionId)
    const nextSessionId = await listStore.deleteSession(sessionId)
    if (deletingActiveSession && nextSessionId) {
      await ensureActiveSession(nextSessionId)
      return
    }
    if (!deletingActiveSession) {
      return
    }
    viewStore.clearActiveSession()
    await router.replace('/console')
  } catch (error) {
    toast.error(toErrorMessage(error, 'Failed to delete session'))
  }
}

async function handleSubmit(text: string) {
  try {
    await viewStore.submitMessage(text)
  } catch (error) {
    toast.error(toErrorMessage(error, 'Failed to submit message'))
  }
}

async function handleConfirmTarget(candidate: TargetCandidate) {
  try {
    await viewStore.confirmTarget(candidate)
  } catch (error) {
    toast.error(toErrorMessage(error, 'Failed to confirm target'))
  }
}

async function handleReselectTarget() {
  try {
    await viewStore.reselectTarget()
  } catch (error) {
    toast.error(toErrorMessage(error, 'Failed to refresh target candidates'))
  }
}

async function handleClearTarget() {
  try {
    await viewStore.clearTargetContext()
  } catch (error) {
    toast.error(toErrorMessage(error, 'Failed to clear target context'))
  }
}

async function handleApprovalAction(
  action: 'approve' | 'reject' | 'cancel',
  row: ApprovalRow,
) {
  try {
    await viewStore.submitApproval(action, row)
  } catch (error) {
    toast.error(toErrorMessage(error, 'Failed to submit approval action'))
  }
}

onMounted(async () => {
  if (!nodesStore.initialized) {
    await nodesStore.fetchAll()
  }
  await listStore.initialize()
  await viewStore.initialize()
  await ensureActiveSession(route.params.sessionId as string | undefined)
})

watch(
  () => route.params.sessionId,
  async (nextSessionId) => {
    if (!listStore.initialized) {
      return
    }
    await ensureActiveSession(nextSessionId as string | undefined)
  },
)
</script>

<template>
  <div class="grid h-full min-h-0 gap-4 xl:grid-cols-[340px_minmax(0,1fr)]">
    <div class="min-h-0">
      <ConsoleSidebar
        :sessions="listStore.sessions"
        :active-session-id="listStore.activeSessionId"
        @create-session="handleCreateSession"
        @delete-session="handleDeleteSession"
        @select-session="handleSelectSession"
      />
    </div>

    <section class="flex h-full min-h-0 flex-col gap-4 overflow-hidden">
      <ConsoleHeader :snapshot="snapshot" @clear-target="handleClearTarget" />
      <Card
        v-if="!hasNodes"
        class="border-amber-300/50 bg-amber-50/80 shadow-sm dark:bg-amber-950/20"
      >
        <CardContent
          class="flex flex-col gap-3 p-4 md:flex-row md:items-center md:justify-between"
        >
          <div class="space-y-1">
            <p class="text-sm font-semibold text-foreground">
              {{ t('nodeOnboarding.console.title') }}
            </p>
            <p class="text-sm text-muted-foreground">
              {{ t('nodeOnboarding.console.description') }}
            </p>
          </div>
          <Button variant="outline" @click="router.push({ name: 'nodes' })">{{
            t('nodeOnboarding.actions.openNodes')
          }}</Button>
        </CardContent>
      </Card>
      <Card
        v-else-if="!hasActiveSession"
        class="border-border/60 bg-background/70 shadow-sm"
      >
        <CardContent
          class="flex flex-col gap-2 p-4 text-sm md:flex-row md:items-center md:justify-between"
        >
          <div class="space-y-1">
            <p class="font-semibold text-foreground">
              {{ t('console.sessions.emptyTitle') }}
            </p>
            <p class="text-muted-foreground">
              {{ t('console.sessions.emptyDescription') }}
            </p>
          </div>
        </CardContent>
      </Card>
      <div class="min-h-0 flex-1">
        <ConsoleTimeline
          class="h-full"
          :rows="viewStore.activeRows"
          :loading="viewStore.isLoadingSnapshot"
          :llm-stream-state="snapshot?.llmStreamState ?? null"
          @confirm-target="handleConfirmTarget"
          @reselect-target="handleReselectTarget"
          @clear-target="handleClearTarget"
          @approval-action="handleApprovalAction"
        />
      </div>
      <div
        class="z-20 -mx-1 shrink-0 bg-gradient-to-t from-brand-canvas via-brand-canvas/95 to-transparent px-1 pt-3"
      >
        <ConsoleComposer
          :disabled="composerDisabled"
          :initial-text="prefill"
          @submit="handleSubmit"
        />
      </div>
    </section>
  </div>
</template>
