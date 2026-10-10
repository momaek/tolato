<script setup lang="ts">
import { ref, computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { toast } from 'vue-sonner'
import { Copy, Check } from 'lucide-vue-next'
import { Button } from '@/components/ui/button'
import { copyText } from '@/lib/clipboard'

// The prompt a user pastes into their coding agent to install the skill and
// CLI. Built against the origin the user is actually browsing, not
// server.public_address: whatever host reached this page is a host their
// machine can reach, which is the whole point of handing them a URL to paste.

const { t } = useI18n()

const prompt = computed(() => t('skill.prompt', { url: window.location.origin }))
const copied = ref(false)

async function copy() {
  if (!(await copyText(prompt.value))) {
    toast.error(t('common.copyFailed'))
    return
  }
  copied.value = true
  setTimeout(() => (copied.value = false), 1500)
}
</script>

<template>
  <div class="relative">
    <pre
      class="max-h-72 overflow-auto whitespace-pre-wrap rounded-lg border p-4 pr-28 text-xs leading-relaxed"
      style="background: var(--muted); border-color: var(--border)"
    >{{ prompt }}</pre>
    <Button size="sm" class="absolute right-3 top-3" @click="copy">
      <Check v-if="copied" class="mr-1.5 h-3.5 w-3.5" />
      <Copy v-else class="mr-1.5 h-3.5 w-3.5" />
      {{ copied ? $t('skill.copied') : $t('skill.copy') }}
    </Button>
  </div>
</template>
