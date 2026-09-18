import { defineStore } from 'pinia'
import { ref, computed } from 'vue'
import { authService } from '@/services/auth'
import { setOnUnauthorized } from '@/services/http'
import type { User } from '@/types'

export const useAuthStore = defineStore('auth', () => {
  const isLoading = ref(false)
  const isInitialized = ref(false)

  const isAuthenticated = computed(() => authService.isAuthenticated())
  const user = computed<User | null>(() => authService.getUserProfile())

  function handleTokenExpired() {
    console.warn('[AuthStore] Session expired, redirecting to login...')
    window.location.href = '/login'
  }

  async function init() {
    if (isInitialized.value) return

    isLoading.value = true
    try {
      await authService.initialize()
      authService.onTokenExpired(handleTokenExpired)
      setOnUnauthorized(handleTokenExpired)
      isInitialized.value = true
    } catch (error) {
      console.error('[AuthStore] Failed to initialize auth:', error)
    } finally {
      isLoading.value = false
    }
  }

  async function login() {
    await authService.login()
  }

  async function logout() {
    await authService.logout()
  }

  return {
    user,
    isLoading,
    isAuthenticated,
    init,
    login,
    logout,
  }
})
