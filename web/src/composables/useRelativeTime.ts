import { computed, toValue, type MaybeRefOrGetter } from 'vue'
import { useNow } from '@vueuse/core'
import { useI18n } from 'vue-i18n'

// Reactive "3 minutes ago" / "3 分钟前" string. Ticks every 30s and follows
// the active i18n locale via the native Intl.RelativeTimeFormat — no message
// tables to maintain.
export function useRelativeTime(time: MaybeRefOrGetter<string | number | Date | undefined>) {
  const now = useNow({ interval: 30_000 })
  const { locale } = useI18n()

  return computed(() => {
    const raw = toValue(time)
    if (raw == null) return ''
    const then = new Date(raw).getTime()
    if (Number.isNaN(then)) return ''

    const rtf = new Intl.RelativeTimeFormat(locale.value, { numeric: 'auto' })
    const sec = Math.round((then - now.value.getTime()) / 1000)
    const abs = Math.abs(sec)

    if (abs < 60) return rtf.format(sec, 'second')
    const min = Math.round(sec / 60)
    if (Math.abs(min) < 60) return rtf.format(min, 'minute')
    const hr = Math.round(sec / 3600)
    if (Math.abs(hr) < 24) return rtf.format(hr, 'hour')
    const day = Math.round(sec / 86400)
    if (Math.abs(day) < 7) return rtf.format(day, 'day')
    const wk = Math.round(day / 7)
    if (Math.abs(wk) < 5) return rtf.format(wk, 'week')
    const mo = Math.round(day / 30)
    if (Math.abs(mo) < 12) return rtf.format(mo, 'month')
    return rtf.format(Math.round(day / 365), 'year')
  })
}
