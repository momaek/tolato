<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { cn } from '@/lib/utils'

const props = defineProps<{
  search: string
  status: 'all' | 'online' | 'busy' | 'stale' | 'offline'
  region: string
  tag: string
  busyOnly: boolean
  regions: string[]
  tags: string[]
  total: number
  visible: number
}>()

const emit = defineEmits<{
  (e: 'update:search', value: string): void
  (e: 'update:status', value: 'all' | 'online' | 'busy' | 'stale' | 'offline'): void
  (e: 'update:region', value: string): void
  (e: 'update:tag', value: string): void
  (e: 'update:busyOnly', value: boolean): void
  (e: 'reset'): void
}>()

const { t } = useI18n()

const statusOptions = computed<Array<{ value: 'all' | 'online' | 'busy' | 'stale' | 'offline'; label: string }>>(() => [
  { value: 'all', label: t('pages.nodes.filterAll') },
  { value: 'online', label: t('pages.nodes.filterOnline') },
  { value: 'busy', label: t('pages.nodes.filterBusy') },
  { value: 'stale', label: t('pages.nodes.filterStale') },
  { value: 'offline', label: t('pages.nodes.filterOffline') },
])
</script>

<template>
  <div class="glass-panel border-border/70 rounded-2xl border p-4">
    <div class="flex flex-col gap-4 lg:flex-row lg:items-center lg:justify-between">
      <div>
        <p class="text-sm font-medium text-foreground">{{ t('pages.nodes.nodeInventory') }}</p>
        <p class="text-sm text-muted-foreground">{{ t('pages.nodes.nodeInventoryDescription', { visible, total }) }}</p>
      </div>

      <div class="flex flex-wrap items-center gap-2">
        <Badge variant="secondary">{{ t('pages.nodes.mockDataReady') }}</Badge>
        <Button variant="ghost" size="sm" @click="emit('reset')">{{ t('common.buttons.reset') }}</Button>
      </div>
    </div>

    <div class="mt-4 grid gap-3 lg:grid-cols-[minmax(0,1fr)_auto]">
      <Input
        :model-value="search"
        :placeholder="t('pages.nodes.searchPlaceholder')"
        class="h-11 bg-background/90"
        @update:model-value="value => emit('update:search', String(value))"
      />

      <div class="flex flex-wrap gap-2">
        <Button
          v-for="option in statusOptions"
          :key="option.value"
          :variant="props.status === option.value ? 'default' : 'outline'"
          :class="cn('h-11 rounded-full px-4', props.status === option.value && 'shadow-sm')"
          @click="emit('update:status', option.value)"
        >
          {{ option.label }}
        </Button>
      </div>
    </div>

    <div class="mt-3 grid gap-3 lg:grid-cols-[repeat(3,minmax(0,1fr))_auto]">
      <select
        :value="region"
        class="h-11 rounded-xl border border-border bg-background px-3 text-sm"
        @change="emit('update:region', ($event.target as HTMLSelectElement).value)"
      >
        <option value="all">{{ t('pages.nodes.allRegions') }}</option>
        <option v-for="option in regions" :key="option" :value="option">{{ option }}</option>
      </select>
      <select
        :value="tag"
        class="h-11 rounded-xl border border-border bg-background px-3 text-sm"
        @change="emit('update:tag', ($event.target as HTMLSelectElement).value)"
      >
        <option value="all">{{ t('pages.nodes.allTags') }}</option>
        <option v-for="option in tags" :key="option" :value="option">{{ option }}</option>
      </select>
      <Button
        :variant="busyOnly ? 'default' : 'outline'"
        class="h-11 rounded-xl"
        @click="emit('update:busyOnly', !busyOnly)"
      >
        {{ busyOnly ? t('pages.nodes.busyOnly') : t('pages.nodes.showAllBusyStates') }}
      </Button>
    </div>
  </div>
</template>
