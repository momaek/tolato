import { defineStore } from 'pinia'

import type { AppLocale } from '@/app/i18n/locale'
import { getAppLocale, setAppLocale } from '@/app/i18n'
import {
  changePassword,
  getSettings,
  revokeOtherSessions,
  saveSettings,
  testModelConfig,
} from '@/shared/api/adapters/settings'
import { toErrorMessage } from '@/shared/lib/errors'
import type {
  ChangePasswordInput,
  ModelConfigTestResult,
  SettingsState,
} from '@/shared/types/settings'

export const useSettingsStore = defineStore('settings', {
  state: () => ({
    value: null as SettingsState | null,
    dirty: false,
    loading: false,
    initialized: false,
    saving: false,
    testingModel: false,
    changingPassword: false,
    revokingSessions: false,
    error: null as string | null,
  }),
  actions: {
    async fetch() {
      this.loading = true
      try {
        const value = await getSettings()
        value.preferences.locale = getAppLocale()
        this.value = value
        this.dirty = false
        this.error = null
      } catch (error) {
        this.error = toErrorMessage(error, 'Failed to load settings')
      } finally {
        this.loading = false
        this.initialized = true
      }
    },
    update(nextValue: SettingsState) {
      this.value = nextValue
      this.dirty = true
      this.error = null
      if (nextValue.preferences.locale !== getAppLocale()) {
        setAppLocale(nextValue.preferences.locale)
      }
    },
    setPreferredLocale(locale: AppLocale, markDirty = false) {
      setAppLocale(locale)
      if (!this.value) {
        return
      }

      this.value = {
        ...this.value,
        preferences: {
          ...this.value.preferences,
          locale,
        },
      }

      if (markDirty) {
        this.dirty = true
      }
    },
    async save() {
      if (!this.value) {
        return
      }
      this.saving = true
      try {
        setAppLocale(this.value.preferences.locale)
        this.value = await saveSettings(this.value)
        this.dirty = false
        this.error = null
      } catch (error) {
        this.error = toErrorMessage(error, 'Failed to save settings')
      } finally {
        this.saving = false
      }
    },
    async testCurrentModelConfig() {
      if (!this.value) {
        return null
      }

      this.testingModel = true
      try {
        const result = await testModelConfig(this.value.modelConfig)
        this.error = null
        return result satisfies ModelConfigTestResult
      } catch (error) {
        this.error = toErrorMessage(error, 'Failed to test model config')
        return null
      } finally {
        this.testingModel = false
      }
    },
    async changePassword(input: ChangePasswordInput) {
      this.changingPassword = true
      try {
        await changePassword(input)
        this.error = null
        return true
      } catch (error) {
        this.error = toErrorMessage(error, 'Failed to change password')
        return false
      } finally {
        this.changingPassword = false
      }
    },
    async revokeOtherSessions() {
      this.revokingSessions = true
      try {
        await revokeOtherSessions()
        this.error = null
        return true
      } catch (error) {
        this.error = toErrorMessage(error, 'Failed to revoke other sessions')
        return false
      } finally {
        this.revokingSessions = false
      }
    },
  },
})
