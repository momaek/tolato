<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { toast } from 'vue-sonner'

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import { useSettingsStore } from '@/entities/settings/model/settings.store'
import { cn } from '@/lib/utils'
import AccountSecurityPanel from '@/widgets/settings-panels/AccountSecurityPanel.vue'
import ModelConfigPanel from '@/widgets/settings-panels/ModelConfigPanel.vue'
import PreferencePanel from '@/widgets/settings-panels/PreferencePanel.vue'
import { listModelOptions } from '@/shared/api/adapters/settings'
import { toErrorMessage } from '@/shared/lib/errors'
import type {
  ChangePasswordInput,
  ModelOption,
  SettingsState,
} from '@/shared/types/settings'

const settingsStore = useSettingsStore()
const router = useRouter()
const draft = ref<SettingsState | null>(null)
const { t } = useI18n()
const activeSection = ref<'model' | 'security' | 'preferences'>('model')
const modelOptions = ref<ModelOption[]>([])
const loadingModels = ref(false)
const modelOptionsError = ref<string | null>(null)
const modelOptionsLoaded = ref(false)

function cloneSettingsState(value: SettingsState | null) {
  if (!value) {
    return null
  }

  return {
    modelConfig: {
      provider: String(value.modelConfig.provider),
      model: String(value.modelConfig.model),
      endpoint: String(value.modelConfig.endpoint),
      apiKey: String(value.modelConfig.apiKey),
      temperature: Number(value.modelConfig.temperature),
      hasApiKey: Boolean(value.modelConfig.hasApiKey),
      approvalMode: value.modelConfig.approvalMode,
    },
    accountSecurity: {
      username: String(value.accountSecurity.username),
      lastLoginAt: String(value.accountSecurity.lastLoginAt),
      mfaEnabled: Boolean(value.accountSecurity.mfaEnabled),
      auditRetentionDays: Number(value.accountSecurity.auditRetentionDays),
    },
    preferences: {
      preferredRegion: String(value.preferences.preferredRegion),
      defaultMode: value.preferences.defaultMode,
      locale: value.preferences.locale,
      compactTimeline: Boolean(value.preferences.compactTimeline),
      streamMarkdown: Boolean(value.preferences.streamMarkdown),
    },
  }
}

function snapshotDraftState() {
  return cloneSettingsState(draft.value)
}

watch(
  () => settingsStore.value,
  (value) => {
    draft.value = cloneSettingsState(value)
  },
  { immediate: true },
)

onMounted(async () => {
  if (!settingsStore.initialized || !settingsStore.value) {
    await settingsStore.fetch()
  }
})

const dirty = computed(() => settingsStore.dirty || false)
const showLoadingState = computed(() => settingsStore.loading && !draft.value)
const showEmptyState = computed(
  () => !settingsStore.loading && !draft.value && !settingsStore.error,
)
const sectionItems = computed(() => [
  {
    key: 'model' as const,
    title: t('settingsPanel.modelConfig.title'),
    description: t('settingsPageNav.modelConfig'),
  },
  {
    key: 'security' as const,
    title: t('settingsPanel.accountSecurity.title'),
    description: t('settingsPageNav.accountSecurity'),
  },
  {
    key: 'preferences' as const,
    title: t('settingsPanel.preferences.title'),
    description: t('settingsPageNav.preferences'),
  },
])
const activeSectionMeta = computed(
  () =>
    sectionItems.value.find((item) => item.key === activeSection.value) ??
    sectionItems.value[0],
)
const canLoadModels = computed(() => {
  const config = draft.value?.modelConfig
  if (!config) {
    return false
  }
  if (config.provider !== 'openai') {
    return true
  }
  return Boolean(config.endpoint.trim())
})

function updateDraft(mutator: (state: SettingsState) => SettingsState) {
  if (!draft.value) {
    return
  }
  draft.value = mutator(draft.value)
  const nextValue = snapshotDraftState()
  if (!nextValue) {
    return
  }
  settingsStore.update(nextValue)
}

async function save() {
  const nextValue = snapshotDraftState()
  if (!nextValue) {
    return
  }
  settingsStore.update(nextValue)
  await settingsStore.save()
  if (settingsStore.error) {
    toast.error(settingsStore.error)
    return
  }
  toast.success(t('pages.settings.saveSuccess'))
}

async function handleTestModelConfig() {
  const result = await settingsStore.testCurrentModelConfig()
  if (!result) {
    toast.error(settingsStore.error ?? t('pages.settings.testFailure'))
    return
  }
  toast.success(result.message)
}

async function handleChangePassword(input: ChangePasswordInput) {
  if (!input.currentPassword.trim() || !input.newPassword.trim()) {
    toast.error(t('pages.settings.passwordRequired'))
    return
  }
  if (input.currentPassword === input.newPassword) {
    toast.error(t('pages.settings.passwordSame'))
    return
  }

  const ok = await settingsStore.changePassword(input)
  if (!ok) {
    toast.error(
      settingsStore.error ?? t('pages.settings.passwordChangeFailure'),
    )
    return
  }
  toast.success(t('pages.settings.passwordChangeSuccess'))
}

async function handleRevokeOtherSessions() {
  const ok = await settingsStore.revokeOtherSessions()
  if (!ok) {
    toast.error(settingsStore.error ?? t('pages.settings.revokeFailure'))
    return
  }
  toast.success(t('pages.settings.revokeSuccess'))
}

async function refreshModelOptions() {
  const config = draft.value?.modelConfig
  if (!config) {
    modelOptions.value = []
    modelOptionsError.value = null
    modelOptionsLoaded.value = false
    return
  }
  if (
    config.provider === 'openai' &&
    (!config.endpoint.trim() || !config.apiKey.trim())
  ) {
    modelOptions.value = []
    modelOptionsError.value = !config.endpoint.trim()
      ? 'Endpoint URL is required'
      : 'API Key is required for browser-side model lookup'
    modelOptionsLoaded.value = false
    return
  }

  loadingModels.value = true
  try {
    modelOptions.value = await listModelOptions({
      provider: config.provider,
      endpoint: config.endpoint,
      apiKey: config.apiKey,
    })
    modelOptionsError.value = null
    modelOptionsLoaded.value = true
  } catch (error) {
    modelOptions.value = []
    modelOptionsError.value = toErrorMessage(error, 'Failed to load model list')
    modelOptionsLoaded.value = false
  } finally {
    loadingModels.value = false
  }
}

watch(
  () => {
    const config = draft.value?.modelConfig
    if (!config) {
      return ''
    }
    return [config.provider, config.endpoint.trim(), config.apiKey.trim()].join(
      '|',
    )
  },
  () => {
    modelOptions.value = []
    modelOptionsError.value = null
    modelOptionsLoaded.value = false
  },
  { immediate: true },
)
</script>

<template>
  <div class="min-h-screen px-4 py-6 md:px-6 xl:px-8">
    <div class="mx-auto flex max-w-7xl flex-col gap-6">
      <section class="glass-panel border-border/70 rounded-[2rem] border p-6">
        <div
          class="flex flex-col gap-4 lg:flex-row lg:items-end lg:justify-between"
        >
          <div class="space-y-3">
            <div class="flex flex-wrap items-center gap-2">
              <Badge>{{ t('pages.settings.badge') }}</Badge>
              <Badge variant="secondary">{{
                t('pages.settings.badgeSecondary')
              }}</Badge>
            </div>
            <h1 class="text-3xl font-semibold tracking-tight md:text-4xl">
              {{ t('pages.settings.title') }}
            </h1>
            <p
              class="max-w-3xl text-sm leading-6 text-muted-foreground md:text-base"
            >
              {{ t('pages.settings.description') }}
            </p>
          </div>

          <div class="flex flex-wrap gap-2">
            <Badge :variant="dirty ? 'default' : 'secondary'">{{
              dirty ? t('common.states.dirty') : t('common.states.saved')
            }}</Badge>
            <Button
              variant="outline"
              @click="router.push({ name: 'console' })"
              >{{ t('common.nav.console') }}</Button
            >
            <Button :disabled="!draft || settingsStore.saving" @click="save">
              {{
                settingsStore.saving
                  ? t('common.buttons.saving')
                  : t('common.buttons.saveSettings')
              }}
            </Button>
          </div>
        </div>
      </section>

      <Card
        v-if="settingsStore.error"
        class="border-brand-danger/25 bg-brand-danger/5"
      >
        <CardContent
          class="flex items-center justify-between gap-3 p-4 text-sm text-brand-danger"
        >
          <span>{{ settingsStore.error }}</span>
          <Button size="sm" variant="outline" @click="settingsStore.fetch()">{{
            t('common.buttons.retry')
          }}</Button>
        </CardContent>
      </Card>

      <div v-if="draft" class="grid gap-6 xl:grid-cols-[260px_minmax(0,1fr)]">
        <Card class="glass-panel border-border/70 rounded-2xl xl:self-start">
          <CardContent class="p-4">
            <div class="space-y-2">
              <button
                v-for="item in sectionItems"
                :key="item.key"
                type="button"
                class="w-full rounded-2xl border px-4 py-3 text-left transition-colors"
                :class="
                  cn(
                    item.key === activeSection
                      ? 'border-primary/30 bg-primary/10'
                      : 'border-transparent bg-background/55 hover:border-border hover:bg-background',
                  )
                "
                @click="activeSection = item.key"
              >
                <p class="text-sm font-semibold text-foreground">
                  {{ item.title }}
                </p>
                <p class="mt-1 text-xs leading-5 text-muted-foreground">
                  {{ item.description }}
                </p>
              </button>
            </div>
          </CardContent>
        </Card>

        <div class="space-y-4">
          <Card class="glass-panel border-border/70 rounded-2xl">
            <CardContent class="p-5">
              <p class="text-lg font-semibold text-foreground">
                {{ activeSectionMeta.title }}
              </p>
              <p class="mt-1 text-sm text-muted-foreground">
                {{ activeSectionMeta.description }}
              </p>
            </CardContent>
          </Card>

          <ModelConfigPanel
            v-if="activeSection === 'model'"
            :model-config="draft.modelConfig"
            :model-options="modelOptions"
            :model-options-loaded="modelOptionsLoaded"
            :loading-models="loadingModels"
            :model-options-error="modelOptionsError"
            :can-refresh-models="canLoadModels"
            :testing="settingsStore.testingModel"
            @update:model-config="
              (value) =>
                updateDraft((state) => ({ ...state, modelConfig: value }))
            "
            @refresh-models="refreshModelOptions"
            @test="handleTestModelConfig"
          />
          <AccountSecurityPanel
            v-else-if="activeSection === 'security'"
            :account-security="draft.accountSecurity"
            :changing-password="settingsStore.changingPassword"
            :revoking-sessions="settingsStore.revokingSessions"
            @update:account-security="
              (value) =>
                updateDraft((state) => ({ ...state, accountSecurity: value }))
            "
            @change-password="handleChangePassword"
            @revoke-others="handleRevokeOtherSessions"
          />
          <PreferencePanel
            v-else
            :preferences="draft.preferences"
            @update:preferences="
              (value) =>
                updateDraft((state) => ({ ...state, preferences: value }))
            "
          />
        </div>
      </div>

      <Card
        v-else-if="showLoadingState"
        class="glass-panel border-border/70 rounded-2xl"
      >
        <CardContent class="p-6 text-sm text-muted-foreground">{{
          t('common.states.loadingSettings')
        }}</CardContent>
      </Card>

      <Card
        v-else-if="showEmptyState"
        class="glass-panel border-border/70 rounded-2xl"
      >
        <CardContent class="flex items-center justify-between gap-3 p-6">
          <p class="text-sm text-muted-foreground">
            {{ t('pages.settings.emptyState') }}
          </p>
          <Button size="sm" variant="outline" @click="settingsStore.fetch()">{{
            t('common.buttons.retry')
          }}</Button>
        </CardContent>
      </Card>
    </div>
  </div>
</template>
