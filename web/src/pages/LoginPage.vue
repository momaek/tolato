<script setup lang="ts">
import { reactive, ref } from "vue"
import { useRouter } from "vue-router"
import { controlApi, ApiError } from "@/shared/api/control-api"
import { useSessionStore } from "@/entities/session/store"
import { Button, Card, CardContent, CardDescription, CardHeader, CardTitle, Input } from "@/shared/ui"

const router = useRouter()
const sessionStore = useSessionStore()

const form = reactive({
  username: "admin",
  password: "admin",
})
const isSubmitting = ref(false)
const errorMessage = ref("")

async function login() {
  isSubmitting.value = true
  errorMessage.value = ""

  try {
    const { session, token } = await controlApi.login(form)
    sessionStore.setSession(session, token)
    await router.push("/console/agent")
  } catch (error) {
    if (error instanceof ApiError && error.status === 401) {
      errorMessage.value = "用户名或密码错误。"
    } else {
      errorMessage.value = error instanceof Error ? error.message : "登录失败"
    }
  } finally {
    isSubmitting.value = false
  }
}
</script>

<template>
  <div class="flex min-h-screen items-center justify-center bg-background px-6">
    <Card class="w-full max-w-md border-none shadow-sm">
      <CardHeader class="space-y-2">
        <p class="text-sm font-semibold uppercase tracking-[0.18em] text-muted-foreground">
          ToLaTo
        </p>
        <CardTitle class="text-3xl">Sign in</CardTitle>
        <CardDescription>
          使用控制台管理员账号登录。
        </CardDescription>
      </CardHeader>
      <CardContent class="space-y-4">
        <form class="space-y-4" @submit.prevent="login">
          <div class="space-y-2">
            <label class="text-sm font-medium text-foreground" for="username">Username</label>
            <Input id="username" v-model="form.username" autocomplete="username" />
          </div>

          <div class="space-y-2">
            <label class="text-sm font-medium text-foreground" for="password">Password</label>
            <Input id="password" v-model="form.password" type="password" autocomplete="current-password" />
          </div>

          <p v-if="errorMessage" class="text-sm text-destructive">
            {{ errorMessage }}
          </p>

          <Button class="w-full" type="submit" :disabled="isSubmitting">
            {{ isSubmitting ? "Signing in..." : "Sign in" }}
          </Button>
        </form>
      </CardContent>
    </Card>
  </div>
</template>
