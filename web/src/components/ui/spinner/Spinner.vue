<script setup lang="ts">
import type { HTMLAttributes } from "vue"
import { Loader2Icon } from "lucide-vue-next"
import { useI18n } from "vue-i18n"
import { cn } from "@/lib/utils"

const props = defineProps<{
  class?: HTMLAttributes["class"]
}>()

const { t } = useI18n()
</script>

<template>
  <Loader2Icon
    role="status"
    :aria-label="t('common.states.thinking')"
    :class="cn('size-4 animate-spin', props.class)"
  />
</template>
