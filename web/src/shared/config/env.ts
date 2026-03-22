export const appEnv = {
  apiBaseUrl: import.meta.env.VITE_API_BASE_URL || 'http://localhost:8080',
  useMock: import.meta.env.VITE_USE_MOCK === 'true',
}
