<script setup lang="ts">
import { computed } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { MessageSquare, Server, FileText, Settings, Zap, Activity, AlertTriangle, Sun, Moon, Languages } from 'lucide-vue-next'
import { useTheme } from '@/composables/useTheme'
import { setLocale, getLocale } from '@/i18n'

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
  { label: t('sidebar.monitor'), icon: Activity, path: '/monitor' },
  { label: t('sidebar.alerts'), icon: AlertTriangle, path: '/alerts' },
  { label: t('sidebar.auditLog'), icon: FileText, path: '/audit' },
  { label: t('sidebar.settings'), icon: Settings, path: '/settings' },
])

const isActive = (path: string) => {
  if (path === '/chat') {
    return route.path === '/chat' || route.path.startsWith('/chat/')
  }
  if (path === '/monitor') {
    return route.path === '/monitor' || route.path.startsWith('/monitor/')
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

    <!-- Conversation list (only on chat routes) -->
    <ScrollArea v-show="isChatRoute" class="flex-1 px-3">
      <ConversationList />
    </ScrollArea>

    <!-- Bottom controls -->
    <div class="mt-auto px-3 pb-4 space-y-1">
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
    </div>
  </aside>
</template>
