<script setup lang="ts">
import { computed, ref, onMounted } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { MessageSquare, Server, FileText, Settings, Users, ShieldCheck, Zap, Sun, Moon, Languages, Github, Copy, Check, ExternalLink, LogOut, ChevronsUpDown } from 'lucide-vue-next'
import { useTheme } from '@/composables/useTheme'
import { setLocale, getLocale } from '@/i18n'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { getVersionInfo, type VersionInfo } from '@/services/api'
import { useAppStore } from '@/stores/app'

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
const appStore = useAppStore()

const navItems = computed(() => [
  { label: t('sidebar.chat'), icon: MessageSquare, path: '/chat' },
  { label: t('sidebar.nodes'), icon: Server, path: '/nodes' },
  { label: t('sidebar.auditLog'), icon: FileText, path: '/audit' },
  ...(appStore.isAdmin
    ? [
        { label: t('sidebar.users'), icon: Users, path: '/users' },
        { label: t('sidebar.permissions'), icon: ShieldCheck, path: '/permissions' },
      ]
    : []),
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

// Falls back to a bullet rather than an empty circle on the one frame before
// /auth/me lands on a browser with no cached user.
const userInitial = computed(() => appStore.user?.username?.trim().charAt(0).toUpperCase() || '•')

// --- Update check (best-effort; failure just means no red dot) ---
const versionInfo = ref<VersionInfo | null>(null)
const hasUpdate = computed(() => versionInfo.value?.has_update ?? false)
const copied = ref(false)

onMounted(async () => {
  try {
    versionInfo.value = await getVersionInfo()
  } catch {
    /* version check is best-effort */
  }
})

// The upgrade prompt names the node Tolato runs on when the server knows it
// (server.self_node), so the AI assistant targets the right machine. Falls
// back to a generic phrasing otherwise.
const upgradePrompt = computed(() => {
  const latest = versionInfo.value?.latest || ''
  const node = versionInfo.value?.self_node?.trim()
  return node
    ? t('update.upgradePromptNode', { latest, node })
    : t('update.upgradePrompt', { latest })
})

// Notes link goes through this server's /releases proxy (works where
// github.com is blocked) when the backend resolved a tag; else the GitHub repo.
const notesUrl = computed(() => versionInfo.value?.release_url || releaseUrl.value)

async function copyPrompt() {
  const text = upgradePrompt.value
  let ok = false
  // navigator.clipboard is undefined over plain http (LAN IP) — fall back
  // to the legacy execCommand path the same way CodeBlock does.
  if (navigator.clipboard?.writeText) {
    try {
      await navigator.clipboard.writeText(text)
      ok = true
    } catch {
      /* fall through */
    }
  }
  if (!ok) {
    try {
      const ta = document.createElement('textarea')
      ta.value = text
      ta.style.position = 'fixed'
      ta.style.opacity = '0'
      document.body.appendChild(ta)
      ta.focus()
      ta.select()
      ok = document.execCommand('copy')
      document.body.removeChild(ta)
    } catch {
      ok = false
    }
  }
  if (ok) {
    copied.value = true
    setTimeout(() => {
      copied.value = false
    }, 1500)
  }
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
        class="flex w-full items-center gap-3 rounded-lg px-3 py-2 text-sm text-sidebar-foreground/70 transition-colors hover:bg-sidebar-accent hover:text-sidebar-foreground"
        @click="toggleLocale"
      >
        <Languages class="h-4 w-4" />
        {{ getLocale() === 'en' ? '中文' : 'English' }}
      </button>
      <button
        class="flex w-full items-center gap-3 rounded-lg px-3 py-2 text-sm text-sidebar-foreground/70 transition-colors hover:bg-sidebar-accent hover:text-sidebar-foreground"
        @click="toggleTheme"
      >
        <Sun v-if="theme === 'dark'" class="h-4 w-4" />
        <Moon v-else class="h-4 w-4" />
        {{ theme === 'dark' ? $t('sidebar.lightMode') : $t('sidebar.darkMode') }}
      </button>

      <!-- Account. Sits at the bottom of the rail because that's where people
           go looking for it, and it's the only way out of a session in the
           web UI — the CLI has `tolato auth logout`. -->
      <DropdownMenu>
        <DropdownMenuTrigger as-child>
          <button
            type="button"
            class="flex w-full items-center gap-2.5 rounded-lg px-3 py-2 text-sm text-sidebar-foreground/70 transition-colors hover:bg-sidebar-accent hover:text-sidebar-foreground"
          >
            <span
              class="flex h-6 w-6 shrink-0 items-center justify-center rounded-full text-xs font-semibold"
              style="background-color: var(--primary); color: var(--primary-foreground)"
              aria-hidden="true"
            >
              {{ userInitial }}
            </span>
            <span class="min-w-0 flex-1 truncate text-left">
              {{ appStore.user?.username || $t('sidebar.account') }}
            </span>
            <ChevronsUpDown class="h-3.5 w-3.5 shrink-0 opacity-60" />
          </button>
        </DropdownMenuTrigger>

        <DropdownMenuContent align="start" side="top" :side-offset="8" class="w-[248px]">
          <div class="px-2 py-1.5">
            <p class="truncate text-sm font-medium">{{ appStore.user?.username }}</p>
            <p class="text-xs" style="color: var(--muted-foreground)">
              {{ appStore.isAdmin ? $t('users.roleAdmin') : $t('users.roleMember') }}
            </p>
          </div>
          <DropdownMenuSeparator />
          <DropdownMenuItem @select="navigate('/settings')">
            <Settings class="h-4 w-4" />
            {{ $t('sidebar.accountSettings') }}
          </DropdownMenuItem>
          <DropdownMenuItem variant="destructive" @select="appStore.logout()">
            <LogOut class="h-4 w-4" />
            {{ $t('sidebar.signOut') }}
          </DropdownMenuItem>
        </DropdownMenuContent>
      </DropdownMenu>

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
        <DropdownMenu>
          <DropdownMenuTrigger as-child>
            <button
              type="button"
              class="relative inline-flex items-center font-mono transition-opacity hover:opacity-100"
            >
              {{ appVersion }}
              <span
                v-if="hasUpdate"
                class="absolute -right-2 -top-1 h-1.5 w-1.5 rounded-full"
                style="background-color: var(--color-error)"
                aria-hidden="true"
              />
            </button>
          </DropdownMenuTrigger>

          <DropdownMenuContent align="end" side="top" :side-offset="8" class="w-80 p-3">
            <!-- Header: current → latest -->
            <div class="mb-2 flex items-center gap-1.5 text-sm font-medium">
              {{ hasUpdate ? $t('update.title') : $t('update.upToDate') }}
            </div>
            <div class="mb-3 flex flex-wrap items-center gap-1.5 text-xs" style="color: var(--muted-foreground)">
              <span>{{ $t('update.current') }}</span>
              <code class="font-mono">{{ versionInfo?.current || appVersion }}</code>
              <template v-if="hasUpdate">
                <span>→</span>
                <span>{{ $t('update.latest') }}</span>
                <code class="font-mono" style="color: var(--color-error)">{{ versionInfo?.latest }}</code>
              </template>
            </div>

            <!-- Upgrade prompt + copy (only when an update exists) -->
            <template v-if="hasUpdate">
              <p class="mb-1.5 text-xs" style="color: var(--muted-foreground)">
                {{ $t('update.promptHint') }}
              </p>
              <pre
                class="mb-2 max-h-32 overflow-auto whitespace-pre-wrap break-words rounded-md p-2 text-[11px] leading-relaxed"
                style="background: var(--secondary)"
              >{{ upgradePrompt }}</pre>
              <button
                type="button"
                class="mb-1 inline-flex w-full items-center justify-center gap-1.5 rounded-md px-2 py-1.5 text-xs font-medium transition-colors"
                style="background: var(--primary); color: var(--primary-foreground)"
                @click="copyPrompt"
              >
                <Check v-if="copied" class="h-3 w-3" />
                <Copy v-else class="h-3 w-3" />
                {{ copied ? $t('update.copied') : $t('update.copyPrompt') }}
              </button>
            </template>

            <!-- Release notes link -->
            <a
              :href="notesUrl"
              target="_blank"
              rel="noopener noreferrer"
              class="inline-flex items-center gap-1 text-xs transition-opacity hover:opacity-100"
              style="color: var(--muted-foreground)"
            >
              <ExternalLink class="h-3 w-3" />
              {{ $t('update.viewNotes') }}
            </a>
          </DropdownMenuContent>
        </DropdownMenu>
      </div>
    </div>
  </aside>
</template>
