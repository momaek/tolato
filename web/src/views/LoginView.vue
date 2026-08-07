<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { Zap, Loader2 } from 'lucide-vue-next'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Separator } from '@/components/ui/separator'
import { getOIDCStatus } from '@/services/api'
import { useAppStore } from '@/stores/app'

const { t } = useI18n()
const appStore = useAppStore()
const route = useRoute()
const router = useRouter()

const username = ref('')
const password = ref('')
const loading = ref(false)
const error = ref('')

// --- Single sign-on ---

const ssoEnabled = ref(false)
const ssoReturning = ref(false)

/** Server-side failures arrive as a fixed code, never a raw error string. */
const SSO_ERRORS: Record<string, string> = {
  sso_unavailable: 'login.sso.errorUnavailable',
  sso_denied: 'login.sso.errorDenied',
  sso_state: 'login.sso.errorState',
  sso_no_account: 'login.sso.errorNoAccount',
  sso_disabled: 'login.sso.errorDisabled',
  sso_failed: 'login.sso.errorFailed',
}

/**
 * Completes an SSO round trip. The server hands the token back in the URL
 * fragment — fragments never reach a server, so the token stays out of access
 * logs and the Referer header. We consume it and wipe the hash immediately so
 * it doesn't linger in browser history.
 */
async function consumeSSOFragment(): Promise<boolean> {
  const hash = window.location.hash.replace(/^#/, '')
  if (!hash) return false

  const params = new URLSearchParams(hash)
  const token = params.get('token')
  if (!token) return false

  history.replaceState(null, '', window.location.pathname)
  ssoReturning.value = true
  try {
    await appStore.adoptToken(token)
    router.replace('/')
    return true
  } catch {
    error.value = t('login.sso.errorFailed')
    return false
  } finally {
    ssoReturning.value = false
  }
}

function startSSO() {
  // A full navigation, not fetch: the browser has to follow the redirect to
  // the identity provider and carry the state cookie back.
  window.location.href = '/api/auth/oidc/login'
}

onMounted(async () => {
  if (await consumeSSOFragment()) return

  const code = route.query.sso_error
  if (typeof code === 'string') {
    error.value = t(SSO_ERRORS[code] ?? 'login.sso.errorFailed')
    router.replace({ query: {} })
  }

  try {
    ssoEnabled.value = (await getOIDCStatus()).enabled
  } catch {
    // Older server or a transient failure — just leave the button hidden.
  }
})

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
        <template v-if="ssoEnabled">
          <div class="relative py-1">
            <Separator />
            <span
              class="absolute left-1/2 top-1/2 -translate-x-1/2 -translate-y-1/2 px-2 text-[11px]"
              style="background-color: var(--card); color: var(--muted-foreground)"
            >
              {{ $t('login.sso.divider') }}
            </span>
          </div>
          <Button
            type="button"
            variant="outline"
            class="w-full"
            :disabled="ssoReturning"
            @click="startSSO"
          >
            <Loader2 v-if="ssoReturning" class="mr-2 h-4 w-4 animate-spin" />
            {{ $t('login.sso.signIn') }}
          </Button>
        </template>
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
