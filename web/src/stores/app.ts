import { defineStore } from 'pinia'
import { ref, computed } from 'vue'
import { login as apiLogin } from '@/services/api'
import type { LoginRequest } from '@/types/api'
import router from '@/router'

export const useAppStore = defineStore('app', () => {
  const token = ref<string | null>(localStorage.getItem('token'))

  const isAuthenticated = computed(() => !!token.value)

  async function login(credentials: LoginRequest) {
    const res = await apiLogin(credentials)
    token.value = res.token
    localStorage.setItem('token', res.token)
  }

  function logout() {
    token.value = null
    localStorage.removeItem('token')
    router.push('/login')
  }

  return {
    token,
    isAuthenticated,
    login,
    logout,
  }
})
