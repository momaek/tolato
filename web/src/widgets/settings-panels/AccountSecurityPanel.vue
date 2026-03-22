<script setup lang="ts">
import { ref } from 'vue'
import { useI18n } from 'vue-i18n'

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import type { AccountSecurity } from '@/shared/types/settings'

const props = defineProps<{
  accountSecurity: AccountSecurity
  changingPassword?: boolean
  revokingSessions?: boolean
}>()

const emit = defineEmits<{
  (e: 'update:accountSecurity', value: AccountSecurity): void
  (e: 'changePassword', value: { currentPassword: string; newPassword: string }): void
  (e: 'revokeOthers'): void
}>()

const { t } = useI18n()
const currentPassword = ref('')
const newPassword = ref('')

function submitPasswordChange() {
  emit('changePassword', {
    currentPassword: currentPassword.value,
    newPassword: newPassword.value,
  })
  currentPassword.value = ''
  newPassword.value = ''
}
</script>

<template>
  <Card class="glass-panel border-border/70 rounded-2xl">
    <CardHeader>
      <CardTitle>{{ t('settingsPanel.accountSecurity.title') }}</CardTitle>
    </CardHeader>

    <CardContent class="space-y-4">
      <div class="grid gap-4 md:grid-cols-2">
        <div class="space-y-2">
          <Label>{{ t('settingsPanel.accountSecurity.username') }}</Label>
          <Input
            :model-value="props.accountSecurity.username"
            @update:model-value="value => emit('update:accountSecurity', { ...props.accountSecurity, username: String(value) })"
          />
        </div>
        <div class="space-y-2">
          <Label>{{ t('settingsPanel.accountSecurity.lastLogin') }}</Label>
          <Input :model-value="props.accountSecurity.lastLoginAt" disabled />
        </div>
      </div>

      <div class="grid gap-4 md:grid-cols-2">
        <div class="space-y-2">
          <Label>{{ t('settingsPanel.accountSecurity.auditRetentionDays') }}</Label>
          <Input
            type="number"
            min="7"
            step="1"
            :model-value="props.accountSecurity.auditRetentionDays"
            @update:model-value="value =>
              emit('update:accountSecurity', { ...props.accountSecurity, auditRetentionDays: Number(value) })
            "
          />
        </div>
        <div class="space-y-2">
          <Label>{{ t('settingsPanel.accountSecurity.mfa') }}</Label>
          <div class="flex h-11 items-center justify-between rounded-md border border-input bg-background px-3 text-sm">
            <span>{{ props.accountSecurity.mfaEnabled ? t('common.states.enabled') : t('common.states.disabled') }}</span>
            <Badge :variant="props.accountSecurity.mfaEnabled ? 'default' : 'secondary'">
              {{ props.accountSecurity.mfaEnabled ? t('common.states.secure') : t('common.states.attention') }}
            </Badge>
          </div>
        </div>
      </div>

      <div class="grid gap-4 md:grid-cols-2">
        <div class="space-y-2">
          <Label>{{ t('settingsPanel.accountSecurity.currentPassword') }}</Label>
          <Input v-model="currentPassword" type="password" autocomplete="current-password" />
        </div>
        <div class="space-y-2">
          <Label>{{ t('settingsPanel.accountSecurity.newPassword') }}</Label>
          <Input v-model="newPassword" type="password" autocomplete="new-password" />
        </div>
      </div>

      <div class="flex flex-wrap justify-end gap-2">
        <Button
          variant="outline"
          :disabled="props.revokingSessions"
          @click="emit('revokeOthers')"
        >
          {{ props.revokingSessions ? t('settingsPanel.accountSecurity.revoking') : t('settingsPanel.accountSecurity.revokeOthers') }}
        </Button>
        <Button
          :disabled="props.changingPassword || !currentPassword.trim() || !newPassword.trim()"
          @click="submitPasswordChange"
        >
          {{ props.changingPassword ? t('settingsPanel.accountSecurity.changingPassword') : t('settingsPanel.accountSecurity.changePassword') }}
        </Button>
      </div>
    </CardContent>
  </Card>
</template>
