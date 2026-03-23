import type { AppLocale } from '@/app/i18n/locale'

export interface ModelConfig {
  provider: string
  model: string
  endpoint: string
  apiKey: string
  temperature: number
  hasApiKey: boolean
  approvalMode: 'safe' | 'balanced' | 'strict'
}

export interface ModelOption {
  id: string
  label: string
}

export interface AccountSecurity {
  username: string
  lastLoginAt: string
  mfaEnabled: boolean
  auditRetentionDays: number
}

export interface UserPreferences {
  preferredRegion: string
  defaultMode: 'ai_agent' | 'direct_shell'
  locale: AppLocale
  compactTimeline: boolean
  streamMarkdown: boolean
}

export interface SettingsState {
  modelConfig: ModelConfig
  accountSecurity: AccountSecurity
  preferences: UserPreferences
}

export interface ModelConfigTestResult {
  ok: boolean
  message: string
}

export interface ListModelOptionsInput {
  provider: string
  endpoint: string
  apiKey: string
}

export interface ChangePasswordInput {
  currentPassword: string
  newPassword: string
}
