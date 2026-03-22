import { createI18n } from 'vue-i18n'

import type { AppLocale } from '@/app/i18n/locale'
import { DEFAULT_LOCALE, resolveInitialLocale, syncLocaleSideEffects } from '@/app/i18n/locale'
import enUS from '@/app/i18n/messages/en-US'
import zhCN from '@/app/i18n/messages/zh-CN'

const initialLocale = resolveInitialLocale()

const i18n = createI18n({
  legacy: false,
  locale: initialLocale,
  fallbackLocale: DEFAULT_LOCALE,
  messages: {
    'zh-CN': zhCN,
    'en-US': enUS,
  },
})

export function initializeAppLocale() {
  syncLocaleSideEffects(getAppLocale())
}

export function getAppLocale(): AppLocale {
  return i18n.global.locale.value as AppLocale
}

export function setAppLocale(locale: AppLocale) {
  if (getAppLocale() === locale) {
    syncLocaleSideEffects(locale)
    return
  }

  i18n.global.locale.value = locale
  syncLocaleSideEffects(locale)
}

export default i18n
