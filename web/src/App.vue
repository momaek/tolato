<script setup lang="ts">
import { Toaster } from 'vue-sonner'
import { useTheme } from '@/composables/useTheme'

// Initialize theme on app load
const { theme } = useTheme()
</script>

<template>
  <RouterView />
  <Toaster position="top-right" :theme="theme" rich-colors />
</template>
