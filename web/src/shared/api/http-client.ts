import { ofetch } from 'ofetch'

import { appEnv } from '@/shared/config/env'
import { toErrorMessage } from '@/shared/lib/errors'

export const httpClient = ofetch.create({
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
