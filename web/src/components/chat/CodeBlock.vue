<script setup lang="ts">
import { ref, shallowRef, computed, watch, onBeforeUnmount } from 'vue'
import { Copy, Check } from 'lucide-vue-next'
import { useTheme } from '@/composables/useTheme'
import { highlightCode } from '@/lib/highlighter'

// markstream-vue passes the parsed node as `node`. For a fenced block that's
// `{ code, language, raw, ... }`.
const props = defineProps<{
  node?: { code?: string; language?: string; raw?: string }
}>()

const { theme } = useTheme()

const code = computed(() => props.node?.code ?? props.node?.raw ?? '')
const lang = computed(() => (props.node?.language ?? '').trim().toLowerCase())

// Highlighted markup. Until shiki resolves (and during streaming re-highlights)
// we keep the previous output so the block doesn't flash to plain text.
const html = shallowRef('')
let highlightToken = 0
watch(
  [code, lang, theme],
  () => {
    const token = ++highlightToken
    highlightCode(code.value, lang.value, theme.value === 'dark').then((out) => {
      if (token === highlightToken) html.value = out
    })
  },
  { immediate: true },
)

const copied = ref(false)
let copyTimer: ReturnType<typeof setTimeout> | undefined
async function copy() {
  if (!(await writeClipboard(code.value))) return
  copied.value = true
  clearTimeout(copyTimer)
  copyTimer = setTimeout(() => (copied.value = false), 1500)
}

// navigator.clipboard only exists in secure contexts (https / localhost). Over
// plain http on a LAN IP it's undefined, so fall back to the legacy execCommand
// path via a temporary textarea.
async function writeClipboard(text: string): Promise<boolean> {
  if (navigator.clipboard?.writeText) {
    try {
      await navigator.clipboard.writeText(text)
      return true
    } catch {
      // fall through to the legacy path
    }
  }
  try {
    const ta = document.createElement('textarea')
    ta.value = text
    ta.style.position = 'fixed'
    ta.style.opacity = '0'
    document.body.appendChild(ta)
    ta.focus()
    ta.select()
    const ok = document.execCommand('copy')
    document.body.removeChild(ta)
    return ok
  } catch {
    return false
  }
}
onBeforeUnmount(() => clearTimeout(copyTimer))
</script>

<template>
  <div
    class="code-block my-3 overflow-hidden rounded-lg border"
    style="border-color: var(--border); background-color: var(--secondary)"
  >
    <div
      class="flex items-center justify-between border-b px-3 py-1.5"
      style="border-color: var(--border)"
    >
      <span
        class="font-mono text-[11px] uppercase tracking-wide"
        style="color: var(--muted-foreground)"
      >
        {{ lang || 'text' }}
      </span>
      <button
        type="button"
        class="inline-flex items-center gap-1 rounded px-1.5 py-0.5 text-[11px] transition hover:bg-black/5 dark:hover:bg-white/10"
        style="color: var(--muted-foreground)"
        @click="copy"
      >
        <component :is="copied ? Check : Copy" class="h-3 w-3" />
        {{ copied ? $t('chat.copied') : $t('chat.copy') }}
      </button>
    </div>
    <!-- shiki output: code is HTML-escaped by the highlighter, safe to inject. -->
    <div class="code-block-body overflow-x-auto text-[13px] leading-relaxed" v-html="html" />
  </div>
</template>

<style scoped>
/* Let shiki own the syntax colors but drop its background so the card's
   --secondary surface shows through; we control padding ourselves. */
.code-block-body :deep(pre.shiki) {
  margin: 0;
  padding: 12px 14px;
  background-color: transparent !important;
  font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, 'Liberation Mono', monospace;
}
.code-block-body :deep(code) {
  font-family: inherit;
}
</style>
