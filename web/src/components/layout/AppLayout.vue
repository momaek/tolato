<script setup lang="ts">
import { onBeforeUnmount } from 'vue'
import AppSidebar from './AppSidebar.vue'
import { useChatStore } from '@/stores/chat'
import { wsService } from '@/services/ws'

// Instantiate the chat store once at the shell so its WS handlers register
// before any event can arrive. Views that need data just `useChatStore()`
// later. The store registering handlers does NOT itself open a connection —
// see ChatView for the lazy connect.
useChatStore()

// chat WS is opened lazily by ChatView (only when the user actually visits
// /chat). New tabs that go straight to /nodes or /nodes/:id/terminal don't
// pay the cost of an idle chat connection. Once opened it stays alive across
// route changes so a long-running LLM stream isn't dropped when the user
// navigates away from /chat and back.

// Cleanly close the WS on page refresh / tab close so the server reaps the
// session immediately instead of waiting on the ping timeout. This is a
// no-op if no connection was opened in this tab.
function handleBeforeUnload() {
  wsService.disconnect()
}
window.addEventListener('beforeunload', handleBeforeUnload)

onBeforeUnmount(() => {
  window.removeEventListener('beforeunload', handleBeforeUnload)
  // Logout (or any path that unmounts the shell) tears the chat WS down.
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
