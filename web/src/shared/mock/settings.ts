import type { SettingsState } from '@/shared/types/settings'

export const mockSettings: SettingsState = {
  modelConfig: {
    provider: 'openai',
    model: 'gpt-5.4',
    endpoint: 'https://api.openai.com/v1',
    apiKey: '',
    temperature: 0.2,
    hasApiKey: true,
    approvalMode: 'balanced',
  },
  accountSecurity: {
    username: 'admin',
    lastLoginAt: '2026-03-22T07:55:00.000Z',
    mfaEnabled: true,
    auditRetentionDays: 90,
  },
  preferences: {
    preferredRegion: 'Tokyo',
    defaultMode: 'ai_agent',
    locale: 'zh-CN',
    compactTimeline: false,
    streamMarkdown: true,
  },
}
