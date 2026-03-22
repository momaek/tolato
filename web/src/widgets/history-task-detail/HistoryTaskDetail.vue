<script setup lang="ts">
import MarkdownRender from 'markstream-vue'
import { useI18n } from 'vue-i18n'

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Separator } from '@/components/ui/separator'
import type { HistoryTaskDetail as HistoryTaskDetailType } from '@/shared/types/history'
import { formatDateTime } from '@/shared/lib/format'

defineProps<{
  task: HistoryTaskDetailType | null
}>()

const emit = defineEmits<{
  (e: 'back-to-console'): void
}>()

const { t } = useI18n()
</script>

<template>
  <Card class="glass-panel border-border/70 rounded-2xl">
    <CardHeader class="flex-row items-start justify-between gap-4">
      <div>
        <CardTitle class="text-lg">{{ t('history.taskDetail.title') }}</CardTitle>
        <p class="mt-2 text-sm text-muted-foreground">
          {{ task ? t('history.taskDetail.updatedAt', { value: formatDateTime(task.updatedAt) }) : t('history.taskDetail.empty') }}
        </p>
      </div>

      <Button variant="outline" size="sm" @click="emit('back-to-console')">{{ t('common.buttons.backToConsole') }}</Button>
    </CardHeader>

    <CardContent v-if="task" class="space-y-6">
      <div class="space-y-3">
        <div class="text-xl font-medium">{{ task.title }}</div>
        <div class="text-sm text-muted-foreground">{{ task.summary }}</div>
        <div class="flex flex-wrap gap-2">
          <Badge>{{ task.status }}</Badge>
          <Badge variant="secondary">{{ task.approvalStatus }}</Badge>
          <Badge variant="outline">{{ task.risk }}</Badge>
          <Badge variant="outline">{{ task.targetLabels.join(', ') }}</Badge>
        </div>
      </div>

      <div class="grid gap-4 lg:grid-cols-2">
        <div class="rounded-2xl border border-border/70 bg-background/70 p-4">
          <div class="text-sm font-medium">{{ t('history.taskDetail.impact') }}</div>
          <p class="mt-2 text-sm text-muted-foreground">{{ task.impact }}</p>
        </div>
        <div class="rounded-2xl border border-border/70 bg-background/70 p-4">
          <div class="text-sm font-medium">{{ t('history.taskDetail.steps') }}</div>
          <ol class="mt-3 space-y-2 text-sm text-muted-foreground">
            <li v-for="step in task.steps" :key="step" class="flex gap-2">
              <span class="text-foreground/60">•</span>
              <span>{{ step }}</span>
            </li>
          </ol>
        </div>
      </div>

      <div v-if="task.plan" class="rounded-2xl border border-border/70 bg-background/70 p-4">
        <div class="text-sm font-medium">{{ t('history.taskDetail.plan') }}</div>
        <p class="mt-2 text-sm text-muted-foreground">{{ task.plan.summary }}</p>
        <div class="mt-3 flex flex-wrap gap-2">
          <Badge variant="outline">{{ task.plan.riskLevel }}</Badge>
          <Badge :variant="task.plan.requiresApproval ? 'secondary' : 'outline'">
            {{ task.plan.requiresApproval ? 'approval required' : 'auto-run ready' }}
          </Badge>
          <Badge v-for="target in task.plan.targetNodes" :key="target" variant="outline">{{ target }}</Badge>
        </div>
        <ol class="mt-4 space-y-3">
          <li v-for="step in task.plan.steps" :key="step.id" class="rounded-xl border border-border/70 p-3">
            <div class="font-medium">{{ step.action }}</div>
            <p v-if="step.args && Object.keys(step.args).length" class="mt-1 text-sm text-muted-foreground">
              {{ JSON.stringify(step.args) }}
            </p>
          </li>
        </ol>
      </div>

      <div v-if="task.approval" class="rounded-2xl border border-border/70 bg-background/70 p-4">
        <div class="text-sm font-medium">{{ t('history.taskDetail.approval') }}</div>
        <div class="mt-3 flex flex-wrap gap-2">
          <Badge>{{ task.approval.status }}</Badge>
          <Badge variant="outline">{{ task.approval.riskLevel }}</Badge>
        </div>
        <p v-if="task.approval.latestDecision" class="mt-3 text-sm text-muted-foreground">
          {{ task.approval.latestDecision }}
        </p>
        <p v-if="task.approval.latestReason" class="mt-1 text-sm text-muted-foreground">{{ task.approval.latestReason }}</p>
      </div>

      <Separator />

      <div class="grid gap-4 xl:grid-cols-[1fr_1fr]">
        <div class="rounded-2xl border border-border/70 bg-background/70 p-4">
          <div class="text-sm font-medium">{{ t('history.taskDetail.execution') }}</div>
          <div class="mt-4 space-y-3">
            <div v-for="execution in task.executions" :key="execution.nodeId" class="rounded-xl border border-border/70 p-3">
              <div class="flex items-center justify-between gap-3">
                <div class="font-medium">{{ execution.label }}</div>
                <Badge :variant="execution.status === 'success' ? 'default' : execution.status === 'failed' ? 'destructive' : 'outline'">
                  {{ execution.status }}
                </Badge>
              </div>
              <div v-if="execution.stdoutTail" class="mt-2 whitespace-pre-wrap text-sm text-muted-foreground">
                {{ execution.stdoutTail }}
              </div>
              <div v-if="execution.stderrTail" class="mt-2 whitespace-pre-wrap text-sm text-destructive">
                {{ execution.stderrTail }}
              </div>
            </div>
          </div>
        </div>

        <div class="rounded-2xl border border-border/70 bg-background/70 p-4">
          <div class="text-sm font-medium">{{ t('history.taskDetail.aiSummary') }}</div>
          <div class="prose prose-sm mt-3 max-w-none prose-slate dark:prose-invert">
            <MarkdownRender :content="task.aiSummary" />
          </div>
          <Separator class="my-4" />
          <div class="text-sm font-medium">{{ t('history.taskDetail.toolMeta') }}</div>
          <div class="mt-3 flex flex-wrap gap-2">
            <Badge v-for="meta in task.toolMeta" :key="meta" variant="outline">{{ meta }}</Badge>
          </div>
        </div>
      </div>

      <div class="rounded-2xl border border-border/70 bg-background/70 p-4">
        <div class="text-sm font-medium">{{ t('history.taskDetail.audit') }}</div>
        <div class="mt-4 space-y-3">
          <div v-for="event in task.auditEvents" :key="event.id" class="rounded-xl border border-border/70 p-3">
            <div class="flex flex-wrap items-center justify-between gap-2 text-sm">
              <span class="font-medium">{{ event.eventType }}</span>
              <span class="text-muted-foreground">{{ formatDateTime(event.createdAt) }}</span>
            </div>
            <p class="mt-2 text-sm text-muted-foreground">{{ event.description }}</p>
          </div>
        </div>
      </div>

      <div v-if="task.toolCalls?.length || task.toolResults?.length" class="grid gap-4 xl:grid-cols-2">
        <div v-if="task.toolCalls?.length" class="rounded-2xl border border-border/70 bg-background/70 p-4">
          <div class="text-sm font-medium">{{ t('history.taskDetail.toolCalls') }}</div>
          <div class="mt-4 space-y-3">
            <div v-for="call in task.toolCalls" :key="call.id" class="rounded-xl border border-border/70 p-3">
              <div class="flex flex-wrap items-center justify-between gap-2">
                <span class="font-medium">{{ call.toolName }}</span>
                <span class="text-xs text-muted-foreground">{{ formatDateTime(call.createdAt) }}</span>
              </div>
              <p v-if="call.argsPreview" class="mt-2 text-sm text-muted-foreground">{{ call.argsPreview }}</p>
            </div>
          </div>
        </div>

        <div v-if="task.toolResults?.length" class="rounded-2xl border border-border/70 bg-background/70 p-4">
          <div class="text-sm font-medium">{{ t('history.taskDetail.toolResults') }}</div>
          <div class="mt-4 space-y-3">
            <div v-for="result in task.toolResults" :key="result.id" class="rounded-xl border border-border/70 p-3">
              <div class="flex flex-wrap items-center justify-between gap-2">
                <span class="font-medium">{{ result.toolName }}</span>
                <Badge variant="outline">{{ result.status }}</Badge>
              </div>
              <p v-if="result.text" class="mt-2 text-sm text-muted-foreground">{{ result.text }}</p>
            </div>
          </div>
        </div>
      </div>

      <div
        v-if="task.planRows?.length || task.approvalRows?.length || task.executionRows?.length || task.summaryRows?.length"
        class="rounded-2xl border border-border/70 bg-background/70 p-4"
      >
        <div class="text-sm font-medium">{{ t('history.taskDetail.timelineRows') }}</div>
        <div class="mt-4 space-y-3">
          <div
            v-for="row in [...(task.planRows ?? []), ...(task.approvalRows ?? []), ...(task.executionRows ?? []), ...(task.summaryRows ?? [])]"
            :key="row.id"
            class="rounded-xl border border-border/70 p-3"
          >
            <div class="flex flex-wrap items-center justify-between gap-2">
              <span class="font-medium">{{ row.kind }}</span>
              <span class="text-xs text-muted-foreground">{{ formatDateTime(row.createdAt) }}</span>
            </div>
            <p v-if="row.text" class="mt-2 text-sm text-muted-foreground">{{ row.text }}</p>
            <p v-else-if="row.toolName" class="mt-2 text-sm text-muted-foreground">{{ row.toolName }}</p>
          </div>
        </div>
      </div>
    </CardContent>

    <CardContent v-else class="py-10 text-sm text-muted-foreground">
      {{ t('history.taskDetail.selectFromList') }}
    </CardContent>
  </Card>
</template>
