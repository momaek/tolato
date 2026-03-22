import { httpClient } from '@/shared/api/http-client'
import { appEnv } from '@/shared/config/env'
import { mockSettings } from '@/shared/mock/settings'
import type { ChangePasswordInput, ModelConfigTestResult, SettingsState } from '@/shared/types/settings'

export async function getSettings(): Promise<SettingsState> {
  if (appEnv.useMock) {
    return structuredClone(mockSettings)
  }

  const [modelConfig, accountSecurity, preferences] = await Promise.all([
    httpClient<SettingsState['modelConfig']>('/api/v1/settings/model-config'),
    httpClient<SettingsState['accountSecurity']>('/api/v1/settings/account-security'),
    httpClient<SettingsState['preferences']>('/api/v1/settings/preferences'),
  ])

  return {
    modelConfig,
    accountSecurity,
    preferences,
  }
}

export async function saveSettings(nextSettings: SettingsState): Promise<SettingsState> {
  if (appEnv.useMock) {
    Object.assign(mockSettings.modelConfig, nextSettings.modelConfig)
    Object.assign(mockSettings.accountSecurity, nextSettings.accountSecurity)
    Object.assign(mockSettings.preferences, nextSettings.preferences)
    return structuredClone(mockSettings)
  }

  const [modelConfig, accountSecurity, preferences] = await Promise.all([
    httpClient<SettingsState['modelConfig']>('/api/v1/settings/model-config', {
      method: 'PUT',
      body: nextSettings.modelConfig,
    }),
    httpClient<SettingsState['accountSecurity']>('/api/v1/settings/account-security', {
      method: 'PUT',
      body: nextSettings.accountSecurity,
    }),
    httpClient<SettingsState['preferences']>('/api/v1/settings/preferences', {
      method: 'PUT',
      body: nextSettings.preferences,
    }),
  ])

  return {
    modelConfig,
    accountSecurity,
    preferences,
  }
}

export async function testModelConfig(modelConfig: SettingsState['modelConfig']): Promise<ModelConfigTestResult> {
  if (appEnv.useMock) {
    return {
      ok: true,
      message: `mock connection test succeeded for ${modelConfig.provider}/${modelConfig.model}`,
    }
  }

  return httpClient<ModelConfigTestResult>('/api/v1/settings/model-config/test', {
    method: 'POST',
    body: modelConfig,
  })
}

export async function changePassword(input: ChangePasswordInput): Promise<void> {
  if (appEnv.useMock) {
    return
  }

  await httpClient('/api/v1/settings/password/change', {
    method: 'POST',
    body: input,
  })
}

export async function revokeOtherSessions(): Promise<void> {
  if (appEnv.useMock) {
    return
  }

  await httpClient('/api/v1/settings/sessions/revoke-others', {
    method: 'POST',
  })
}
