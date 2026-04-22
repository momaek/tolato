<script setup lang="ts">
import { ref, watch, onMounted } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import ChatTopBar from '@/components/chat/ChatTopBar.vue'
import ChatMessages from '@/components/chat/ChatMessages.vue'
import ChatInput from '@/components/chat/ChatInput.vue'
import { useChatStore } from '@/stores/chat'

const route = useRoute()
const router = useRouter()
const chatStore = useChatStore()
const chatInputRef = ref<InstanceType<typeof ChatInput> | null>(null)
// Holds the topbar's model selection before a conversation exists.
const pendingModel = ref('')
const pendingNodeId = ref<string | undefined>(undefined)

onMounted(() => {
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
    // Small delay to let state sync
    await new Promise((r) => setTimeout(r, 50))
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
</script>

<template>
  <div class="flex h-full flex-col" style="background-color: var(--background)">
    <ChatTopBar
      :conversation-id="chatStore.activeConversationId || undefined"
      :title="chatStore.activeState?.title || ''"
      :model="chatStore.activeState?.model || pendingModel"
      :default-node-id="chatStore.activeState?.defaultNodeId ?? pendingNodeId"
      @update:model="(v) => {
        if (chatStore.activeState) chatStore.activeState.model = v
        else pendingModel = v
      }"
      @update:default-node-id="(v) => {
        if (chatStore.activeState) chatStore.activeState.defaultNodeId = v
        else pendingNodeId = v
      }"
    />

    <ChatMessages
      :messages="chatStore.activeState?.messages || []"
      :streaming="chatStore.activeState?.streaming || null"
      :status="chatStore.activeState?.status || 'idle'"
      :confirm-request="chatStore.activeState?.confirmRequest || null"
      @quick-action="handleQuickAction"
      @confirm="handleConfirm"
    />

    <ChatInput
      ref="chatInputRef"
      :status="chatStore.activeState?.status || 'idle'"
      @send="handleSend"
    />
  </div>
</template>
