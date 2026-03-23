<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'

import { Button } from '@/components/ui/button'
import { appEnv } from '@/shared/config/env'
import StatusBadge from '@/shared/ui/status-badge/StatusBadge.vue'
import type { TargetCandidate, TargetConfirmationRow as Row } from '@/shared/types/console'

const props = defineProps<{
  row: Row
}>()

const emit = defineEmits<{
  confirm: [candidate: TargetCandidate]
  reselect: []
  clear: []
}>()
const { t } = useI18n()

function normalizeScopeLabel(scope: string) {
  switch (scope) {
    case 'single node':
    case 'single':
      return t('console.targetConfirmation.scopeSingle')
    case 'multi node target set':
    case 'multi':
      return t('console.targetConfirmation.scopeMulti')
    case 'all online nodes':
    case 'all_online':
      return t('console.targetConfirmation.scopeAllOnline')
    default:
      return scope || t('common.labels.none')
  }
}

function normalizeSourceLabel(source: string) {
  switch (source) {
    case 'resolver':
    case 'assistant_resolved':
      return t('console.targetConfirmation.sourceResolver')
    case 'manual':
    case 'user_explicit':
      return t('console.targetConfirmation.sourceManual')
    case 'session_context':
    case 'context_inherited':
      return t('console.targetConfirmation.sourceSessionContext')
    default:
      return t('console.targetConfirmation.sourceNone')
  }
}

function normalizeCandidateReason(reason: string) {
  switch (reason) {
    case 'no direct match, available for manual confirmation':
      return t('console.targetConfirmation.reasonManualConfirmation')
    case 'matched node id':
      return t('console.targetConfirmation.reasonMatchedNodeId')
    case 'matched hostname':
      return t('console.targetConfirmation.reasonMatchedHostname')
    case 'matched region':
      return t('console.targetConfirmation.reasonMatchedRegion')
    case 'matched tag':
      return t('console.targetConfirmation.reasonMatchedTag')
    case 'matched all online nodes':
      return t('console.targetConfirmation.reasonAllOnline')
    case 'resolved from backend target context':
      return t('console.targetConfirmation.reasonResolved')
    default:
      return reason
  }
}

const rowScopeTone = computed(() => {
  if (props.row.scope.includes('all online')) {
    return 'warning'
  }

  if (props.row.scope.includes('multi')) {
    return 'info'
  }

  return 'neutral'
})

const preferredCandidate = computed(() => props.row.candidates[0] ?? null)
const title = computed(() =>
  props.row.candidates.length > 1
    ? t('console.targetConfirmation.titleMulti')
    : t('console.targetConfirmation.titleSingle'),
)
const description = computed(() =>
  t('console.targetConfirmation.description', {
    count: props.row.candidates.length,
  }),
)
const scopeLabel = computed(() => normalizeScopeLabel(props.row.scope))
const sourceLabel = computed(() => normalizeSourceLabel(props.row.source))
const pauseReasonText = computed(() => {
  if (
    props.row.originalTargetText &&
    !props.row.originalTargetText.toLowerCase().includes('unset')
  ) {
    return props.row.originalTargetText
  }

  if (preferredCandidate.value) {
    return t('console.targetConfirmation.pauseReasonFallbackCandidate', {
      label: preferredCandidate.value.label,
      nodeId: preferredCandidate.value.nodeId,
    })
  }

  return t('console.targetConfirmation.pauseReasonFallback')
})
const supportsReselect = appEnv.useMock
const supportsClear = true
</script>

<template>
  <div class="rounded-[1rem] border border-border/70 bg-background px-5 py-5 shadow-sm shadow-black/[0.03]">
    <div class="flex items-start justify-between gap-3">
      <p class="text-[11px] font-semibold tracking-[0.14em] text-muted-foreground">
        {{ t('console.targetConfirmation.eyebrow') }}
      </p>
      <StatusBadge :label="t('console.targetConfirmation.pending')" />
    </div>

    <h3 class="mt-4 text-[1.85rem] font-semibold leading-tight tracking-[-0.03em] text-foreground">
      {{ title }}
    </h3>

    <p class="mt-3 text-sm leading-6 text-muted-foreground">{{ description }}</p>

    <div class="mt-4 flex flex-wrap items-center gap-2">
      <StatusBadge :label="scopeLabel" :tone="rowScopeTone" />
      <StatusBadge :label="sourceLabel" />
    </div>

    <div class="mt-4 rounded-[0.9rem] border border-border/60 bg-muted/20 px-4 py-3">
      <p class="text-[11px] font-semibold uppercase tracking-[0.18em] text-muted-foreground">
        {{ t('console.targetConfirmation.pauseReason') }}
      </p>
      <p class="mt-1.5 text-sm leading-6 text-foreground">{{ pauseReasonText }}</p>
      <p
        v-if="row.source === 'session_context' || row.inheritedHint"
        class="mt-1 text-sm leading-6 text-muted-foreground"
      >
        {{ t('console.targetConfirmation.inheritedHint') }}
      </p>
    </div>

    <div class="mt-4 space-y-2">
      <div
        v-for="candidate in row.candidates"
        :key="candidate.id"
        class="rounded-[0.95rem] border border-border/70 bg-background px-4 py-4"
      >
        <div class="flex flex-wrap items-start justify-between gap-3">
          <div class="min-w-0">
            <p class="text-base font-semibold leading-6 text-foreground">{{ candidate.label }}</p>
            <p class="mt-1 text-sm leading-6 text-muted-foreground">
              {{ t('console.targetConfirmation.candidateNodeId') }}: {{ candidate.nodeId }}
            </p>
          </div>
          <StatusBadge :label="candidate.region" tone="info" />
        </div>

        <div class="mt-3 flex flex-wrap gap-2">
          <StatusBadge
            :label="normalizeScopeLabel(candidate.scope)"
            :tone="candidate.scope === 'all_online' ? 'warning' : candidate.scope === 'multi' ? 'info' : 'neutral'"
          />
          <StatusBadge v-for="tag in candidate.tags" :key="tag" :label="tag" />
        </div>

        <div class="mt-3 rounded-[0.8rem] bg-muted/25 px-3 py-2">
          <p class="text-[11px] font-semibold uppercase tracking-[0.16em] text-muted-foreground">
            {{ t('console.targetConfirmation.candidateReason') }}
          </p>
          <p class="mt-1.5 text-sm leading-6 text-foreground">
            {{ normalizeCandidateReason(candidate.reason) }}
          </p>
        </div>

        <div class="mt-4 flex flex-wrap gap-2">
          <Button
            size="lg"
            class="h-11 rounded-lg px-5"
            @click="emit('confirm', candidate)"
          >
            {{
              t('console.targetConfirmation.confirmCandidate', {
                label: candidate.label,
              })
            }}
          </Button>
        </div>
      </div>
    </div>

    <div class="mt-5 flex flex-wrap gap-2">
      <Button v-if="supportsReselect" variant="outline" size="lg" class="h-11 rounded-lg px-5" @click="emit('reselect')">
        {{ t('common.buttons.reselect') }}
      </Button>
      <Button v-if="supportsClear" variant="outline" size="lg" class="h-11 rounded-lg px-5" @click="emit('clear')">
        {{ t('console.targetConfirmation.clearAndReselect') }}
      </Button>
    </div>
  </div>
</template>
