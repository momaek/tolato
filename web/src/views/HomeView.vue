<script setup lang="ts">
import { ref, computed, onMounted, onBeforeUnmount, watch } from 'vue'
import { useRouter } from 'vue-router'
import { Check, Plus, ArrowRight, Loader2, PartyPopper } from 'lucide-vue-next'
import { Button } from '@/components/ui/button'
import InstallPrompt from '@/components/skill/InstallPrompt.vue'
import { useOnboardingStore } from '@/stores/onboarding'
import { useNodesStore } from '@/stores/nodes'
import { useAppStore } from '@/stores/app'

// The landing page. A new user gets two steps — a node online, then Tolato in
// their own coding agent — and anyone who has done both (or skipped) is sent
// straight on to the node list, which is what the web UI is mostly for now.

const router = useRouter()
const onboarding = useOnboardingStore()
const nodesStore = useNodesStore()
const appStore = useAppStore()

const ready = ref(false)

const enrolledCount = computed(
  () => nodesStore.nodes.filter((n) => n.status === 'online' || !!n.last_heartbeat).length,
)

// Both steps finish somewhere else — on a server, in a terminal, in another
// tab's consent screen — so poll while there is something left to tick, and
// re-check the moment the user comes back to this tab.
let timer: ReturnType<typeof setInterval> | undefined

function stopPolling() {
  if (timer) clearInterval(timer)
  timer = undefined
}

function onVisible() {
  if (document.visibilityState === 'visible' && !onboarding.complete) onboarding.refresh()
}

onMounted(async () => {
  await onboarding.refresh()
  // Only on arrival: if the checklist completes while the user is watching,
  // they get the "all set" card instead of being yanked to another page.
  if (onboarding.complete || onboarding.dismissed) {
    router.replace('/nodes')
    return
  }
  ready.value = true
  timer = setInterval(() => onboarding.refresh(), 5000)
  document.addEventListener('visibilitychange', onVisible)
})

watch(
  () => onboarding.complete,
  (done) => done && stopPolling(),
)

onBeforeUnmount(() => {
  stopPolling()
  document.removeEventListener('visibilitychange', onVisible)
})

function addNode() {
  router.push({ path: '/nodes', query: { add: '1' } })
}

function skip() {
  onboarding.dismiss()
  router.replace('/nodes')
}
</script>

<template>
  <div class="h-full overflow-auto" style="background-color: var(--background)">
    <div v-if="!ready" class="flex h-full items-center justify-center">
      <Loader2 class="h-5 w-5 animate-spin" style="color: var(--muted-foreground)" />
    </div>

    <div v-else class="mx-auto max-w-3xl space-y-6 px-6 py-10">
      <!-- Header -->
      <div v-if="!onboarding.complete" class="space-y-2">
        <div class="flex items-center justify-between gap-4">
          <h1 class="text-2xl font-semibold" style="letter-spacing: -0.015em">
            {{ $t('home.title') }}
          </h1>
          <button
            type="button"
            class="shrink-0 text-sm transition-colors hover:text-foreground"
            style="color: var(--muted-foreground)"
            @click="skip"
          >
            {{ $t('home.skip') }}
          </button>
        </div>
        <p class="text-sm" style="color: var(--muted-foreground)">{{ $t('home.subtitle') }}</p>
        <div class="flex items-center gap-3 pt-1">
          <div class="h-1.5 w-40 overflow-hidden rounded-full" style="background: var(--secondary)">
            <div
              class="h-full rounded-full transition-all duration-500"
              style="background: var(--primary)"
              :style="{ width: `${(onboarding.doneCount / 2) * 100}%` }"
            />
          </div>
          <span class="text-xs" style="color: var(--muted-foreground)">
            {{ $t('home.progress', { done: onboarding.doneCount }) }}
          </span>
        </div>
      </div>

      <!-- All set -->
      <div
        v-else
        class="flex items-start gap-4 rounded-xl border p-5"
        style="border-color: var(--border); background: color-mix(in oklab, var(--primary) 6%, transparent)"
      >
        <PartyPopper class="mt-0.5 h-6 w-6 shrink-0" style="color: var(--primary)" />
        <div class="flex-1 space-y-1">
          <h1 class="text-lg font-semibold">{{ $t('home.allSet.title') }}</h1>
          <p class="text-sm" style="color: var(--muted-foreground)">{{ $t('home.allSet.desc') }}</p>
        </div>
        <Button class="shrink-0" @click="router.push('/nodes')">
          {{ $t('home.allSet.action') }}
          <ArrowRight class="ml-1.5 h-4 w-4" />
        </Button>
      </div>

      <!-- Step 1: a node online -->
      <section class="rounded-xl border p-5" style="border-color: var(--border)">
        <div class="flex items-start gap-4">
          <span
            class="flex h-7 w-7 shrink-0 items-center justify-center rounded-full text-sm font-semibold"
            :style="
              onboarding.hasNode
                ? { background: 'var(--primary)', color: 'var(--primary-foreground)' }
                : { background: 'var(--secondary)', color: 'var(--foreground)' }
            "
          >
            <Check v-if="onboarding.hasNode" class="h-4 w-4" />
            <template v-else>1</template>
          </span>
          <div class="min-w-0 flex-1 space-y-1">
            <h2 class="text-base font-medium">{{ $t('home.step1.title') }}</h2>
            <p v-if="onboarding.hasNode" class="text-sm" style="color: var(--muted-foreground)">
              {{ $t('home.step1.done', { count: enrolledCount }) }}
            </p>
            <template v-else-if="appStore.isAdmin">
              <p class="text-sm" style="color: var(--muted-foreground)">{{ $t('home.step1.desc') }}</p>
              <p v-if="nodesStore.nodes.length > 0" class="text-xs" style="color: var(--muted-foreground)">
                {{ $t('home.step1.waiting') }}
              </p>
            </template>
            <p v-else class="text-sm" style="color: var(--muted-foreground)">{{ $t('home.step1.member') }}</p>
          </div>
          <Button v-if="onboarding.hasNode" variant="ghost" size="sm" class="shrink-0" @click="router.push('/nodes')">
            {{ $t('home.step1.viewNodes') }}
            <ArrowRight class="ml-1.5 h-4 w-4" />
          </Button>
          <Button v-else-if="appStore.isAdmin" size="sm" class="shrink-0" @click="addNode">
            <Plus class="mr-1.5 h-4 w-4" />
            {{ $t('home.step1.action') }}
          </Button>
        </div>
      </section>

      <!-- Step 2: Tolato in the user's own coding agent -->
      <section
        class="rounded-xl border p-5"
        :style="{
          borderColor: onboarding.cliConnected ? 'var(--border)' : 'color-mix(in oklab, var(--primary) 45%, var(--border))',
        }"
      >
        <div class="flex items-start gap-4">
          <span
            class="flex h-7 w-7 shrink-0 items-center justify-center rounded-full text-sm font-semibold"
            :style="
              onboarding.cliConnected
                ? { background: 'var(--primary)', color: 'var(--primary-foreground)' }
                : { background: 'var(--secondary)', color: 'var(--foreground)' }
            "
          >
            <Check v-if="onboarding.cliConnected" class="h-4 w-4" />
            <template v-else>2</template>
          </span>
          <div class="min-w-0 flex-1 space-y-3">
            <div class="space-y-1">
              <h2 class="text-base font-medium">{{ $t('home.step2.title') }}</h2>
              <p class="text-sm" style="color: var(--muted-foreground)">
                {{ onboarding.cliConnected ? $t('home.step2.done') : $t('home.step2.desc') }}
              </p>
            </div>
            <template v-if="!onboarding.cliConnected">
              <InstallPrompt />
              <p class="flex items-center gap-2 text-xs" style="color: var(--muted-foreground)">
                <Loader2 class="h-3.5 w-3.5 animate-spin" />
                {{ $t('home.step2.waiting') }}
              </p>
            </template>
          </div>
        </div>
      </section>
    </div>
  </div>
</template>
