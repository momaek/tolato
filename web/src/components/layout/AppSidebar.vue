<script setup lang="ts">
import { computed } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { MessageSquare, Server, FileText, Settings, Zap } from 'lucide-vue-next'
import { Separator } from '@/components/ui/separator'
import { ScrollArea } from '@/components/ui/scroll-area'
import ConversationList from './ConversationList.vue'

const route = useRoute()
const router = useRouter()

const navItems = [
  { label: 'Chat', icon: MessageSquare, path: '/chat' },
  { label: 'Nodes', icon: Server, path: '/nodes' },
  { label: 'Audit Log', icon: FileText, path: '/audit' },
  { label: 'Settings', icon: Settings, path: '/settings' },
]

const isActive = (path: string) => {
  if (path === '/chat') {
    return route.path === '/chat' || route.path.startsWith('/chat/')
  }
  return route.path === path
}

const isChatRoute = computed(() => route.path === '/chat' || route.path.startsWith('/chat/'))

function navigate(path: string) {
  router.push(path)
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
    <ScrollArea v-if="isChatRoute" class="flex-1 px-3">
      <ConversationList />
    </ScrollArea>
  </aside>
</template>
