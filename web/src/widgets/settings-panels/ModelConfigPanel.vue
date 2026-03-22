<script setup lang="ts">
import { useI18n } from 'vue-i18n'

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import type { ModelConfig } from '@/shared/types/settings'

const props = defineProps<{
  modelConfig: ModelConfig
  testing?: boolean
}>()

const emit = defineEmits<{
  (e: 'update:modelConfig', value: ModelConfig): void
  (e: 'test'): void
}>()

const { t } = useI18n()
</script>

<template>
  <Card class="glass-panel border-border/70 rounded-2xl">
    <CardHeader>
      <CardTitle>{{ t('settingsPanel.modelConfig.title') }}</CardTitle>
    </CardHeader>

    <CardContent class="space-y-4">
      <div class="grid gap-4 md:grid-cols-2">
        <div class="space-y-2">
          <Label>{{ t('settingsPanel.modelConfig.provider') }}</Label>
          <Input
            :model-value="props.modelConfig.provider"
            @update:model-value="value => emit('update:modelConfig', { ...props.modelConfig, provider: String(value) })"
          />
        </div>
        <div class="space-y-2">
          <Label>{{ t('settingsPanel.modelConfig.model') }}</Label>
          <Input
            :model-value="props.modelConfig.model"
            @update:model-value="value => emit('update:modelConfig', { ...props.modelConfig, model: String(value) })"
          />
        </div>
      </div>

      <div class="grid gap-4 md:grid-cols-2">
        <div class="space-y-2">
          <Label>{{ t('settingsPanel.modelConfig.temperature') }}</Label>
          <Input
            type="number"
            step="0.1"
            min="0"
            max="2"
            :model-value="props.modelConfig.temperature"
            @update:model-value="value =>
              emit('update:modelConfig', { ...props.modelConfig, temperature: Number(value) })
            "
          />
        </div>
        <div class="space-y-2">
          <Label>{{ t('settingsPanel.modelConfig.approvalMode') }}</Label>
          <select
            class="flex h-11 w-full rounded-md border border-input bg-background px-3 text-sm shadow-sm outline-none"
            :value="props.modelConfig.approvalMode"
            @change="event => emit('update:modelConfig', { ...props.modelConfig, approvalMode: (event.target as HTMLSelectElement).value as ModelConfig['approvalMode'] })"
          >
            <option value="safe">safe</option>
            <option value="balanced">balanced</option>
            <option value="strict">strict</option>
          </select>
        </div>
      </div>

      <div class="flex flex-wrap gap-2">
        <Badge variant="secondary">{{ t('settingsPanel.modelConfig.openaiCompatible') }}</Badge>
        <Badge variant="outline">{{ t('settingsPanel.modelConfig.mockReady') }}</Badge>
      </div>

      <div class="flex justify-end">
        <Button variant="outline" :disabled="props.testing" @click="emit('test')">
          {{ props.testing ? t('settingsPanel.modelConfig.testing') : t('settingsPanel.modelConfig.testConnection') }}
        </Button>
      </div>
    </CardContent>
  </Card>
</template>
