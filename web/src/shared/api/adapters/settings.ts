import { t } from '@/app/i18n'
import { httpClient } from '@/shared/api/http-client'
import { appEnv } from '@/shared/config/env'
import { mockSettings } from '@/shared/mock/settings'
import type {
  ChangePasswordInput,
  ListModelOptionsInput,
  ModelOption,
  ModelConfigTestResult,
  SettingsState,
} from '@/shared/types/settings'

function mapModelConfig(
  modelConfig: Partial<SettingsState['modelConfig']>,
  fallback?: Partial<SettingsState['modelConfig']>,
): SettingsState['modelConfig'] {
  return {
    provider: String(modelConfig.provider ?? fallback?.provider ?? 'openai')
      .trim()
      .toLowerCase(),
    model: modelConfig.model ?? fallback?.model ?? 'gpt-5.4',
    endpoint:
      modelConfig.endpoint ?? fallback?.endpoint ?? 'https://api.openai.com/v1',
    apiKey: modelConfig.apiKey ?? fallback?.apiKey ?? '',
    temperature: modelConfig.temperature ?? fallback?.temperature ?? 0.2,
    hasApiKey:
      modelConfig.hasApiKey ??
      fallback?.hasApiKey ??
      Boolean((modelConfig.apiKey ?? fallback?.apiKey ?? '').trim()),
    approvalMode:
      modelConfig.approvalMode ?? fallback?.approvalMode ?? 'balanced',
  }
}

export async function getSettings(): Promise<SettingsState> {
  if (appEnv.useMock) {
    return structuredClone(mockSettings)
  }

  const [modelConfig, accountSecurity, preferences] = await Promise.all([
    httpClient<SettingsState['modelConfig']>('/api/v1/settings/model-config'),
    httpClient<SettingsState['accountSecurity']>(
      '/api/v1/settings/account-security',
    ),
    httpClient<SettingsState['preferences']>('/api/v1/settings/preferences'),
  ])

  return {
    modelConfig: mapModelConfig(modelConfig),
    accountSecurity,
    preferences,
  }
}

export async function saveSettings(
  nextSettings: SettingsState,
): Promise<SettingsState> {
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
    httpClient<SettingsState['accountSecurity']>(
      '/api/v1/settings/account-security',
      {
        method: 'PUT',
        body: nextSettings.accountSecurity,
      },
    ),
    httpClient<SettingsState['preferences']>('/api/v1/settings/preferences', {
      method: 'PUT',
      body: nextSettings.preferences,
    }),
  ])

  return {
    modelConfig: mapModelConfig(modelConfig, nextSettings.modelConfig),
    accountSecurity,
    preferences,
  }
}

export async function testModelConfig(
  modelConfig: SettingsState['modelConfig'],
): Promise<ModelConfigTestResult> {
  if (appEnv.useMock) {
    return {
      ok: true,
      message: `mock connection test succeeded for ${modelConfig.provider}/${modelConfig.model}`,
    }
  }

  return httpClient<ModelConfigTestResult>(
    '/api/v1/settings/model-config/test',
    {
      method: 'POST',
      body: modelConfig,
    },
  )
}

export async function listModelOptions(
  input: ListModelOptionsInput,
): Promise<ModelOption[]> {
  if (appEnv.useMock) {
    return [
      { id: 'gpt-5.4', label: 'gpt-5.4' },
      { id: 'gpt-5.4-mini', label: 'gpt-5.4-mini' },
      { id: 'gpt-4.1', label: 'gpt-4.1' },
    ]
  }

  const endpoint = input.endpoint.trim().replace(/\/+$/, '')
  const apiKey = input.apiKey.trim()
  if (!endpoint || !apiKey) {
    return []
  }

  const response = await fetch(`${endpoint}/models`, {
    method: 'GET',
    headers: {
      Authorization: `Bearer ${apiKey}`,
    },
  })

  let payload: {
    data?: Array<{ id?: string | null }>
    error?: { message?: string }
  } | null = null
  try {
    payload = (await response.json()) as {
      data?: Array<{ id?: string | null }>
      error?: { message?: string }
    }
  } catch {
    payload = null
  }

  if (!response.ok) {
    throw new Error(
      payload?.error?.message ||
        `${response.status} ${response.statusText}`.trim() ||
        t('errors.failedToLoadModels'),
    )
  }

  const seen = new Set<string>()
  return (payload?.data ?? [])
    .map((item) => String(item.id ?? '').trim())
    .filter((item) => {
      if (!item || seen.has(item)) {
        return false
      }
      seen.add(item)
      return true
    })
    .sort((a, b) => a.localeCompare(b))
    .map((item) => ({ id: item, label: item }))
}

export async function changePassword(
  input: ChangePasswordInput,
): Promise<void> {
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
