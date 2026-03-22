<script setup lang="ts">
import { useI18n } from 'vue-i18n'

import { Badge } from '@/components/ui/badge'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { ScrollArea } from '@/components/ui/scroll-area'
import type { HistoryTaskItem } from '@/shared/types/history'
import { formatRelativeMinutes } from '@/shared/lib/format'
import { cn } from '@/lib/utils'

const props = defineProps<{
  items: HistoryTaskItem[]
  selectedTaskId: string
  search: string
  statusFilter: string
}>()

const emit = defineEmits<{
  (e: 'update:search', value: string): void
  (e: 'update:statusFilter', value: string): void
  (e: 'select', taskId: string): void
}>()

const statusOptions = ['all', 'waiting_approval', 'running', 'success', 'failed', 'partial_failed']
const { t } = useI18n()
</script>

<template>
  <Card class="glass-panel border-border/70 rounded-2xl">
    <CardHeader class="space-y-3">
      <CardTitle class="text-lg">{{ t('history.taskList.title') }}</CardTitle>
      <Input
        :model-value="search"
        :placeholder="t('history.taskList.searchPlaceholder')"
        @update:model-value="value => emit('update:search', String(value))"
      />
      <div class="flex flex-wrap gap-2">
        <button
          v-for="option in statusOptions"
          :key="option"
          type="button"
          class="rounded-full border px-3 py-1.5 text-xs capitalize transition-colors"
          :class="props.statusFilter === option ? 'border-foreground bg-foreground text-background' : 'border-border/70 bg-background/70'"
          @click="emit('update:statusFilter', option)"
        >
          {{ option === 'all' ? t('common.labels.all') : option.replace('_', ' ') }}
        </button>
      </div>
    </CardHeader>

    <CardContent>
      <ScrollArea class="h-[72vh] pr-3">
        <div class="space-y-3">
          <button
            v-for="item in props.items"
            :key="item.id"
            type="button"
            :class="
              cn(
                'w-full rounded-2xl border p-4 text-left transition-colors',
                props.selectedTaskId === item.id
                  ? 'border-foreground bg-foreground text-background'
                  : 'border-border/70 bg-background/70 hover:bg-background',
              )
            "
            @click="emit('select', item.id)"
          >
            <div class="flex items-start justify-between gap-4">
              <div class="space-y-2">
                <div class="font-medium leading-tight">{{ item.title }}</div>
                <div :class="cn('text-sm', props.selectedTaskId === item.id ? 'text-background/75' : 'text-muted-foreground')">
                  {{ item.summary }}
                </div>
                <div class="flex flex-wrap gap-2">
                  <Badge :variant="props.selectedTaskId === item.id ? 'secondary' : 'outline'">
                    {{ item.status }}
                  </Badge>
                  <Badge :variant="props.selectedTaskId === item.id ? 'secondary' : 'outline'">
                    {{ item.approvalStatus }}
                  </Badge>
                  <Badge :variant="props.selectedTaskId === item.id ? 'secondary' : 'outline'">
                    {{ item.risk }}
                  </Badge>
                </div>
              </div>
              <div :class="cn('shrink-0 text-xs', props.selectedTaskId === item.id ? 'text-background/70' : 'text-muted-foreground')">
                {{ formatRelativeMinutes(item.updatedAt) }}
              </div>
            </div>
          </button>
        </div>
      </ScrollArea>
    </CardContent>
  </Card>
</template>
