import { ofetch } from 'ofetch'

import { clearStoredAuthSession, getAccessToken } from '@/shared/auth/session'
import { appEnv } from '@/shared/config/env'
import { toErrorMessage } from '@/shared/lib/errors'

export const httpClient = ofetch.create({
  baseURL: appEnv.apiBaseUrl,
  credentials: 'include',
  async onRequest({ options }) {
    const token = getAccessToken()
    if (!token) {
      return
    }

    const headers = new Headers(options.headers ?? {})
    headers.set('Authorization', `Bearer ${token}`)
    options.headers = headers
  },
  async onResponseError({ response }) {
    if (response.status === 401 && !appEnv.useMock && typeof window !== 'undefined') {
      clearStoredAuthSession()
      const next = `${window.location.pathname}${window.location.search}${window.location.hash}`
      const loginURL = new URL('/login', window.location.origin)
      if (next && next !== '/login') {
        loginURL.searchParams.set('next', next)
      }
      window.location.assign(loginURL.toString())
    }

    const payload = response._data as { error?: string; message?: string } | undefined
    throw new Error(
      payload?.error ||
      payload?.message ||
      `${response.status} ${response.statusText}`.trim() ||
      toErrorMessage(undefined, 'Request failed'),
    )
  },
})
