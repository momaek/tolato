import { defineStore } from 'pinia'
import { ref, computed } from 'vue'
import { login as apiLogin, getCurrentUser } from '@/services/api'
import type { LoginRequest, UserItem } from '@/types/api'
import router from '@/router'

const USER_KEY = 'user'

function loadStoredUser(): UserItem | null {
  const raw = localStorage.getItem(USER_KEY)
  if (!raw) return null
  try {
    return JSON.parse(raw) as UserItem
  } catch {
    localStorage.removeItem(USER_KEY)
    return null
  }
}

export const useAppStore = defineStore('app', () => {
  const token = ref<string | null>(localStorage.getItem('token'))
  // Cached so a page reload renders the right navigation immediately instead of
  // flashing the member view while /auth/me is in flight. The server re-checks
  // the role on every request, so a stale copy here can't grant anything.
  const user = ref<UserItem | null>(loadStoredUser())

  const isAuthenticated = computed(() => !!token.value)
  const isAdmin = computed(() => user.value?.role === 'admin')

  function setUser(u: UserItem | null) {
    user.value = u
    if (u) {
      localStorage.setItem(USER_KEY, JSON.stringify(u))
    } else {
      localStorage.removeItem(USER_KEY)
    }
  }

  async function login(credentials: LoginRequest) {
    const res = await apiLogin(credentials)
    token.value = res.token
    localStorage.setItem('token', res.token)
    setUser(res.user)
  }

  // Adopts a token minted by the single sign-on callback, then fetches the
  // identity behind it. Unlike login() there are no credentials to post — the
  // server already authenticated the user against the identity provider.
  async function adoptToken(newToken: string) {
    token.value = newToken
    localStorage.setItem('token', newToken)
    setUser(await getCurrentUser())
  }

  // Refreshes the cached identity from the server. Called on app start so a
  // role change made while the user was away takes effect on the next load.
  async function refreshUser() {
    if (!token.value) return
    try {
      setUser(await getCurrentUser())
    } catch {
      // 401 is already handled by the axios interceptor, which redirects to
      // login; anything else leaves the cached copy in place.
    }
  }

  function logout() {
    token.value = null
    setUser(null)
    localStorage.removeItem('token')
    router.push('/login')
  }

  return {
    token,
    user,
    isAuthenticated,
    isAdmin,
    login,
    logout,
    adoptToken,
    refreshUser,
  }
})
