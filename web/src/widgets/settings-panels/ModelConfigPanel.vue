<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import type { ModelConfig, ModelOption } from '@/shared/types/settings'

const props = defineProps<{
  modelConfig: ModelConfig
  modelOptions: ModelOption[]
  modelOptionsLoaded?: boolean
  loadingModels?: boolean
  modelOptionsError?: string | null
  canRefreshModels?: boolean
  testing?: boolean
}>()

const emit = defineEmits<{
  (e: 'update:modelConfig', value: ModelConfig): void
  (e: 'refresh-models'): void
  (e: 'test'): void
}>()

const { t } = useI18n()

const providerOptions = computed(() => [
  { value: 'openai', label: t('settingsPanel.modelConfig.providers.openai') },
  { value: 'devloop', label: t('settingsPanel.modelConfig.providers.devloop') },
])

const resolvedModelOptions = computed(() => {
  const currentModel = props.modelConfig.model.trim()
  if (!props.modelOptionsLoaded) {
    return currentModel ? [{ id: currentModel, label: currentModel }] : []
  }

  const items = [...props.modelOptions]
  if (currentModel && !items.some((item) => item.id === currentModel)) {
    items.unshift({ id: currentModel, label: currentModel })
  }
  return items
})

const displayedModelValue = computed(() => props.modelConfig.model.trim())
</script>

<template>
  <Card class="glass-panel border-border/70 rounded-2xl">
    <CardHeader>
      <CardTitle>{{ t('settingsPanel.modelConfig.title') }}</CardTitle>
    </CardHeader>

    <CardContent class="space-y-4">
      <div class="max-w-xl space-y-2">
        <Label>{{ t('settingsPanel.modelConfig.provider') }}</Label>
        <select
          class="flex h-11 w-full rounded-md border border-input bg-background px-3 text-sm shadow-sm outline-none"
          :value="props.modelConfig.provider"
          @change="
            (event) =>
              emit('update:modelConfig', {
                ...props.modelConfig,
                provider: (event.target as HTMLSelectElement).value,
              })
          "
        >
          <option
            v-for="option in providerOptions"
            :key="option.value"
            :value="option.value"
          >
            {{ option.label }}
          </option>
        </select>
      </div>

      <div class="grid gap-4 md:grid-cols-2">
        <div class="space-y-2">
          <Label>{{ t('settingsPanel.modelConfig.endpoint') }}</Label>
          <Input
            :model-value="props.modelConfig.endpoint"
            :placeholder="t('settingsPanel.modelConfig.endpointPlaceholder')"
            @update:model-value="
              (value) =>
                emit('update:modelConfig', {
                  ...props.modelConfig,
                  endpoint: String(value),
                })
            "
          />
        </div>
        <div class="space-y-2">
          <Label>{{ t('settingsPanel.modelConfig.apiKey') }}</Label>
          <Input
            type="password"
            autocomplete="off"
            :model-value="props.modelConfig.apiKey"
            :placeholder="
              props.modelConfig.hasApiKey
                ? t('settingsPanel.modelConfig.apiKeyRetained')
                : t('settingsPanel.modelConfig.apiKeyPlaceholder')
            "
            @update:model-value="
              (value) =>
                emit('update:modelConfig', {
                  ...props.modelConfig,
                  apiKey: String(value),
                  hasApiKey:
                    props.modelConfig.hasApiKey ||
                    String(value).trim().length > 0,
                })
            "
          />
          <p class="text-xs text-muted-foreground">
            {{
              props.modelConfig.hasApiKey
                ? t('settingsPanel.modelConfig.apiKeyHelpRetained')
                : t('settingsPanel.modelConfig.apiKeyHelp')
            }}
          </p>
        </div>
      </div>

      <div class="space-y-2">
        <div class="flex items-center justify-between gap-3">
          <Label>{{ t('settingsPanel.modelConfig.model') }}</Label>
          <Button
            size="sm"
            variant="outline"
            :disabled="props.loadingModels || !props.canRefreshModels"
            @click="emit('refresh-models')"
          >
            {{
              props.loadingModels
                ? t('settingsPanel.modelConfig.loadingModels')
                : t('settingsPanel.modelConfig.refreshModels')
            }}
          </Button>
        </div>
        <select
          class="flex h-11 w-full rounded-md border border-input bg-background px-3 text-sm shadow-sm outline-none disabled:cursor-not-allowed disabled:opacity-60"
          :value="displayedModelValue"
          :disabled="props.loadingModels || resolvedModelOptions.length === 0"
          @change="
            (event) =>
              emit('update:modelConfig', {
                ...props.modelConfig,
                model: (event.target as HTMLSelectElement).value,
              })
          "
        >
          <option v-if="resolvedModelOptions.length === 0" value="">
            {{
              props.modelConfig.model.trim()
                ? props.modelConfig.model.trim()
                : props.modelOptionsLoaded
                  ? t('settingsPanel.modelConfig.modelsEmpty')
                  : props.canRefreshModels
                    ? t('settingsPanel.modelConfig.modelsPending')
                    : t('settingsPanel.modelConfig.modelsUnavailable')
            }}
          </option>
          <option
            v-for="option in resolvedModelOptions"
            :key="option.id"
            :value="option.id"
          >
            {{ option.label }}
          </option>
        </select>
        <p v-if="props.modelOptionsError" class="text-xs text-brand-danger">
          {{ props.modelOptionsError }}
        </p>
        <p v-else class="text-xs text-muted-foreground">
          {{ t('settingsPanel.modelConfig.modelHelp') }}
        </p>
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
            @update:model-value="
              (value) =>
                emit('update:modelConfig', {
                  ...props.modelConfig,
                  temperature: Number(value),
                })
            "
          />
        </div>
        <div class="space-y-2">
          <Label>{{ t('settingsPanel.modelConfig.approvalMode') }}</Label>
          <select
            class="flex h-11 w-full rounded-md border border-input bg-background px-3 text-sm shadow-sm outline-none"
            :value="props.modelConfig.approvalMode"
            @change="
              (event) =>
                emit('update:modelConfig', {
                  ...props.modelConfig,
                  approvalMode: (event.target as HTMLSelectElement)
                    .value as ModelConfig['approvalMode'],
                })
            "
          >
            <option value="safe">safe</option>
            <option value="balanced">balanced</option>
            <option value="strict">strict</option>
          </select>
        </div>
      </div>

      <div class="flex flex-wrap gap-2">
        <Badge variant="secondary">{{
          t('settingsPanel.modelConfig.openaiCompatible')
        }}</Badge>
        <Badge variant="outline">{{
          props.modelConfig.hasApiKey
            ? t('settingsPanel.modelConfig.apiKeyConfigured')
            : t('settingsPanel.modelConfig.mockReady')
        }}</Badge>
      </div>

      <div class="flex justify-end">
        <Button
          variant="outline"
          :disabled="props.testing"
          @click="emit('test')"
        >
          {{
            props.testing
              ? t('settingsPanel.modelConfig.testing')
              : t('settingsPanel.modelConfig.testConnection')
          }}
        </Button>
      </div>
    </CardContent>
  </Card>
</template>
