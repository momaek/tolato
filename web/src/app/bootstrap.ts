import { hasAuthToken } from "@/shared/api/auth-token"
import { controlApi } from "@/shared/api/control-api"
import { controlWs } from "@/shared/api/control-ws"
import { useAuditsStore } from "@/entities/audits/store"
import { useConnectionStore } from "@/entities/connection/store"
import { useNodesStore } from "@/entities/nodes/store"
import { useSessionStore } from "@/entities/session/store"
import { useTasksStore } from "@/entities/tasks/store"

let bootstrapPromise: Promise<void> | null = null
let stopWs: (() => void) | null = null

export function bootstrapApp() {
  if (bootstrapPromise) {
    return bootstrapPromise
  }

  const sessionStore = useSessionStore()
  const connectionStore = useConnectionStore()
  const nodesStore = useNodesStore()
  const tasksStore = useTasksStore()
  const auditsStore = useAuditsStore()

  bootstrapPromise = (async () => {
    if (import.meta.env.VITE_USE_MOCK !== "true" && !hasAuthToken()) {
      connectionStore.markConnecting()
      return
    }

    connectionStore.markConnecting()

    const [session, nodes, tasks, audits] = await Promise.all([
      controlApi.getSession(),
      controlApi.getNodes(),
      controlApi.getTasks(),
      controlApi.getAudits(),
    ])

    sessionStore.setSession(session)
    nodesStore.setNodes(nodes)
    tasksStore.setTasks(tasks)
    auditsStore.setAudits(audits)

    stopWs?.()
    stopWs = controlWs.connect((event) => {
      connectionStore.consumeWsEvent(event)
      nodesStore.consumeWsEvent(event)
      tasksStore.consumeWsEvent(event)
    })
  })()

  return bootstrapPromise.catch((error) => {
    bootstrapPromise = null
    throw error
  })
}

export function resetBootstrapApp() {
  stopWs?.()
  stopWs = null
  bootstrapPromise = null
}
