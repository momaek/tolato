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
const supportsReselect = appEnv.useMock
const supportsClear = true
</script>

<template>
  <div class="rounded-[1rem] border border-border/70 bg-background px-5 py-5 shadow-sm shadow-black/[0.03]">
    <div class="flex items-start justify-between gap-3">
      <p class="text-[11px] font-semibold tracking-[0.14em] text-muted-foreground">target_confirmation</p>
      <StatusBadge label="pending" />
    </div>

    <h3 class="mt-4 text-[1.85rem] font-semibold leading-tight tracking-[-0.03em] text-foreground">
      {{ row.title }}
    </h3>

    <p class="mt-3 text-sm leading-6 text-muted-foreground">{{ row.basis }}</p>

    <div class="mt-4 flex flex-wrap items-center gap-2">
      <StatusBadge :label="row.scope" :tone="rowScopeTone" />
      <StatusBadge :label="row.source" />
    </div>

    <div class="mt-4 rounded-[0.9rem] border border-border/60 bg-muted/20 px-4 py-3">
      <p class="text-[11px] font-semibold uppercase tracking-[0.18em] text-muted-foreground">{{ t('console.targetConfirmation.originalTarget') }}</p>
      <p class="mt-1.5 text-sm leading-6 text-foreground">{{ row.originalTargetText }}</p>
      <p v-if="row.inheritedHint" class="mt-1 text-sm leading-6 text-muted-foreground">
        {{ row.inheritedHint }}
      </p>
    </div>

    <div class="mt-4 space-y-2">
      <button
        v-for="candidate in row.candidates"
        :key="candidate.id"
        class="w-full rounded-[0.95rem] border border-border/70 bg-background px-4 py-3 text-left transition hover:border-primary/25 hover:bg-muted/25"
        @click="emit('confirm', candidate)"
      >
        <div class="flex flex-wrap items-start justify-between gap-3">
          <div class="min-w-0">
            <p class="text-base font-semibold leading-6 text-foreground">{{ candidate.label }}</p>
            <p class="mt-1 text-sm leading-6 text-muted-foreground">{{ candidate.reason }}</p>
          </div>
          <StatusBadge :label="candidate.region" tone="info" />
        </div>

        <div class="mt-3 flex flex-wrap gap-2">
          <StatusBadge :label="candidate.scope" :tone="candidate.scope === 'all_online' ? 'warning' : candidate.scope === 'multi' ? 'info' : 'neutral'" />
          <StatusBadge v-for="tag in candidate.tags" :key="tag" :label="tag" />
        </div>
      </button>
    </div>

    <div class="mt-5 flex flex-wrap gap-2">
      <Button
        v-if="row.candidates.length === 1 && preferredCandidate"
        size="lg"
        class="h-11 rounded-lg px-5"
        @click="emit('confirm', preferredCandidate)"
      >
        {{ t('common.buttons.confirmTarget') }}
      </Button>
      <Button v-if="supportsReselect" variant="outline" size="lg" class="h-11 rounded-lg px-5" @click="emit('reselect')">
        {{ t('common.buttons.reselect') }}
      </Button>
      <Button v-if="supportsClear" variant="outline" size="lg" class="h-11 rounded-lg px-5" @click="emit('clear')">
        {{ t('common.buttons.clearContext') }}
      </Button>
    </div>
  </div>
</template>
