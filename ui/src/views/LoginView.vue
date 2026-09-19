<template>
  <div class="flex min-h-screen items-center justify-center bg-muted">
    <div class="w-full max-w-md space-y-8 rounded-lg border border-border bg-card p-8 shadow-lg">
      <div class="text-center">
        <img src="/favicon.svg" alt="" width="40" height="40" class="mx-auto mb-3" />
        <h1 class="text-3xl font-bold">Trex Workflow Server</h1>
        <p class="mt-2 text-muted-foreground">Sign in to view stored workflow definitions</p>
      </div>

      <button
        @click="handleLogin"
        :disabled="isLoading"
        class="w-full rounded-md bg-primary px-4 py-3 text-primary-foreground hover:opacity-90 disabled:opacity-50 disabled:cursor-not-allowed transition-opacity"
      >
        <span v-if="isLoading">Signing in...</span>
        <span v-else>Sign in with SSO</span>
      </button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { useRoute } from 'vue-router'
import { useAuthStore } from '@/stores/auth'

const route = useRoute()
const authStore = useAuthStore()
const isLoading = ref(false)

async function handleLogin() {
  try {
    isLoading.value = true
    const returnUrl = route.query.returnUrl as string
    if (returnUrl) {
      sessionStorage.setItem('returnUrl', returnUrl)
    }
    await authStore.login()
  } catch (error) {
    console.error('Login failed:', error)
    isLoading.value = false
  }
}
</script>
