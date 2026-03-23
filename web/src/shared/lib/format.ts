import { getAppLocale } from '@/app/i18n'
import type { AppLocale } from '@/app/i18n/locale'

const EMPTY_DISPLAY_VALUES = new Set([
  '',
  '-',
  'unknown',
  'Unknown',
  'n/a',
  'N/A',
  'none',
  'None',
  'null',
  'undefined',
])

export function formatDateTime(
  value: string,
  locale: AppLocale = getAppLocale(),
): string {
  return new Intl.DateTimeFormat(locale, {
    month: 'short',
    day: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
  }).format(new Date(value))
}

export function formatRelativeMinutes(
  value: string,
  locale: AppLocale = getAppLocale(),
): string {
  const diff = Math.max(
    0,
    Math.round((Date.now() - new Date(value).getTime()) / 60000),
  )
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

export function normalizePercentValue(value: number): number {
  if (!Number.isFinite(value)) {
    return 0
  }

  const scaled = value >= 0 && value <= 1 ? value * 100 : value
  return clampNumber(Math.round(scaled))
}

export function formatPercent(value: number): string {
  return `${normalizePercentValue(value)}%`
}

export function hasDisplayValue(
  value: string | null | undefined,
): value is string {
  const normalized = value?.trim()
  return Boolean(normalized && !EMPTY_DISPLAY_VALUES.has(normalized))
}

export function displayValue(
  value: string | null | undefined,
  fallback = '',
): string {
  return hasDisplayValue(value) ? value.trim() : fallback
}

export function joinDisplayParts(
  parts: Array<string | null | undefined>,
  separator = ' · ',
): string {
  return parts
    .map((part) => part?.trim())
    .filter((part): part is string => Boolean(part))
    .join(separator)
}
