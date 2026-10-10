<script setup lang="ts">
import { ref, watch, onMounted } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import ChatMessages from '@/components/chat/ChatMessages.vue'
import ChatInput from '@/components/chat/ChatInput.vue'
import { useChatStore } from '@/stores/chat'
import { useAppStore } from '@/stores/app'
import { useOnboardingStore } from '@/stores/onboarding'
import { AlertCircle } from 'lucide-vue-next'
import { Button } from '@/components/ui/button'
import { wsService } from '@/services/ws'

const { t } = useI18n()

const route = useRoute()
const router = useRouter()
const chatStore = useChatStore()
const appStore = useAppStore()
const onboarding = useOnboardingStore()
const chatInputRef = ref<InstanceType<typeof ChatInput> | null>(null)
// Holds the topbar's model selection before a conversation exists.
const pendingModel = ref('')
const pendingNodeId = ref<string | undefined>(undefined)

onMounted(() => {
  // Lazy chat WS: only opened when the user actually enters /chat. Tabs that
  // stay on /nodes or /nodes/:id/terminal never pay for an idle chat
  // connection. wsService.connect is idempotent — re-mounting ChatView (e.g.
  // navigating between /chat and /chat/:id) is a no-op against an existing
  // socket with the same token.
  if (appStore.token) {
    wsService.connect(appStore.token)
  }

  const convId = route.params.conversationId as string
  if (convId) {
    chatStore.setActive(convId)
    chatStore.loadConversation(convId)
  }
})

// Watch route changes
watch(
  () => route.params.conversationId,
  (newId) => {
    const id = newId as string
    if (id) {
      chatStore.setActive(id)
      chatStore.loadConversation(id)
    } else {
      chatStore.setActive(null)
    }
  }
)

async function handleSend(content: string) {
  // If no active conversation, create one first
  if (!chatStore.activeConversationId) {
    const conv = await chatStore.addConversation({
      title: content.slice(0, 30),
      model: pendingModel.value,
      default_node_id: pendingNodeId.value,
    })
    chatStore.setActive(conv.id)
    router.push(`/chat/${conv.id}`)
  }
  chatStore.sendMessage(content)
}

function handleQuickAction(text: string) {
  if (chatInputRef.value) {
    chatInputRef.value.fillInput(text)
  }
}

function handleConfirm(id: string, approved: boolean) {
  chatStore.confirmAction(id, approved)
}

async function handleDeleteMessage(id: string) {
  if (!window.confirm(t('chat.confirmDeleteMessage'))) return
  try {
    await chatStore.removeMessage(id)
  } catch {
    // TODO: surface a toast — silent failure mirrors removeConversation today.
  }
}
</script>

<template>
  <div class="flex h-full flex-col" style="background-color: var(--background)">
    <!-- Chat is the one feature that needs an LLM key. Say so up front rather
         than letting the first message fail, and point at the path that
         doesn't need one. -->
    <div
      v-if="onboarding.loaded && !onboarding.llmConfigured"
      class="mx-6 mt-4 flex flex-wrap items-start gap-3 rounded-lg border px-4 py-3"
      style="border-color: var(--border); background: var(--secondary)"
    >
      <AlertCircle class="mt-0.5 h-4 w-4 shrink-0" style="color: var(--muted-foreground)" />
      <div class="min-w-0 flex-1 space-y-0.5 text-sm">
        <p class="font-medium">{{ $t('chat.llmMissing.title') }}</p>
        <p style="color: var(--muted-foreground)">
          {{ appStore.isAdmin ? $t('chat.llmMissing.admin') : $t('chat.llmMissing.member') }}
          {{ $t('chat.llmMissing.skillHint') }}
        </p>
      </div>
      <div class="flex shrink-0 gap-2">
        <Button v-if="appStore.isAdmin" size="sm" variant="outline" @click="router.push('/settings')">
          {{ $t('chat.llmMissing.configure') }}
        </Button>
        <Button size="sm" @click="router.push('/skill')">
          {{ $t('chat.llmMissing.useSkill') }}
        </Button>
      </div>
    </div>

    <ChatMessages
      :messages="chatStore.activeState?.messages || []"
      :streaming="chatStore.activeState?.streaming || null"
      :status="chatStore.activeState?.status || 'idle'"
      :confirm-request="chatStore.activeState?.confirmRequest || null"
      :error="chatStore.activeState?.error || null"
      @quick-action="handleQuickAction"
      @confirm="handleConfirm"
      @delete-message="handleDeleteMessage"
    />

    <ChatInput
      ref="chatInputRef"
      :status="chatStore.activeState?.status || 'idle'"
      :conversation-id="chatStore.activeConversationId || undefined"
      :model="chatStore.activeState?.model || pendingModel"
      :default-node-id="chatStore.activeState?.defaultNodeId ?? pendingNodeId"
      @send="handleSend"
      @stop="chatStore.stopGeneration"
      @update:model="(v) => {
        if (chatStore.activeState) chatStore.activeState.model = v
        else pendingModel = v
      }"
      @update:default-node-id="(v) => {
        if (chatStore.activeState) chatStore.activeState.defaultNodeId = v
        else pendingNodeId = v
      }"
    />
  </div>
</template>
