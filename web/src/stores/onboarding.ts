import { defineStore } from 'pinia'
import { ref, computed } from 'vue'
import { getOnboarding } from '@/services/api'
import { useNodesStore } from '@/stores/nodes'

const DISMISSED_KEY = 'tolato.onboarding.dismissed'

function readDismissed(): boolean {
  try {
    return localStorage.getItem(DISMISSED_KEY) === '1'
  } catch {
    return false
  }
}

// The two-step "get started" checklist on the home page: a node online, then
// Tolato installed in the user's own coding agent. Web chat is no longer a
// step — it needs an LLM key the other path doesn't — but its readiness lives
// here too so the sidebar and chat view can say why it won't answer.
export const useOnboardingStore = defineStore('onboarding', () => {
  const nodesStore = useNodesStore()

  const loaded = ref(false)
  const cliConnected = ref(false)
  // Optimistic until the server says otherwise, so a configured instance never
  // flashes a "not configured" warning on load.
  const llmConfigured = ref(true)
  const dismissed = ref(readDismissed())

  // A node that has heartbeated at least once is enrolled; one that was only
  // created still has its install command pending. Going offline later doesn't
  // undo the step.
  const hasNode = computed(() =>
    nodesStore.nodes.some((n) => n.status === 'online' || !!n.last_heartbeat),
  )
  const doneCount = computed(() => Number(hasNode.value) + Number(cliConnected.value))
  const complete = computed(() => hasNode.value && cliConnected.value)
  // Whether the checklist still deserves a place in the navigation.
  const pending = computed(() => loaded.value && !complete.value && !dismissed.value)

  async function refresh() {
    try {
      const [status] = await Promise.all([getOnboarding(), nodesStore.fetchNodes()])
      cliConnected.value = status.cli_connected
      llmConfigured.value = status.llm_configured
      loaded.value = true
    } catch {
      /* best-effort: the checklist just stays where it was */
    }
  }

  function dismiss() {
    dismissed.value = true
    try {
      localStorage.setItem(DISMISSED_KEY, '1')
    } catch {
      /* private mode — it'll come back next visit, which is harmless */
    }
  }

  return { loaded, cliConnected, llmConfigured, dismissed, hasNode, doneCount, complete, pending, refresh, dismiss }
})
