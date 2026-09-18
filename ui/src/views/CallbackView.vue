<template>
  <div class="flex min-h-screen items-center justify-center">
    <div class="text-center">
      <div
        class="mb-4 inline-block h-8 w-8 animate-spin rounded-full border-4 border-solid border-current border-r-transparent"
      ></div>
      <p class="text-neutral-500">Completing sign in...</p>
    </div>
  </div>
</template>

<script setup lang="ts">
import { onMounted } from 'vue'
import { useRouter } from 'vue-router'
import { useAuthStore } from '@/stores/auth'

const router = useRouter()
const authStore = useAuthStore()

onMounted(async () => {
  try {
    await authStore.init()

    if (authStore.isAuthenticated) {
      const returnUrl = sessionStorage.getItem('returnUrl') || '/'
      sessionStorage.removeItem('returnUrl')
      router.push(returnUrl)
    } else {
      console.error('Authentication callback completed but user is not authenticated')
      router.push('/login')
    }
  } catch (error) {
    console.error('Authentication callback failed:', error)
    router.push('/login')
  }
})
</script>
