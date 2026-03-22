<script setup lang="ts">
import { computed, onMounted } from 'vue'
import { RouterLink, RouterView, useRoute } from 'vue-router'
import { useI18n } from 'vue-i18n'

import type { AppLocale } from '@/app/i18n/locale'
import { Button } from '@/components/ui/button'
import { useConnectionStore } from '@/entities/session/model/connection.store'
import { useSettingsStore } from '@/entities/settings/model/settings.store'

const route = useRoute()
const connectionStore = useConnectionStore()
const settingsStore = useSettingsStore()
const { t, locale } = useI18n()

const isConsoleRoute = computed(() => route.path.startsWith('/console'))

const navItems = computed(() => [
  { label: t('common.nav.console'), href: '/console' },
  { label: t('common.nav.nodes'), href: '/nodes' },
  { label: t('common.nav.history'), href: '/history' },
  { label: t('common.nav.settings'), href: '/settings' },
])

const localeOptions = computed(() => [
  { value: 'zh-CN' as AppLocale, label: t('common.locales.zhCN') },
  { value: 'en-US' as AppLocale, label: t('common.locales.enUS') },
])

const connectionStatusLabel = computed(() => {
  switch (connectionStore.status) {
    case 'connected':
      return t('common.connection.connected')
    case 'reconnecting':
      return t('common.connection.reconnecting')
    case 'offline':
      return t('common.connection.offline')
    default:
      return t('common.connection.connecting')
  }
})

function handleLocaleChange(event: Event) {
  const nextLocale = (event.target as HTMLSelectElement).value as AppLocale
  settingsStore.setPreferredLocale(nextLocale)
}

onMounted(async () => {
  await connectionStore.initialize()
})
</script>

<template>
  <div :class="isConsoleRoute ? 'flex h-screen flex-col overflow-hidden' : 'min-h-screen'">
    <header class="sticky top-0 z-40 shrink-0 border-b border-white/50 bg-background/85 backdrop-blur-xl">
      <div class="mx-auto flex max-w-[1600px] items-center justify-between gap-6 px-6 py-4">
        <div class="flex items-center gap-4">
          <RouterLink to="/console" class="flex items-center gap-3">
            <div class="flex size-10 items-center justify-center rounded-2xl bg-primary/8 ring-1 ring-primary/15">
              <img alt="ToLaTo" class="size-7" src="/logo/tolato-logo-icon-dark.svg" />
            </div>
            <div>
              <p class="text-[11px] font-semibold uppercase tracking-[0.3em] text-muted-foreground">ToLaTo</p>
              <p class="text-sm font-semibold text-foreground">{{ t('appShell.subtitle') }}</p>
            </div>
          </RouterLink>
          <nav class="hidden items-center gap-2 md:flex">
            <RouterLink v-for="item in navItems" :key="item.href" :to="item.href">
              <Button
                variant="ghost"
                class="rounded-full px-4"
                :class="route.path.startsWith(item.href) ? 'bg-primary text-primary-foreground hover:bg-primary/90 hover:text-primary-foreground' : ''"
              >
                {{ item.label }}
              </Button>
            </RouterLink>
          </nav>
        </div>
        <div class="hidden items-center gap-3 md:flex">
          <label class="flex items-center gap-2 rounded-full border border-border/80 bg-background/70 px-3 py-1.5 text-xs text-muted-foreground">
            <span>{{ t('appShell.languageLabel') }}</span>
            <select :value="locale" class="bg-transparent text-foreground outline-none" @change="handleLocaleChange">
              <option v-for="option in localeOptions" :key="option.value" :value="option.value">{{ option.label }}</option>
            </select>
          </label>
          <div class="rounded-full border border-border/80 bg-background/70 px-3 py-1.5 text-xs text-muted-foreground">
            {{ connectionStatusLabel }}
          </div>
        </div>
      </div>
    </header>

    <main
      class="mx-auto w-full max-w-[1600px] px-4 py-6 md:px-6"
      :class="isConsoleRoute ? 'flex-1 min-h-0 overflow-hidden' : ''"
    >
      <RouterView v-slot="{ Component }">
        <component :is="Component" />
      </RouterView>
    </main>

    <AppToastViewport />
  </div>
</template>
