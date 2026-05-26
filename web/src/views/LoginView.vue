<script setup lang="ts">
import { ref } from 'vue'
import { useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { Zap } from 'lucide-vue-next'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { useAppStore } from '@/stores/app'

const { t } = useI18n()
const appStore = useAppStore()
const router = useRouter()

const username = ref('')
const password = ref('')
const loading = ref(false)
const error = ref('')

async function handleLogin() {
  if (!username.value || !password.value) {
    error.value = t('login.errorRequired')
    return
  }

  loading.value = true
  error.value = ''

  try {
    await appStore.login({
      username: username.value,
      password: password.value,
    })
    router.push('/')
  } catch (err: unknown) {
    if (err && typeof err === 'object' && 'response' in err) {
      const axiosErr = err as { response?: { data?: { message?: string } } }
      error.value = axiosErr.response?.data?.message || t('login.errorFailed')
    } else {
      error.value = t('login.errorNetwork')
    }
  } finally {
    loading.value = false
  }
}
</script>

<template>
  <div
    class="relative flex h-screen w-full items-center justify-center overflow-hidden"
    style="background-color: var(--background)"
  >
    <!-- Dot grid texture, faded toward edges -->
    <div
      class="pointer-events-none absolute inset-0 opacity-60"
      style="
        background-image: radial-gradient(circle, var(--border) 1px, transparent 1px);
        background-size: 22px 22px;
        -webkit-mask-image: radial-gradient(ellipse 60% 55% at center, black 0%, transparent 75%);
                mask-image: radial-gradient(ellipse 60% 55% at center, black 0%, transparent 75%);
      "
    />

    <!-- Warm glow behind the brand -->
    <div
      class="pointer-events-none absolute left-1/2 top-1/2 h-[440px] w-[440px] -translate-x-1/2 rounded-full"
      style="
        transform: translate(-50%, calc(-50% - 150px));
        background: radial-gradient(circle, color-mix(in oklab, var(--primary) 22%, transparent) 0%, transparent 65%);
        filter: blur(36px);
      "
    />

    <!-- Hairline frame -->
    <div class="pointer-events-none absolute inset-x-0 top-0 h-px" style="background-color: var(--border)" />
    <div class="pointer-events-none absolute inset-x-0 bottom-0 h-px" style="background-color: var(--border)" />

    <div class="relative z-10 w-full max-w-sm px-6">
      <!-- Brand -->
      <div class="mb-8 flex flex-col items-center gap-4">
        <div
          class="flex h-12 w-12 items-center justify-center rounded-2xl"
          style="
            background-color: var(--primary);
            box-shadow: 0 10px 32px -10px color-mix(in oklab, var(--primary) 65%, transparent);
          "
        >
          <Zap class="h-6 w-6" style="color: var(--primary-foreground)" />
        </div>
        <div class="flex flex-col items-center gap-1.5">
          <h1
            class="text-2xl font-semibold tracking-tight"
            style="color: var(--foreground)"
          >
            {{ $t('login.title') }}
          </h1>
          <p class="text-sm" style="color: var(--muted-foreground)">
            {{ $t('login.subtitle') }}
          </p>
        </div>
      </div>

      <!-- Form -->
      <form
        class="space-y-4 rounded-2xl border p-6"
        style="
          background-color: color-mix(in oklab, var(--card) 92%, transparent);
          border-color: var(--border);
          backdrop-filter: blur(8px);
          box-shadow: 0 1px 0 color-mix(in oklab, var(--foreground) 4%, transparent) inset, 0 20px 40px -24px rgba(0,0,0,0.5);
        "
        @submit.prevent="handleLogin"
      >
        <div class="space-y-1.5">
          <label
            for="login-username"
            class="text-xs font-medium"
            style="color: var(--muted-foreground)"
          >
            {{ $t('login.username') }}
          </label>
          <Input
            id="login-username"
            v-model="username"
            type="text"
            autocomplete="username"
          />
        </div>
        <div class="space-y-1.5">
          <label
            for="login-password"
            class="text-xs font-medium"
            style="color: var(--muted-foreground)"
          >
            {{ $t('login.password') }}
          </label>
          <Input
            id="login-password"
            v-model="password"
            type="password"
            autocomplete="current-password"
          />
        </div>

        <div
          v-if="error"
          class="rounded-lg border px-3 py-2 text-sm"
          style="
            background-color: var(--color-error);
            color: var(--color-error-foreground);
            border-color: color-mix(in oklab, var(--color-error-foreground) 25%, transparent);
          "
        >
          {{ error }}
        </div>

        <Button type="submit" class="w-full" :disabled="loading">
          <span class="inline-flex items-center gap-2">
            {{ loading ? $t('login.signingIn') : $t('login.signIn') }}
            <kbd
              v-if="!loading"
              class="hidden h-4 min-w-4 items-center justify-center rounded border px-1 font-mono text-[10px] leading-none sm:inline-flex"
              style="
                border-color: color-mix(in oklab, var(--primary-foreground) 35%, transparent);
                color: color-mix(in oklab, var(--primary-foreground) 85%, transparent);
              "
            >↵</kbd>
          </span>
        </Button>
      </form>

      <!-- Tiny meta line -->
      <p
        class="mt-6 text-center text-[11px] tracking-wide"
        style="color: color-mix(in oklab, var(--muted-foreground) 70%, transparent)"
      >
        {{ $t('login.meta') }}
      </p>
    </div>
  </div>
</template>
