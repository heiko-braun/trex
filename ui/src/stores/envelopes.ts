import { defineStore } from 'pinia'
import { ref } from 'vue'
import { authedFetch } from '@/services/http'
import type { EnvelopeIndex } from '@/types'

export const useEnvelopesStore = defineStore('envelopes', () => {
  const index = ref<EnvelopeIndex | null>(null)
  const blobContent = ref<string | null>(null)
  const isLoading = ref(false)
  const isLoadingBlob = ref(false)
  const error = ref<string | null>(null)

  async function fetchEnvelope(workflowId: string) {
    isLoading.value = true
    error.value = null
    index.value = null
    blobContent.value = null
    try {
      const response = await authedFetch(`/envelopes/${encodeURIComponent(workflowId)}`)
      if (!response.ok) {
        throw new Error(
          response.status === 404
            ? `No envelope found for workflow ${workflowId}`
            : `Failed to load envelope: ${response.status}`,
        )
      }
      index.value = (await response.json()) as EnvelopeIndex
    } catch (e) {
      error.value = e instanceof Error ? e.message : String(e)
    } finally {
      isLoading.value = false
    }
  }

  async function fetchBlob(workflowId: string, digest: string) {
    isLoadingBlob.value = true
    error.value = null
    blobContent.value = null
    try {
      const response = await authedFetch(
        `/envelopes/${encodeURIComponent(workflowId)}/${encodeURIComponent(digest)}`,
      )
      if (!response.ok) {
        throw new Error(`Failed to load blob: ${response.status}`)
      }
      blobContent.value = await response.text()
    } catch (e) {
      error.value = e instanceof Error ? e.message : String(e)
    } finally {
      isLoadingBlob.value = false
    }
  }

  return { index, blobContent, isLoading, isLoadingBlob, error, fetchEnvelope, fetchBlob }
})
