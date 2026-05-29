import { computed, toValue, type MaybeRefOrGetter } from 'vue'
import { useNow } from '@vueuse/core'
import { useI18n } from 'vue-i18n'

// Reactive "3 minutes ago" / "3 分钟前" string. Ticks every 30s and follows
// the active i18n locale via the native Intl.RelativeTimeFormat — no message
// tables to maintain.
export function useRelativeTime(time: MaybeRefOrGetter<string | number | Date | undefined>) {
  const now = useNow({ interval: 10_000 })
  const { locale } = useI18n()

  return computed(() => {
    const raw = toValue(time)
    if (raw == null) return ''
    const then = new Date(raw).getTime()
    if (Number.isNaN(then)) return ''

    const rtf = new Intl.RelativeTimeFormat(locale.value, { numeric: 'auto' })
    // Seconds elapsed since `then`. Clamp to >= 0 so minor clock skew or a
    // freshly-created message (whose timestamp can sit just ahead of our
    // sampled `now`) never renders as the future ("in 3 seconds" / "3秒后").
    const elapsed = Math.max(0, Math.round((now.value.getTime() - then) / 1000))

    // Pass a NEGATIVE value to format() — negative = past ("3 seconds ago").
    if (elapsed < 60) return rtf.format(-elapsed, 'second')
    const min = Math.round(elapsed / 60)
    if (min < 60) return rtf.format(-min, 'minute')
    const hr = Math.round(elapsed / 3600)
    if (hr < 24) return rtf.format(-hr, 'hour')
    const day = Math.round(elapsed / 86400)
    if (day < 7) return rtf.format(-day, 'day')
    const wk = Math.round(day / 7)
    if (wk < 5) return rtf.format(-wk, 'week')
    const mo = Math.round(day / 30)
    if (mo < 12) return rtf.format(-mo, 'month')
    return rtf.format(-Math.round(day / 365), 'year')
  })
}
