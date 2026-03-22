export const APP_LOCALES = ['zh-CN', 'en-US'] as const

export type AppLocale = (typeof APP_LOCALES)[number]

export const DEFAULT_LOCALE: AppLocale = 'zh-CN'
export const LOCALE_STORAGE_KEY = 'tolato.locale'

export function isSupportedLocale(value: string | null | undefined): value is AppLocale {
  return APP_LOCALES.includes(value as AppLocale)
}

export function getStoredLocale(): AppLocale | null {
  if (typeof window === 'undefined') {
    return null
  }

  const stored = window.localStorage.getItem(LOCALE_STORAGE_KEY)
  return isSupportedLocale(stored) ? stored : null
}

export function resolveBrowserLocale(language?: string | null): AppLocale {
  if (!language) {
    return DEFAULT_LOCALE
  }

  const normalized = language.toLowerCase()
  if (normalized.startsWith('zh')) {
    return 'zh-CN'
  }

  if (normalized.startsWith('en')) {
    return 'en-US'
  }

  return DEFAULT_LOCALE
}

export function resolveInitialLocale(): AppLocale {
  const stored = getStoredLocale()
  if (stored) {
    return stored
  }

  if (typeof navigator !== 'undefined') {
    return resolveBrowserLocale(navigator.language)
  }

  return DEFAULT_LOCALE
}

export function persistLocale(locale: AppLocale) {
  if (typeof window === 'undefined') {
    return
  }

  window.localStorage.setItem(LOCALE_STORAGE_KEY, locale)
}

export function applyDocumentLanguage(locale: AppLocale) {
  if (typeof document === 'undefined') {
    return
  }

  document.documentElement.lang = locale
}

export function syncLocaleSideEffects(locale: AppLocale) {
  persistLocale(locale)
  applyDocumentLanguage(locale)
}
