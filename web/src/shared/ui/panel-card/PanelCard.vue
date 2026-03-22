<script setup lang="ts">
import type { HTMLAttributes } from 'vue'
import { computed } from 'vue'

import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'

const props = withDefaults(
  defineProps<{
    title?: string
    description?: string
    compact?: boolean
    dense?: boolean
    bodyClass?: HTMLAttributes['class']
  }>(),
  {
    title: '',
    description: '',
    compact: false,
    dense: false,
    bodyClass: undefined,
  },
)

const contentClass = computed(() => {
  if (props.dense) {
    return props.compact ? 'px-3.5 pb-3.5 pt-0' : 'px-3.5 pb-3.5 pt-0'
  }

  return props.compact ? 'px-4 pb-4 pt-0' : 'px-5 pb-5 pt-0'
})

const headerClass = computed(() => {
  if (props.dense) {
    return props.compact ? 'px-3.5 pb-2 pt-3.5' : 'px-3.5 pb-2.5 pt-3.5'
  }

  return props.compact ? 'px-4 pb-3 pt-4' : 'px-5 pb-4 pt-5'
})
</script>

<template>
  <Card class="glass-panel gap-0 rounded-[1rem] border-white/60 bg-brand-panel/90 py-0">
    <CardHeader v-if="title || description" :class="headerClass">
      <CardTitle class="text-sm font-semibold leading-5 text-foreground">
        {{ title }}
      </CardTitle>
      <CardDescription v-if="description" class="text-sm leading-5 text-muted-foreground">
        {{ description }}
      </CardDescription>
      <slot name="header" />
    </CardHeader>
    <CardContent :class="[contentClass, props.bodyClass]">
      <slot />
    </CardContent>
  </Card>
</template>
