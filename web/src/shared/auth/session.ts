import { appEnv } from '@/shared/config/env'
import type { AuthSession } from '@/shared/types/auth'

const AUTH_STORAGE_KEY = 'tolato.auth.session'

function cloneSession(session: AuthSession | null) {
  return session ? { ...session } : null
}

function sanitizeSession(value: unknown): AuthSession | null {
  if (!value || typeof value !== 'object') {
    return null
  }

  const token = Reflect.get(value, 'token')
  if (typeof token !== 'string' || !token.trim()) {
    return null
  }

  const userId = Reflect.get(value, 'userId')
  const sessionId = Reflect.get(value, 'sessionId')

  return {
    token: token.trim(),
    userId: typeof userId === 'string' ? userId : '',
    sessionId: typeof sessionId === 'string' ? sessionId : '',
  }
}

function readStoredSession() {
  if (typeof window === 'undefined') {
    return null
  }

  const raw = window.localStorage.getItem(AUTH_STORAGE_KEY)
  if (!raw) {
    return null
  }

  try {
    return sanitizeSession(JSON.parse(raw))
  } catch {
    return null
  }
}

function fallbackSession() {
  if (!appEnv.apiToken) {
    return null
  }

  return {
    userId: '',
    sessionId: '',
    token: appEnv.apiToken,
  } satisfies AuthSession
}

let storedSession = readStoredSession()

export function getStoredAuthSession() {
  return cloneSession(storedSession)
}

export function getAuthSession() {
  return cloneSession(storedSession ?? fallbackSession())
}

export function getAccessToken() {
  return getAuthSession()?.token ?? ''
}

export function hasAccessToken() {
  return Boolean(getAccessToken())
}

export function setAuthSession(session: AuthSession) {
  storedSession = sanitizeSession(session)
  if (typeof window !== 'undefined' && storedSession) {
    window.localStorage.setItem(AUTH_STORAGE_KEY, JSON.stringify(storedSession))
  }
  return getAuthSession()
}

export function clearStoredAuthSession() {
  storedSession = null
  if (typeof window !== 'undefined') {
    window.localStorage.removeItem(AUTH_STORAGE_KEY)
  }
}
