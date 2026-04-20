<script setup lang="ts">
import { onMounted, onBeforeUnmount, watch } from 'vue'
import { toast } from 'vue-sonner'
import AppSidebar from './AppSidebar.vue'
import { useAppStore } from '@/stores/app'
import { useChatStore } from '@/stores/chat'
import { wsService } from '@/services/ws'

const appStore = useAppStore()
// Instantiate the chat store once here so its WS handlers register before
// any event can arrive. Views that need data just `useChatStore()` later.
useChatStore()

function connect() {
  if (appStore.token) {
    wsService.connect(appStore.token)
  }
}

// Connect once on shell mount, reconnect if the token changes (re-login).
onMounted(connect)
watch(() => appStore.token, (tok) => {
  if (tok) connect()
  else wsService.disconnect()
})

// Session-replaced: another browser/tab logged in with the same account and the
// server kicked us. Drop local auth and route back to /login.
const offState = wsService.onStateChange((s) => {
  if (s === 'replaced') {
    toast.error('Session replaced by another login')
    appStore.logout()
  }
})

onBeforeUnmount(() => {
  offState()
  wsService.disconnect()
})
</script>

<template>
  <div class="flex h-screen w-full overflow-hidden">
    <AppSidebar />
    <main class="flex-1 overflow-hidden">
      <RouterView />
    </main>
  </div>
</template>
