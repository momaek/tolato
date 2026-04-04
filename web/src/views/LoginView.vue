<script setup lang="ts">
import { ref } from 'vue'
import { useRouter } from 'vue-router'
import { Zap } from 'lucide-vue-next'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { useAppStore } from '@/stores/app'

const appStore = useAppStore()
const router = useRouter()

const username = ref('')
const password = ref('')
const loading = ref(false)
const error = ref('')

async function handleLogin() {
  if (!username.value || !password.value) {
    error.value = 'Please enter username and password'
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
      error.value = axiosErr.response?.data?.message || 'Login failed'
    } else {
      error.value = 'Network error'
    }
  } finally {
    loading.value = false
  }
}
</script>

<template>
  <div class="flex h-screen w-full items-center justify-center" style="background-color: var(--background)">
    <div class="w-full max-w-sm space-y-6 px-4">
      <!-- Logo -->
      <div class="flex flex-col items-center gap-3">
        <div
          class="flex h-12 w-12 items-center justify-center rounded-xl"
          style="background-color: var(--primary)"
        >
          <Zap class="h-6 w-6" style="color: var(--primary-foreground)" />
        </div>
        <h1 class="text-2xl font-semibold" style="color: var(--foreground)">Tolato</h1>
        <p class="text-sm" style="color: var(--muted-foreground)">
          AI-powered VPS management
        </p>
      </div>

      <!-- Form -->
      <form class="space-y-4" @submit.prevent="handleLogin">
        <div class="space-y-2">
          <Input
            v-model="username"
            type="text"
            placeholder="Username"
            autocomplete="username"
          />
        </div>
        <div class="space-y-2">
          <Input
            v-model="password"
            type="password"
            placeholder="Password"
            autocomplete="current-password"
          />
        </div>

        <div
          v-if="error"
          class="rounded-lg px-3 py-2 text-sm"
          style="background-color: var(--color-error); color: var(--color-error-foreground)"
        >
          {{ error }}
        </div>

        <Button type="submit" class="w-full" :disabled="loading">
          {{ loading ? 'Signing in...' : 'Sign in' }}
        </Button>
      </form>
    </div>
  </div>
</template>
