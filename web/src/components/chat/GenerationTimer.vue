<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted } from 'vue'
import { Loader2 } from 'lucide-vue-next'

const props = defineProps<{
  startedAt: number
}>()

const now = ref(Date.now())
let timer: ReturnType<typeof setInterval> | undefined

onMounted(() => {
  timer = setInterval(() => (now.value = Date.now()), 500)
})
onUnmounted(() => clearInterval(timer))

const elapsed = computed(() => {
  const total = Math.max(0, Math.floor((now.value - props.startedAt) / 1000))
  if (total < 60) return `${total}s`
  const m = Math.floor(total / 60)
  const s = total % 60
  return `${m}m ${s}s`
})
</script>

<template>
  <div class="flex items-center gap-2 px-5 py-2">
    <Loader2 class="h-3.5 w-3.5 animate-spin" style="color: var(--primary)" />
    <span class="text-xs" style="color: var(--muted-foreground)">{{ $t('chat.aiThinking') }}</span>
    <span class="text-xs tabular-nums" style="color: var(--muted-foreground)">{{ elapsed }}</span>
  </div>
</template>
