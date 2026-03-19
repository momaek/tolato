<script setup lang="ts">
import { storeToRefs } from "pinia"
import { useAuditsStore } from "@/entities/audits/store"
import { Card, CardContent, CardHeader, CardTitle, Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/shared/ui"
import { formatDateTime } from "@/shared/lib/format"

const auditsStore = useAuditsStore()
const { items } = storeToRefs(auditsStore)
</script>

<template>
  <div class="space-y-4">
    <div>
      <p class="text-sm font-semibold uppercase tracking-[0.18em] text-muted-foreground">
        Compliance
      </p>
      <h1 class="mt-2 text-3xl font-semibold tracking-tight">Audits</h1>
    </div>

    <Card class="border-none shadow-sm">
      <CardHeader>
        <CardTitle>Audit trail</CardTitle>
      </CardHeader>
      <CardContent>
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>Task</TableHead>
              <TableHead>Action</TableHead>
              <TableHead>Actor</TableHead>
              <TableHead>Description</TableHead>
              <TableHead>Time</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            <TableRow v-for="audit in items" :key="audit.id">
              <TableCell>{{ audit.taskId }}</TableCell>
              <TableCell class="font-medium">{{ audit.action }}</TableCell>
              <TableCell>{{ audit.actorId }}</TableCell>
              <TableCell>{{ audit.description }}</TableCell>
              <TableCell>{{ formatDateTime(audit.createdAt) }}</TableCell>
            </TableRow>
          </TableBody>
        </Table>
      </CardContent>
    </Card>
  </div>
</template>
