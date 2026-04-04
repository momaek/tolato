<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { useRoute } from 'vue-router'
import { Bot, ChevronDown } from 'lucide-vue-next'
import { Button } from '@/components/ui/button'
import { Textarea } from '@/components/ui/textarea'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Separator } from '@/components/ui/separator'
import EmptyState from '@/components/chat/EmptyState.vue'
import { useChatStore } from '@/stores/chat'

const route = useRoute()
const chatStore = useChatStore()

const messageInput = ref('')
const selectedModel = ref('gpt-4o')
const selectedNode = ref('')

onMounted(() => {
  const convId = route.params.conversationId as string
  if (convId) {
    chatStore.setActive(convId)
  }
})

function handleQuickAction(text: string) {
  messageInput.value = text
}
</script>

<template>
  <div class="flex h-full flex-col" style="background-color: var(--background)">
    <!-- Top bar -->
    <div class="flex items-center gap-3 border-b px-5 py-3">
      <div class="flex items-center gap-2">
        <Bot class="h-4 w-4" style="color: var(--muted-foreground)" />
        <span class="text-sm font-medium">New Conversation</span>
      </div>
      <div class="flex-1" />
      <Select v-model="selectedModel">
        <SelectTrigger class="w-[160px]">
          <SelectValue placeholder="Select model" />
        </SelectTrigger>
        <SelectContent>
          <SelectItem value="gpt-4o">GPT-4o</SelectItem>
          <SelectItem value="gpt-4o-mini">GPT-4o Mini</SelectItem>
          <SelectItem value="claude-3.5-sonnet">Claude 3.5 Sonnet</SelectItem>
        </SelectContent>
      </Select>
      <Select v-model="selectedNode">
        <SelectTrigger class="w-[160px]">
          <SelectValue placeholder="Select node" />
        </SelectTrigger>
        <SelectContent>
          <SelectItem value="all">All Nodes</SelectItem>
        </SelectContent>
      </Select>
    </div>

    <!-- Messages area -->
    <div class="flex flex-1 flex-col overflow-y-auto">
      <EmptyState @quick-action="handleQuickAction" />
    </div>

    <Separator />

    <!-- Input area -->
    <div class="px-5 py-4">
      <div class="relative">
        <Textarea
          v-model="messageInput"
          placeholder="Send a message..."
          disabled
          class="min-h-[52px] resize-none pr-14"
          :rows="1"
        />
        <Button
          size="icon"
          class="absolute bottom-2 right-2 h-8 w-8 rounded-full"
          disabled
        >
          <ChevronDown class="h-4 w-4 rotate-[-90deg]" />
        </Button>
      </div>
    </div>
  </div>
</template>
