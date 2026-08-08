<script setup lang="ts">
/**
 * Consent screen for `tolato auth login`.
 *
 * The CLI opens this page with a loopback redirect_uri it is already listening
 * on. Approving mints a one-minute, single-use code which we hand back through
 * that redirect; the CLI then exchanges the code — with a verifier that never
 * appeared in any URL — for the actual API key. The key itself never touches
 * this page or the address bar.
 *
 * Nothing here happens without a click. The mint endpoint is a POST carrying
 * the session precisely so that merely landing on a URL cannot produce a
 * credential.
 */
import { computed, ref } from 'vue'
import { useRoute } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { Terminal, Loader2, ShieldCheck } from 'lucide-vue-next'
import { Button } from '@/components/ui/button'
import { Separator } from '@/components/ui/separator'
import { authorizeCLI } from '@/services/api'
import { useAppStore } from '@/stores/app'

const { t } = useI18n()
const route = useRoute()
const appStore = useAppStore()

const redirectUri = String(route.query.redirect_uri ?? '')
const state = String(route.query.state ?? '')
const codeChallenge = String(route.query.code_challenge ?? '')
const label = String(route.query.label ?? '')

const permission = ref<'readonly' | 'writable'>('readonly')
const submitting = ref(false)
const error = ref('')

/**
 * Only ever send the browser to a loopback address. The server enforces this
 * too — that is the check that counts — but a malformed or hostile parameter
 * should not get as far as a navigation off this origin.
 */
const parsedRedirect = computed(() => {
  try {
    const u = new URL(redirectUri)
    const loopback = u.hostname === '127.0.0.1' || u.hostname === '[::1]' || u.hostname === '::1'
    if (u.protocol !== 'http:' || !loopback || u.pathname !== '/callback' || !u.port) return null
    return u
  } catch {
    return null
  }
})

const valid = computed(() => parsedRedirect.value !== null && codeChallenge.length >= 43)

function leave(params: Record<string, string>) {
  const u = parsedRedirect.value
  if (!u) return
  for (const [k, v] of Object.entries(params)) u.searchParams.set(k, v)
  window.location.href = u.toString()
}

async function approve() {
  if (!valid.value || submitting.value) return
  submitting.value = true
  error.value = ''
  try {
    const { code } = await authorizeCLI({
      redirect_uri: redirectUri,
      code_challenge: codeChallenge,
      label,
      permission: permission.value,
    })
    leave(state ? { code, state } : { code })
  } catch (err: unknown) {
    const axiosErr = err as { response?: { data?: { message?: string } } }
    error.value = axiosErr?.response?.data?.message || t('cliAuth.failed')
    submitting.value = false
  }
}

function deny() {
  // Tell the waiting CLI rather than leaving it to time out.
  leave(state ? { error: 'access_denied', state } : { error: 'access_denied' })
}
</script>

<template>
  <div class="flex min-h-screen items-center justify-center p-6" style="background-color: var(--background)">
    <div class="w-full max-w-md space-y-6 rounded-xl border p-8" style="border-color: var(--border)">
      <div class="flex flex-col items-center gap-3 text-center">
        <div class="rounded-full p-3" style="background-color: var(--secondary)">
          <Terminal class="h-6 w-6" />
        </div>
        <h1 class="text-xl font-semibold">{{ $t('cliAuth.title') }}</h1>
        <p class="text-sm" style="color: var(--muted-foreground)">
          {{ $t('cliAuth.subtitle', { label: label || $t('cliAuth.unknownMachine') }) }}
        </p>
      </div>

      <template v-if="!valid">
        <div class="rounded-lg p-4 text-sm" style="background-color: var(--secondary)">
          {{ $t('cliAuth.invalidRequest') }}
        </div>
      </template>

      <template v-else>
        <Separator />

        <dl class="space-y-3 text-sm">
          <div class="flex items-center justify-between gap-4">
            <dt style="color: var(--muted-foreground)">{{ $t('cliAuth.account') }}</dt>
            <dd class="font-medium">{{ appStore.user?.username }}</dd>
          </div>
          <div class="flex items-center justify-between gap-4">
            <dt style="color: var(--muted-foreground)">{{ $t('cliAuth.returnsTo') }}</dt>
            <dd class="font-mono text-xs">{{ parsedRedirect?.host }}</dd>
          </div>
        </dl>

        <div class="space-y-2">
          <p class="text-sm font-medium">{{ $t('cliAuth.permission') }}</p>
          <label
            v-for="opt in (['readonly', 'writable'] as const)"
            :key="opt"
            class="flex cursor-pointer items-start gap-3 rounded-lg border p-3"
            style="border-color: var(--border)"
          >
            <input v-model="permission" type="radio" :value="opt" class="mt-1" />
            <span>
              <span class="block text-sm font-medium">{{ $t(`cliAuth.permissions.${opt}.label`) }}</span>
              <span class="block text-xs" style="color: var(--muted-foreground)">
                {{ $t(`cliAuth.permissions.${opt}.description`) }}
              </span>
            </span>
          </label>
        </div>

        <p v-if="error" class="text-sm" style="color: var(--color-error-foreground)">{{ error }}</p>

        <div class="flex gap-3">
          <Button variant="outline" class="flex-1" :disabled="submitting" @click="deny">
            {{ $t('cliAuth.deny') }}
          </Button>
          <Button class="flex-1" :disabled="submitting" @click="approve">
            <Loader2 v-if="submitting" class="mr-2 h-4 w-4 animate-spin" />
            {{ $t('cliAuth.approve') }}
          </Button>
        </div>

        <p class="flex items-start gap-2 text-xs" style="color: var(--muted-foreground)">
          <ShieldCheck class="mt-0.5 h-3.5 w-3.5 shrink-0" />
          {{ $t('cliAuth.revokeHint') }}
        </p>
      </template>
    </div>
  </div>
</template>
