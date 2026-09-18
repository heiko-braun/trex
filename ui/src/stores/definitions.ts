import { defineStore } from 'pinia'
import { ref } from 'vue'
import { authedFetch } from '@/services/http'
import type { Definition } from '@/types'

export const useDefinitionsStore = defineStore('definitions', () => {
  const definitions = ref<Definition[]>([])
  const isLoading = ref(false)
  const error = ref<string | null>(null)

  async function fetchAll() {
    isLoading.value = true
    error.value = null
    try {
      const response = await authedFetch('/definitions')
      if (!response.ok) {
        throw new Error(`Failed to load definitions: ${response.status}`)
      }
      definitions.value = (await response.json()) as Definition[]
    } catch (e) {
      error.value = e instanceof Error ? e.message : String(e)
    } finally {
      isLoading.value = false
    }
  }

  return { definitions, isLoading, error, fetchAll }
})
