<script setup lang="ts">
import { computed } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { MessageSquare, Server, FileText, Settings, Zap, Sun, Moon, Languages, Github } from 'lucide-vue-next'
import { useTheme } from '@/composables/useTheme'
import { setLocale, getLocale } from '@/i18n'

const REPO_URL = 'https://github.com/momaek/tolato'
const appVersion = __APP_VERSION__
const releaseUrl = computed(() =>
  /^v\d/.test(appVersion) ? `${REPO_URL}/releases/tag/${appVersion}` : `${REPO_URL}/releases`,
)

const { t } = useI18n()
const { theme, toggleTheme } = useTheme()
import { Separator } from '@/components/ui/separator'
import { ScrollArea } from '@/components/ui/scroll-area'
import ConversationList from './ConversationList.vue'

const route = useRoute()
const router = useRouter()

const navItems = computed(() => [
  { label: t('sidebar.chat'), icon: MessageSquare, path: '/chat' },
  { label: t('sidebar.nodes'), icon: Server, path: '/nodes' },
  { label: t('sidebar.auditLog'), icon: FileText, path: '/audit' },
  { label: t('sidebar.settings'), icon: Settings, path: '/settings' },
])

const isActive = (path: string) => {
  if (path === '/chat') {
    return route.path === '/chat' || route.path.startsWith('/chat/')
  }
  if (path === '/nodes') {
    return route.path === '/nodes' || route.path.startsWith('/nodes/')
  }
  return route.path === path
}

const isChatRoute = computed(() => route.path === '/chat' || route.path.startsWith('/chat/'))

function navigate(path: string) {
  router.push(path)
}

function toggleLocale() {
  const current = getLocale()
  setLocale(current === 'en' ? 'zh-CN' : 'en')
}
</script>

<template>
  <aside
    class="flex h-full w-[280px] flex-col border-r"
    style="background-color: var(--sidebar); border-color: var(--sidebar-border)"
  >
    <!-- Logo -->
    <div class="flex items-center gap-2 px-5 py-5">
      <div
        class="flex h-8 w-8 items-center justify-center rounded-lg"
        style="background-color: var(--primary)"
      >
        <Zap class="h-4 w-4" style="color: var(--primary-foreground)" />
      </div>
      <span class="text-lg font-semibold" style="color: var(--sidebar-foreground)">
        Tolato
      </span>
    </div>

    <!-- Navigation -->
    <nav class="flex flex-col gap-1 px-3">
      <button
        v-for="item in navItems"
        :key="item.path"
        class="flex items-center gap-3 px-3 py-2 text-sm font-medium transition-colors"
        :class="
          isActive(item.path)
            ? 'text-primary-foreground'
            : 'text-sidebar-foreground/70 hover:text-sidebar-foreground hover:bg-sidebar-accent'
        "
        :style="{
          borderRadius: 'var(--radius-pill, 999px)',
          backgroundColor: isActive(item.path) ? 'var(--primary)' : undefined,
          color: isActive(item.path) ? 'var(--primary-foreground)' : undefined,
        }"
        @click="navigate(item.path)"
      >
        <component :is="item.icon" class="h-4 w-4" />
        {{ item.label }}
      </button>
    </nav>

    <Separator class="my-3 mx-3" style="background-color: var(--sidebar-border)" />

    <!-- Conversation list (only on chat routes).
         min-h-0 is load-bearing: a flex child defaults to min-height:auto,
         which lets ScrollArea grow to its content height and pushes the
         bottom controls off-screen. With min-h-0 the flex layout can shrink
         this region and the inner ScrollAreaViewport actually scrolls.
         When the list is hidden on non-chat routes we collapse this region
         entirely so the bottom controls hug the separator instead of
         leaving a tall blank gap. -->
    <ScrollArea v-if="isChatRoute" class="min-h-0 flex-1 px-3">
      <ConversationList />
    </ScrollArea>

    <!-- Bottom controls. shrink-0 keeps flex from squeezing them when the
         conversation list is long; mt-auto pins them to the bottom on
         non-chat routes where the ScrollArea isn't rendered (so there's no
         flex-1 sibling consuming the remaining space). -->
    <div class="mt-auto shrink-0 px-3 pb-4 pt-1 space-y-1">
      <button
        class="flex w-full items-center gap-3 rounded-lg px-3 py-2 text-sm transition-colors"
        style="color: var(--sidebar-foreground); opacity: 0.7"
        @click="toggleLocale"
      >
        <Languages class="h-4 w-4" />
        {{ getLocale() === 'en' ? '中文' : 'English' }}
      </button>
      <button
        class="flex w-full items-center gap-3 rounded-lg px-3 py-2 text-sm transition-colors"
        style="color: var(--sidebar-foreground); opacity: 0.7"
        @click="toggleTheme"
      >
        <Sun v-if="theme === 'dark'" class="h-4 w-4" />
        <Moon v-else class="h-4 w-4" />
        {{ theme === 'dark' ? $t('sidebar.lightMode') : $t('sidebar.darkMode') }}
      </button>

      <div
        class="flex items-center justify-between px-3 pt-2 text-xs"
        style="color: var(--sidebar-foreground); opacity: 0.5"
      >
        <a
          :href="REPO_URL"
          target="_blank"
          rel="noopener noreferrer"
          class="inline-flex items-center transition-opacity hover:opacity-100"
          title="GitHub"
          aria-label="GitHub repository"
        >
          <Github class="h-4 w-4" />
        </a>
        <a
          :href="releaseUrl"
          target="_blank"
          rel="noopener noreferrer"
          class="font-mono transition-opacity hover:opacity-100"
        >
          {{ appVersion }}
        </a>
      </div>
    </div>
  </aside>
</template>
