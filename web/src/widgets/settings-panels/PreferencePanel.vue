<script setup lang="ts">
import { useI18n } from 'vue-i18n'

import type { AppLocale } from '@/app/i18n/locale'
import { Badge } from '@/components/ui/badge'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'
import type { UserPreferences } from '@/shared/types/settings'

const props = defineProps<{
  preferences: UserPreferences
}>()

const emit = defineEmits<{
  (e: 'update:preferences', value: UserPreferences): void
}>()

const { t } = useI18n()
</script>

<template>
  <Card class="glass-panel border-border/70 rounded-2xl">
    <CardHeader>
      <CardTitle>{{ t('settingsPanel.preferences.title') }}</CardTitle>
    </CardHeader>

    <CardContent class="space-y-4">
      <div class="grid gap-4 md:grid-cols-3">
        <div class="space-y-2">
          <Label>{{ t('settingsPanel.preferences.preferredRegion') }}</Label>
          <input
            class="flex h-11 w-full rounded-md border border-input bg-background px-3 text-sm shadow-sm outline-none"
            :value="props.preferences.preferredRegion"
            @input="
              event =>
                emit('update:preferences', {
                  ...props.preferences,
                  preferredRegion: (event.target as HTMLInputElement).value,
                })
            "
          >
        </div>
        <div class="space-y-2">
          <Label>{{ t('settingsPanel.preferences.defaultMode') }}</Label>
          <select
            class="flex h-11 w-full rounded-md border border-input bg-background px-3 text-sm shadow-sm outline-none"
            :value="props.preferences.defaultMode"
            @change="
              event =>
                emit('update:preferences', {
                  ...props.preferences,
                  defaultMode: (event.target as HTMLSelectElement).value as UserPreferences['defaultMode'],
                })
            "
          >
            <option value="ai_agent">ai_agent</option>
            <option value="direct_shell">direct_shell</option>
          </select>
        </div>
        <div class="space-y-2">
          <Label>{{ t('settingsPanel.preferences.locale') }}</Label>
          <select
            class="flex h-11 w-full rounded-md border border-input bg-background px-3 text-sm shadow-sm outline-none"
            :value="props.preferences.locale"
            @change="
              event =>
                emit('update:preferences', {
                  ...props.preferences,
                  locale: (event.target as HTMLSelectElement).value as AppLocale,
                })
            "
          >
            <option value="zh-CN">{{ t('common.locales.zhCN') }}</option>
            <option value="en-US">{{ t('common.locales.enUS') }}</option>
          </select>
        </div>
      </div>

      <div class="grid gap-4 md:grid-cols-2">
        <div class="flex items-center justify-between rounded-xl border border-input bg-background px-4 py-3">
          <div>
            <div class="font-medium">{{ t('settingsPanel.preferences.compactTimeline') }}</div>
            <div class="text-sm text-muted-foreground">{{ t('settingsPanel.preferences.compactTimelineDescription') }}</div>
          </div>
          <Switch
            :model-value="props.preferences.compactTimeline"
            @update:model-value="value => emit('update:preferences', { ...props.preferences, compactTimeline: value })"
          />
        </div>
        <div class="flex items-center justify-between rounded-xl border border-input bg-background px-4 py-3">
          <div>
            <div class="font-medium">{{ t('settingsPanel.preferences.streamMarkdown') }}</div>
            <div class="text-sm text-muted-foreground">{{ t('settingsPanel.preferences.streamMarkdownDescription') }}</div>
          </div>
          <Switch
            :model-value="props.preferences.streamMarkdown"
            @update:model-value="value => emit('update:preferences', { ...props.preferences, streamMarkdown: value })"
          />
        </div>
      </div>

      <div class="flex flex-wrap gap-2">
        <Badge variant="outline">{{ t('settingsPanel.preferences.desktopFirst') }}</Badge>
        <Badge variant="secondary">{{ t('settingsPanel.preferences.mockDriven') }}</Badge>
      </div>
    </CardContent>
  </Card>
</template>
