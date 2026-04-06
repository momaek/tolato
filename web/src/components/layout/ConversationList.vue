<script setup lang="ts">
import { onMounted } from 'vue'
import { useRouter } from 'vue-router'
import { Plus, Trash2 } from 'lucide-vue-next'
import { Button } from '@/components/ui/button'
import { useChatStore } from '@/stores/chat'

const chatStore = useChatStore()
const router = useRouter()

onMounted(() => {
  chatStore.fetchConversations()
})

async function handleNew() {
  try {
    const conv = await chatStore.addConversation({})
    router.push(`/chat/${conv.id}`)
  } catch {
    // TODO: show error toast
  }
}

function selectConversation(id: string) {
  chatStore.setActive(id)
  router.push(`/chat/${id}`)
}

async function handleDelete(id: string, e: Event) {
  e.stopPropagation()
  try {
    await chatStore.removeConversation(id)
    if (chatStore.activeConversationId === null) {
      router.push('/chat')
    }
  } catch {
    // TODO: show error toast
  }
}
</script>

<template>
  <div class="flex flex-col gap-1">
    <div class="flex items-center justify-between px-2 py-1">
      <span class="text-xs font-medium" style="color: var(--muted-foreground)">
        {{ $t('chat.conversations') }}
      </span>
      <Button variant="ghost" size="icon" class="h-6 w-6" @click="handleNew">
        <Plus class="h-3.5 w-3.5" />
      </Button>
    </div>

    <div v-if="chatStore.loading" class="px-2 py-4 text-center text-xs" style="color: var(--muted-foreground)">
      {{ $t('common.loading') }}
    </div>

    <div
      v-for="conv in chatStore.conversations"
      :key="conv.id"
      class="group flex cursor-pointer items-center gap-2 rounded-lg px-3 py-2 text-sm transition-colors"
      :style="{
        backgroundColor: chatStore.activeConversationId === conv.id ? 'var(--sidebar-accent)' : undefined,
        color: 'var(--sidebar-foreground)',
      }"
      @click="selectConversation(conv.id)"
    >
      <span class="flex-1 truncate">{{ conv.title || $t('chat.newConversation') }}</span>
      <button
        class="hidden h-5 w-5 items-center justify-center rounded group-hover:flex"
        style="color: var(--muted-foreground)"
        @click="handleDelete(conv.id, $event)"
      >
        <Trash2 class="h-3 w-3" />
      </button>
    </div>

    <div
      v-if="!chatStore.loading && chatStore.conversations.length === 0"
      class="px-2 py-4 text-center text-xs"
      style="color: var(--muted-foreground)"
    >
      {{ $t('chat.noConversations') }}
    </div>
  </div>
</template>
