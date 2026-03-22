import { getAppLocale } from '@/app/i18n'
import type { AppLocale } from '@/app/i18n/locale'

export function formatDateTime(value: string, locale: AppLocale = getAppLocale()): string {
  return new Intl.DateTimeFormat(locale, {
    month: 'short',
    day: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
  }).format(new Date(value))
}

export function formatRelativeMinutes(value: string, locale: AppLocale = getAppLocale()): string {
  const diff = Math.max(0, Math.round((Date.now() - new Date(value).getTime()) / 60000))
  const rtf = new Intl.RelativeTimeFormat(locale, { numeric: 'auto' })

  if (diff < 1) {
    return locale === 'zh-CN' ? '刚刚' : 'just now'
  }

  if (diff < 60) {
    return rtf.format(-diff, 'minute')
  }

  const hours = Math.floor(diff / 60)
  if (hours < 24) {
    return rtf.format(-hours, 'hour')
  }

  const days = Math.floor(hours / 24)
  return rtf.format(-days, 'day')
}

export function clampNumber(value: number, min = 0, max = 100): number {
  return Math.min(max, Math.max(min, value))
}
