<script setup lang="ts">
import { computed, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'

import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { useAuthStore } from '@/entities/auth/model/auth.store'

const authStore = useAuthStore()
const route = useRoute()
const router = useRouter()
const { t } = useI18n()

const username = ref('admin')
const password = ref('')

const nextPath = computed(() => {
  const value = route.query.next
  return typeof value === 'string' && value.startsWith('/') ? value : '/console'
})

async function handleSubmit() {
  if (!username.value.trim() || !password.value.trim()) {
    authStore.error = t('login.validation.required')
    return
  }

  const session = await authStore.login(username.value, password.value)
  if (!session) {
    return
  }

  await router.replace(nextPath.value)
}
</script>

<template>
  <div
    class="relative min-h-screen overflow-hidden bg-[radial-gradient(circle_at_top,_rgba(28,117,188,0.18),_transparent_38%),linear-gradient(180deg,_rgba(248,250,252,0.96)_0%,_rgba(241,245,249,0.98)_100%)]"
  >
    <div
      class="absolute inset-0 bg-[linear-gradient(120deg,rgba(15,23,42,0.03),transparent_35%,rgba(15,23,42,0.06))]"
    />
    <div
      class="relative mx-auto flex min-h-screen max-w-6xl items-center justify-center px-6 py-10"
    >
      <div class="w-full max-w-[420px] space-y-6">
        <div
          class="mx-auto inline-flex items-center gap-3 rounded-full border border-white/70 bg-white/70 px-4 py-2 shadow-sm backdrop-blur"
        >
          <div
            class="flex size-9 items-center justify-center rounded-2xl bg-primary/10 ring-1 ring-primary/20"
          >
            <img
              alt="ToLaTo"
              class="size-6"
              src="/logo/tolato-logo-icon-dark.svg"
            />
          </div>
          <div>
            <p
              class="text-[11px] font-semibold uppercase tracking-[0.3em] text-muted-foreground"
            >
              ToLaTo
            </p>
            <p class="text-sm font-semibold text-foreground">
              {{ t('appShell.subtitle') }}
            </p>
          </div>
        </div>

        <Card
          class="border-white/80 bg-white/88 shadow-2xl shadow-slate-900/8 backdrop-blur"
        >
          <CardHeader class="space-y-2 pb-4">
            <CardTitle class="text-2xl">{{ t('login.form.title') }}</CardTitle>
            <CardDescription>{{ t('login.form.description') }}</CardDescription>
          </CardHeader>
          <CardContent class="space-y-5">
            <form class="space-y-4" @submit.prevent="handleSubmit">
              <label class="block space-y-2">
                <span class="text-sm font-medium text-slate-700">{{
                  t('login.form.username')
                }}</span>
                <Input
                  v-model="username"
                  autocomplete="username"
                  :placeholder="t('login.form.usernamePlaceholder')"
                />
              </label>

              <label class="block space-y-2">
                <span class="text-sm font-medium text-slate-700">{{
                  t('login.form.password')
                }}</span>
                <Input
                  v-model="password"
                  type="password"
                  autocomplete="current-password"
                  :placeholder="t('login.form.passwordPlaceholder')"
                />
              </label>

              <p
                v-if="authStore.error"
                class="rounded-2xl border border-destructive/20 bg-destructive/5 px-3 py-2 text-sm text-destructive"
              >
                {{ authStore.error }}
              </p>

              <Button
                class="h-11 w-full rounded-2xl"
                :disabled="authStore.loggingIn"
                type="submit"
              >
                {{
                  authStore.loggingIn
                    ? t('login.form.submitting')
                    : t('login.form.submit')
                }}
              </Button>
            </form>
          </CardContent>
        </Card>
      </div>
    </div>
  </div>
</template>
