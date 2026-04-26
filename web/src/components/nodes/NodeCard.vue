<script setup lang="ts">
import { ref, computed } from 'vue'
import { useI18n } from 'vue-i18n'
import {
  Pencil,
  Terminal as TerminalIcon,
  Eye,
  MoreHorizontal,
  Copy,
  Check,
  Tag,
  Clock,
  StickyNote,
  ExternalLink,
} from 'lucide-vue-next'
import { Button } from '@/components/ui/button'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from '@/components/ui/tooltip'
import type { NodeListItem } from '@/types/api'

const props = defineProps<{
  node: NodeListItem
  editing: boolean
  editingDraft: string
  aliasSaving: boolean
}>()

const emit = defineEmits<{
  (e: 'startEdit'): void
  (e: 'commitEdit'): void
  (e: 'cancelEdit'): void
  (e: 'update:editingDraft', v: string): void
  (e: 'openTerminal'): void
  (e: 'viewDetail'): void
  (e: 'remove'): void
}>()

const { t } = useI18n()
const ipCopied = ref(false)

function flagEmoji(code?: string): string {
  if (!code || code.length !== 2) return ''
  const A = 0x1f1e6
  const c0 = code.toUpperCase().charCodeAt(0)
  const c1 = code.toUpperCase().charCodeAt(1)
  if (c0 < 65 || c0 > 90 || c1 < 65 || c1 > 90) return ''
  return String.fromCodePoint(A + c0 - 65, A + c1 - 65)
}

// Color thresholds for resource bars: <60 success, <85 warning, else error.
// Use *-foreground variants because the unprefixed tokens are pale Badge backgrounds.
function metricColor(val?: number): string {
  if (val === undefined || val === null) return 'var(--muted-foreground)'
  if (val < 60) return 'var(--color-success-foreground)'
  if (val < 85) return 'var(--color-warning-foreground)'
  return 'var(--color-error-foreground)'
}

function metricPct(val?: number): number {
  if (val === undefined || val === null) return 0
  return Math.max(0, Math.min(100, val))
}

function formatPercent(val?: number): string {
  if (val === undefined || val === null) return '-'
  return `${val.toFixed(1)}%`
}

const lastSeenLabel = computed(() => {
  const iso = props.node.last_heartbeat
  if (!iso) return '-'
  const diffMs = Date.now() - new Date(iso).getTime()
  if (diffMs < 60_000) return t('nodes.justNow')
  const m = Math.floor(diffMs / 60_000)
  if (m < 60) return t('nodes.minutesAgo', { n: m })
  const h = Math.floor(m / 60)
  if (h < 24) return t('nodes.hoursAgo', { n: h })
  const d = Math.floor(h / 24)
  return t('nodes.daysAgo', { n: d })
})

const lastSeenTitle = computed(() => {
  const iso = props.node.last_heartbeat
  return iso ? new Date(iso).toLocaleString() : ''
})

// Subscription / extra summary.
const extra = computed(() => props.node.extra ?? {})

const hasSubscription = computed(() => {
  const e = extra.value
  return !!(e.provider || e.plan || e.expires_at || e.monthly_cost || e.notes || e.renewal_url)
})

const providerLine = computed(() => {
  const parts: string[] = []
  if (extra.value.provider) parts.push(String(extra.value.provider))
  if (extra.value.plan) parts.push(String(extra.value.plan))
  return parts.join(' · ')
})

const costLabel = computed(() => {
  const e = extra.value
  if (e.monthly_cost === undefined || e.monthly_cost === null) return ''
  const currency = (e.currency as string) || 'USD'
  const symbol = currency === 'USD' ? '$' : currency === 'CNY' ? '¥' : currency === 'EUR' ? '€' : ''
  const amount = symbol ? `${symbol}${e.monthly_cost}` : `${e.monthly_cost} ${currency}`
  const cycle = (e.billing_cycle as string) || 'monthly'
  const cycleLabel =
    cycle === 'monthly' ? t('nodes.perMonth')
      : cycle === 'yearly' ? t('nodes.perYear')
      : t('nodes.perCycle', { cycle })
  return `${amount}${cycleLabel}`
})

type ExpiryInfo = {
  text: string
  color: string
  date: string
}

const expiryInfo = computed<ExpiryInfo | null>(() => {
  const raw = extra.value.expires_at as string | undefined
  if (!raw) return null
  const target = new Date(raw)
  if (isNaN(target.getTime())) return null
  const now = new Date()
  // normalize to start-of-day comparison so "today" shows "expires today"
  const startOfDay = (d: Date) => new Date(d.getFullYear(), d.getMonth(), d.getDate())
  const days = Math.round((startOfDay(target).getTime() - startOfDay(now).getTime()) / 86_400_000)
  const dateStr = target.toISOString().slice(0, 10)
  if (days < 0) {
    return { text: t('nodes.expiredAgo', { days: -days }), color: 'var(--color-error-foreground)', date: dateStr }
  }
  if (days === 0) {
    return { text: t('nodes.expiresToday'), color: 'var(--color-error-foreground)', date: dateStr }
  }
  let color = 'var(--muted-foreground)'
  if (days <= 7) color = 'var(--color-error-foreground)'
  else if (days <= 30) color = 'var(--color-warning-foreground)'
  return { text: t('nodes.expiresIn', { days }), color, date: dateStr }
})

async function copyIp() {
  await navigator.clipboard.writeText(props.node.ip)
  ipCopied.value = true
  setTimeout(() => { ipCopied.value = false }, 1500)
}

function onAliasInputMounted(vnode: { el?: HTMLInputElement | null }) {
  vnode.el?.focus()
  vnode.el?.select()
}
</script>

<template>
  <div
    class="group flex flex-col rounded-lg border transition hover:shadow-md"
    style="background-color: var(--card); border-color: var(--border)"
  >
    <!-- Header: status dot + name + menu -->
    <div class="flex items-start gap-2 px-4 pt-4">
      <span
        class="mt-1.5 h-2.5 w-2.5 shrink-0 rounded-full"
        :style="{
          backgroundColor: node.status === 'online' ? 'var(--color-success-foreground)' : 'var(--muted-foreground)',
          boxShadow: node.status === 'online' ? '0 0 0 3px color-mix(in srgb, var(--color-success-foreground) 25%, transparent)' : 'none',
        }"
        :title="node.status === 'online' ? $t('common.online') : $t('common.offline')"
      />
      <div class="min-w-0 flex-1">
        <div v-if="editing" class="flex items-center gap-1">
          <input
            :value="editingDraft"
            :disabled="aliasSaving"
            class="border-input h-7 w-full rounded-md border bg-transparent px-2 text-sm outline-none focus-visible:ring-2 focus-visible:ring-ring/50"
            @input="(e) => emit('update:editingDraft', (e.target as HTMLInputElement).value)"
            @keydown.enter.prevent="emit('commitEdit')"
            @keydown.esc.prevent="emit('cancelEdit')"
            @blur="emit('commitEdit')"
            @vue:mounted="onAliasInputMounted"
          />
        </div>
        <div v-else class="flex items-center gap-1">
          <span class="truncate text-base font-semibold" :title="node.alias || node.name">
            {{ node.alias || node.name }}
          </span>
          <Button
            variant="ghost"
            size="icon"
            class="h-6 w-6 opacity-0 group-hover:opacity-100"
            :title="$t('nodes.editAlias')"
            @click="emit('startEdit')"
          >
            <Pencil class="h-3.5 w-3.5" />
          </Button>
        </div>

        <!-- Subline: IP · region · OS -->
        <div class="mt-0.5 flex flex-wrap items-center gap-x-2 text-xs" style="color: var(--muted-foreground)">
          <button
            class="inline-flex items-center gap-1 font-mono hover:text-foreground"
            :title="$t('nodes.copyIp')"
            @click="copyIp"
          >
            {{ node.ip }}
            <Check v-if="ipCopied" class="h-3 w-3" style="color: var(--color-success-foreground)" />
            <Copy v-else class="h-3 w-3 opacity-0 group-hover:opacity-60" />
          </button>
          <template v-if="node.country_code">
            <span aria-hidden="true">·</span>
            <span>
              <span class="mr-0.5">{{ flagEmoji(node.country_code) }}</span>
              {{ node.city || node.country_code }}
            </span>
          </template>
          <template v-if="node.os">
            <span aria-hidden="true">·</span>
            <span>{{ node.os }}</span>
          </template>
        </div>
        <div v-if="node.asn" class="truncate text-[11px]" style="color: var(--muted-foreground)" :title="node.asn">
          {{ node.asn }}
        </div>
      </div>

      <DropdownMenu>
        <DropdownMenuTrigger as-child>
          <Button variant="ghost" size="icon" class="h-7 w-7">
            <MoreHorizontal class="h-4 w-4" />
          </Button>
        </DropdownMenuTrigger>
        <DropdownMenuContent align="end">
          <DropdownMenuItem @click="emit('startEdit')">
            <Pencil class="mr-2 h-4 w-4" />
            {{ $t('nodes.editAlias') }}
          </DropdownMenuItem>
          <DropdownMenuItem @click="copyIp">
            <Copy class="mr-2 h-4 w-4" />
            {{ $t('nodes.copyIp') }}
          </DropdownMenuItem>
          <DropdownMenuSeparator />
          <DropdownMenuItem
            class="text-destructive focus:text-destructive"
            @click="emit('remove')"
          >
            {{ $t('common.remove') }}
          </DropdownMenuItem>
        </DropdownMenuContent>
      </DropdownMenu>
    </div>

    <!-- Vitals -->
    <div class="grid grid-cols-3 gap-3 px-4 pt-4">
      <div v-for="metric in [
        { label: $t('nodes.cpu'), val: node.cpu },
        { label: $t('nodes.memory'), val: node.memory },
        { label: $t('nodes.disk'), val: node.disk },
      ]" :key="metric.label">
        <div class="flex items-baseline justify-between">
          <span class="text-[10px] uppercase tracking-wider" style="color: var(--muted-foreground)">
            {{ metric.label }}
          </span>
          <span class="text-xs font-medium tabular-nums">{{ formatPercent(metric.val) }}</span>
        </div>
        <div class="mt-1 h-1.5 overflow-hidden rounded-full" style="background-color: var(--secondary)">
          <div
            class="h-full rounded-full transition-all"
            :style="{ width: `${metricPct(metric.val)}%`, backgroundColor: metricColor(metric.val) }"
          />
        </div>
      </div>
    </div>

    <!-- Subscription block (collapsed if empty) -->
    <div
      v-if="hasSubscription"
      class="mx-4 mt-4 space-y-1.5 rounded-md border-t pt-3 text-xs"
      style="border-color: var(--border)"
    >
      <div v-if="providerLine || costLabel" class="flex items-center justify-between gap-2">
        <span class="flex min-w-0 items-center gap-1.5 truncate">
          <Tag class="h-3.5 w-3.5 shrink-0" style="color: var(--muted-foreground)" />
          <span class="truncate">{{ providerLine || '-' }}</span>
        </span>
        <span v-if="costLabel" class="shrink-0 font-medium tabular-nums">{{ costLabel }}</span>
      </div>

      <div v-if="expiryInfo" class="flex items-center gap-1.5" :style="{ color: expiryInfo.color }">
        <Clock class="h-3.5 w-3.5 shrink-0" />
        <span class="font-medium">{{ expiryInfo.text }}</span>
        <span class="opacity-70">· {{ expiryInfo.date }}</span>
        <a
          v-if="extra.renewal_url"
          :href="extra.renewal_url as string"
          target="_blank"
          rel="noopener"
          class="ml-auto inline-flex items-center gap-0.5 underline-offset-2 hover:underline"
          :title="$t('nodes.renew')"
        >
          <ExternalLink class="h-3 w-3" />
          {{ $t('nodes.renew') }}
        </a>
      </div>

      <div v-if="extra.notes" class="flex items-start gap-1.5" style="color: var(--muted-foreground)">
        <StickyNote class="mt-0.5 h-3.5 w-3.5 shrink-0" />
        <TooltipProvider :delay-duration="300">
          <Tooltip>
            <TooltipTrigger as-child>
              <span class="line-clamp-1 cursor-default">{{ extra.notes }}</span>
            </TooltipTrigger>
            <TooltipContent class="max-w-xs whitespace-pre-wrap">
              {{ extra.notes }}
            </TooltipContent>
          </Tooltip>
        </TooltipProvider>
      </div>
    </div>

    <!-- Footer: heartbeat + actions -->
    <div class="mt-auto flex items-center justify-between gap-2 px-4 py-3 pt-4">
      <span class="text-xs" style="color: var(--muted-foreground)" :title="lastSeenTitle">
        {{ $t('nodes.lastSeen', { time: lastSeenLabel }) }}
      </span>
      <div class="flex items-center gap-1">
        <Button
          variant="ghost"
          size="icon"
          class="h-8 w-8"
          :title="$t('nodes.openTerminal')"
          :disabled="node.status !== 'online'"
          @click="emit('openTerminal')"
        >
          <TerminalIcon class="h-4 w-4" />
        </Button>
        <Button
          variant="ghost"
          size="icon"
          class="h-8 w-8"
          :title="$t('nodes.viewDetail')"
          @click="emit('viewDetail')"
        >
          <Eye class="h-4 w-4" />
        </Button>
      </div>
    </div>
  </div>
</template>
