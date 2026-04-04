<script setup lang="ts">
import AppSidebar from './AppSidebar.vue'
</script>

<template>
  <div class="flex h-screen w-full overflow-hidden">
    <AppSidebar />
    <main class="flex-1 overflow-hidden">
      <RouterView />
    </main>
  </div>
</template>
