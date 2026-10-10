<script setup lang="ts">
import { onMounted } from 'vue'
import { FileText, CheckCircle2 } from 'lucide-vue-next'
import { Separator } from '@/components/ui/separator'
import InstallPrompt from '@/components/skill/InstallPrompt.vue'
import { useOnboardingStore } from '@/stores/onboarding'

const onboarding = useOnboardingStore()
const skillUrl = `${window.location.origin}/skill.md`

onMounted(() => onboarding.refresh())
</script>

<template>
  <div class="h-full overflow-auto" style="background-color: var(--background)">
    <div class="mx-auto max-w-3xl space-y-6 px-6 py-8">
      <div>
        <h1 class="text-xl font-semibold">{{ $t('skill.title') }}</h1>
        <p class="mt-1.5 text-sm" style="color: var(--muted-foreground)">
          {{ $t('skill.description') }}
        </p>
      </div>

      <div
        v-if="onboarding.cliConnected"
        class="flex items-start gap-3 rounded-lg border px-4 py-3"
        style="border-color: var(--border)"
      >
        <CheckCircle2 class="mt-0.5 h-4 w-4 shrink-0" style="color: var(--color-success-foreground)" />
        <div>
          <p class="text-sm font-medium">{{ $t('skill.connected') }}</p>
          <p class="mt-0.5 text-sm" style="color: var(--muted-foreground)">
            {{ $t('skill.connectedHelp') }}
          </p>
        </div>
      </div>

      <div class="space-y-3">
        <div>
          <h2 class="text-sm font-medium">{{ $t('skill.promptLabel') }}</h2>
          <p class="mt-1 text-sm" style="color: var(--muted-foreground)">
            {{ $t('skill.promptHelp') }}
          </p>
        </div>
        <InstallPrompt />
      </div>

      <Separator />

      <div class="space-y-2">
        <h2 class="text-sm font-medium">{{ $t('skill.manualTitle') }}</h2>
        <p class="text-sm" style="color: var(--muted-foreground)">
          {{ $t('skill.manualHelp') }}
        </p>
        <a
          :href="skillUrl"
          target="_blank"
          rel="noopener noreferrer"
          class="inline-flex items-center gap-1.5 text-sm underline underline-offset-4"
          style="color: var(--primary)"
        >
          <FileText class="h-4 w-4" />
          {{ $t('skill.viewSkill') }}
        </a>
      </div>
    </div>
  </div>
</template>
