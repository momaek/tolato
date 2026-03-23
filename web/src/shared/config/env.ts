export const appEnv = {
  apiBaseUrl: import.meta.env.VITE_API_BASE_URL || '',
  apiToken: import.meta.env.VITE_API_TOKEN || '',
  useMock: import.meta.env.VITE_USE_MOCK === 'true',
}
