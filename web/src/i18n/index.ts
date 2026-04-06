import { createI18n } from 'vue-i18n'
import en from './en'
import zhCN from './zh-CN'

const savedLocale = localStorage.getItem('locale') || 'en'

const i18n = createI18n({
  legacy: false,
  locale: savedLocale,
  fallbackLocale: 'en',
  messages: {
    en,
    'zh-CN': zhCN,
  },
})

export function setLocale(locale: string) {
  ;(i18n.global.locale as any).value = locale
  localStorage.setItem('locale', locale)
}

export function getLocale(): string {
  return (i18n.global.locale as any).value
}

export default i18n
