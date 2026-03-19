import { defineStore } from "pinia"
import type { AuditEvent } from "@/shared/api/contracts"

export const useAuditsStore = defineStore("audits", {
  state: () => ({
    items: [] as AuditEvent[],
  }),
  actions: {
    setAudits(audits: AuditEvent[]) {
      this.items = audits
    },
  },
})
