<script setup lang="ts">
import { ref, nextTick, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import { Plus, Trash2, Pencil, Check } from 'lucide-vue-next'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { useChatStore } from '@/stores/chat'
import { updateConversation } from '@/services/api'

const chatStore = useChatStore()
const router = useRouter()

const editingId = ref<string | null>(null)
const editingTitle = ref('')
const editInputRef = ref<HTMLInputElement | null>(null)

// A conversation is "running" while its turn is in flight. Driven by the
// per-conversation state in the store, so a background conversation (not the
// active one) still shows its loading indicator as it streams.
function isRunning(id: string): boolean {
  const status = chatStore.conversationStates.get(id)?.status
  return status === 'streaming' || status === 'tool_exec' || status === 'confirming'
}

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
  if (editingId.value === id) return
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

async function startEdit(conv: { id: string; title?: string }, e: Event) {
  e.stopPropagation()
  editingId.value = conv.id
  editingTitle.value = conv.title || ''
  await nextTick()
  editInputRef.value?.focus()
  editInputRef.value?.select()
}

async function saveEdit() {
  const id = editingId.value
  if (!id) return
  const newTitle = editingTitle.value.trim()
  const conv = chatStore.conversations.find((c) => c.id === id)
  if (conv && newTitle && newTitle !== conv.title) {
    try {
      await updateConversation(id, { title: newTitle })
      conv.title = newTitle
    } catch {
      // TODO: show error toast
    }
  }
  editingId.value = null
}

function cancelEdit() {
  editingId.value = null
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
      <template v-if="editingId === conv.id">
        <Input
          ref="editInputRef"
          v-model="editingTitle"
          class="h-6 flex-1 px-1.5 text-sm"
          @click.stop
          @keyup.enter="saveEdit"
          @keyup.esc="cancelEdit"
          @blur="saveEdit"
        />
        <button
          class="flex h-5 w-5 items-center justify-center rounded"
          style="color: var(--muted-foreground)"
          @click.stop="saveEdit"
        >
          <Check class="h-3 w-3" />
        </button>
      </template>
      <template v-else>
        <span v-if="isRunning(conv.id)" class="loading-dots" aria-hidden="true">
          <span></span><span></span><span></span>
        </span>
        <span class="flex-1 truncate">{{ conv.title || $t('chat.newConversation') }}</span>
        <button
          class="row-action hidden h-5 w-5 items-center justify-center rounded group-hover:flex"
          @click="startEdit(conv, $event)"
        >
          <Pencil class="h-3 w-3" />
        </button>
        <button
          class="row-action row-action-danger hidden h-5 w-5 items-center justify-center rounded group-hover:flex"
          @click="handleDelete(conv.id, $event)"
        >
          <Trash2 class="h-3 w-3" />
        </button>
      </template>
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

<style scoped>
/* Three-dot typing/loading indicator shown while a conversation's turn runs. */
.loading-dots {
  display: inline-flex;
  flex-shrink: 0;
  align-items: center;
  gap: 2px;
}
.loading-dots > span {
  width: 4px;
  height: 4px;
  border-radius: 9999px;
  background-color: var(--primary);
  animation: loading-dots-bounce 1.2s infinite ease-in-out both;
}
.loading-dots > span:nth-child(1) {
  animation-delay: -0.32s;
}
.loading-dots > span:nth-child(2) {
  animation-delay: -0.16s;
}
@keyframes loading-dots-bounce {
  0%,
  80%,
  100% {
    transform: scale(0.6);
    opacity: 0.4;
  }
  40% {
    transform: scale(1);
    opacity: 1;
  }
}

/* Row hover actions (rename / delete). Inline styles previously set the color,
   which would have outranked a :hover class — so colors live here instead. */
.row-action {
  color: var(--muted-foreground);
  cursor: pointer;
  transition:
    background-color 0.15s ease,
    color 0.15s ease;
}
.row-action:hover {
  background-color: color-mix(in srgb, var(--sidebar-foreground) 12%, transparent);
  color: var(--sidebar-foreground);
}
.row-action-danger:hover {
  background-color: color-mix(in srgb, var(--destructive) 15%, transparent);
  color: var(--destructive);
}
</style>
