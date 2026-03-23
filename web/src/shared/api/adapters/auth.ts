import { ofetch } from 'ofetch'

import { appEnv } from '@/shared/config/env'
import { toErrorMessage } from '@/shared/lib/errors'
import type { AuthSession, LoginInput } from '@/shared/types/auth'

const anonymousHttpClient = ofetch.create({
  baseURL: appEnv.apiBaseUrl,
  credentials: 'include',
  async onResponseError({ response }) {
    const payload = response._data as { error?: string; message?: string } | undefined
    throw new Error(
      payload?.error ||
      payload?.message ||
      `${response.status} ${response.statusText}`.trim() ||
      toErrorMessage(undefined, 'Request failed'),
    )
  },
})

export async function login(input: LoginInput): Promise<AuthSession> {
  if (appEnv.useMock) {
    return {
      userId: input.username.trim() || 'mock-user',
      sessionId: 'mock-session',
      token: 'mock-token',
    }
  }

  return anonymousHttpClient<AuthSession>('/api/v1/auth/login', {
    method: 'POST',
    body: input,
  })
}
