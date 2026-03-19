import { defineStore } from "pinia"
import type { ConsoleMode } from "@/shared/api/contracts"

export const useConsoleStore = defineStore("console", {
  state: () => ({
    mode: "ai_agent" as ConsoleMode,
    targetNodeId: "all",
    composerText: "检查所有在线节点的系统负载和磁盘占用",
  }),
  actions: {
    setMode(mode: ConsoleMode) {
      this.mode = mode
    },
    setTargetNodeId(nodeId: string) {
      this.targetNodeId = nodeId
    },
    setComposerText(value: string) {
      this.composerText = value
    },
  },
})
