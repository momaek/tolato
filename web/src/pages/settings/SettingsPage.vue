<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { toast } from 'vue-sonner'

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import { useSettingsStore } from '@/entities/settings/model/settings.store'
import AccountSecurityPanel from '@/widgets/settings-panels/AccountSecurityPanel.vue'
import ModelConfigPanel from '@/widgets/settings-panels/ModelConfigPanel.vue'
import PreferencePanel from '@/widgets/settings-panels/PreferencePanel.vue'
import type { ChangePasswordInput, SettingsState } from '@/shared/types/settings'

const settingsStore = useSettingsStore()
const router = useRouter()
const draft = ref<SettingsState | null>(null)
const { t } = useI18n()

onMounted(async () => {
  if (!settingsStore.value) {
    await settingsStore.fetch()
  }
  draft.value = settingsStore.value ? structuredClone(settingsStore.value) : null
})

const dirty = computed(() => settingsStore.dirty || false)

function updateDraft(mutator: (state: SettingsState) => SettingsState) {
  if (!draft.value) {
    return
  }
  draft.value = mutator(draft.value)
  settingsStore.update(structuredClone(draft.value))
}

async function save() {
  if (!draft.value) {
    return
  }
  settingsStore.update(structuredClone(draft.value))
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
    toast.error(settingsStore.error ?? t('pages.settings.passwordChangeFailure'))
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
</script>

<template>
  <div class="min-h-screen px-4 py-6 md:px-6 xl:px-8">
    <div class="mx-auto flex max-w-7xl flex-col gap-6">
      <section class="glass-panel border-border/70 rounded-[2rem] border p-6">
        <div class="flex flex-col gap-4 lg:flex-row lg:items-end lg:justify-between">
          <div class="space-y-3">
            <div class="flex flex-wrap items-center gap-2">
              <Badge>{{ t('pages.settings.badge') }}</Badge>
              <Badge variant="secondary">{{ t('pages.settings.badgeSecondary') }}</Badge>
            </div>
            <h1 class="text-3xl font-semibold tracking-tight md:text-4xl">{{ t('pages.settings.title') }}</h1>
            <p class="max-w-3xl text-sm leading-6 text-muted-foreground md:text-base">{{ t('pages.settings.description') }}</p>
          </div>

          <div class="flex flex-wrap gap-2">
            <Badge :variant="dirty ? 'default' : 'secondary'">{{ dirty ? t('common.states.dirty') : t('common.states.saved') }}</Badge>
            <Button variant="outline" @click="router.push({ name: 'console' })">{{ t('common.nav.console') }}</Button>
            <Button :disabled="!draft || settingsStore.saving" @click="save">
              {{ settingsStore.saving ? t('common.buttons.saving') : t('common.buttons.saveSettings') }}
            </Button>
          </div>
        </div>
      </section>

      <Card v-if="settingsStore.error" class="border-brand-danger/25 bg-brand-danger/5">
        <CardContent class="p-4 text-sm text-brand-danger">{{ settingsStore.error }}</CardContent>
      </Card>

      <div v-if="draft" class="grid gap-6 xl:grid-cols-1">
        <ModelConfigPanel
          :model-config="draft.modelConfig"
          :testing="settingsStore.testingModel"
          @update:model-config="value => updateDraft(state => ({ ...state, modelConfig: value }))"
          @test="handleTestModelConfig"
        />
        <AccountSecurityPanel
          :account-security="draft.accountSecurity"
          :changing-password="settingsStore.changingPassword"
          :revoking-sessions="settingsStore.revokingSessions"
          @update:account-security="value => updateDraft(state => ({ ...state, accountSecurity: value }))"
          @change-password="handleChangePassword"
          @revoke-others="handleRevokeOtherSessions"
        />
        <PreferencePanel
          :preferences="draft.preferences"
          @update:preferences="value => updateDraft(state => ({ ...state, preferences: value }))"
        />
      </div>

      <Card v-else class="glass-panel border-border/70 rounded-2xl">
        <CardContent class="p-6 text-sm text-muted-foreground">{{ t('common.states.loadingSettings') }}</CardContent>
      </Card>
    </div>
  </div>
</template>
