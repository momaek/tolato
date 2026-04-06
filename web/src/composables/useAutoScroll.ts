import { ref, onMounted, onUnmounted, type Ref } from 'vue'

export function useAutoScroll(containerRef: Ref<HTMLElement | null>) {
  const isAutoScroll = ref(true)
  const showScrollButton = ref(false)

  let observer: MutationObserver | null = null

  function scrollToBottom() {
    const el = containerRef.value
    if (el) {
      el.scrollTop = el.scrollHeight
    }
  }

  function onScroll() {
    const el = containerRef.value
    if (!el) return

    const threshold = 100
    const isAtBottom = el.scrollHeight - el.scrollTop - el.clientHeight < threshold
    isAutoScroll.value = isAtBottom
    showScrollButton.value = !isAtBottom
  }

  function handleAutoScroll() {
    if (isAutoScroll.value) {
      scrollToBottom()
    }
  }

  onMounted(() => {
    const el = containerRef.value
    if (!el) return

    el.addEventListener('scroll', onScroll)

    // Watch for DOM changes (new messages, streaming content)
    observer = new MutationObserver(() => {
      handleAutoScroll()
    })
    observer.observe(el, { childList: true, subtree: true, characterData: true })
  })

  onUnmounted(() => {
    const el = containerRef.value
    if (el) {
      el.removeEventListener('scroll', onScroll)
    }
    observer?.disconnect()
  })

  return {
    isAutoScroll,
    showScrollButton,
    scrollToBottom,
  }
}
