<script setup lang="ts">
import { computed, onMounted, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { toast } from 'vue-sonner'

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

const listStore = useConsoleSessionListStore()
const viewStore = useConsoleSessionViewStore()

const snapshot = computed(() => viewStore.activeSnapshot)
const prefill = computed(() => (typeof route.query.prefill === 'string' ? route.query.prefill : ''))

async function ensureActiveSession(sessionId?: string) {
  const fallback = sessionId || listStore.activeSessionId || listStore.sessions[0]?.id
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

async function handleApprovalAction(action: 'approve' | 'reject' | 'cancel', row: ApprovalRow) {
  try {
    await viewStore.submitApproval(action, row)
  } catch (error) {
    toast.error(toErrorMessage(error, 'Failed to submit approval action'))
  }
}

onMounted(async () => {
  await listStore.initialize()
  await viewStore.initialize()
  await ensureActiveSession(route.params.sessionId as string | undefined)
})

watch(
  () => route.params.sessionId,
  async nextSessionId => {
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
        @select-session="handleSelectSession"
      />
    </div>

    <section class="flex h-full min-h-0 flex-col gap-4 overflow-hidden">
      <ConsoleHeader :snapshot="snapshot" @clear-target="handleClearTarget" />
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
      <div class="z-20 -mx-1 shrink-0 bg-gradient-to-t from-brand-canvas via-brand-canvas/95 to-transparent px-1 pt-3">
        <ConsoleComposer
          :disabled="Boolean(snapshot?.pendingActionType)"
          :initial-text="prefill"
          @submit="handleSubmit"
        />
      </div>
    </section>
  </div>
</template>
