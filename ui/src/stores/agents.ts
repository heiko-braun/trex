import { defineStore } from 'pinia'
import { ref } from 'vue'
import { authedFetch } from '@/services/http'
import type { DiscoveredAgent, RegisteredAgent } from '@/types'

export const useAgentsStore = defineStore('agents', () => {
  const discovered = ref<DiscoveredAgent[]>([])
  const registered = ref<RegisteredAgent[]>([])
  const isLoading = ref(false)
  const error = ref<string | null>(null)
  const pendingIds = ref<Set<string>>(new Set())

  async function fetchAll() {
    isLoading.value = true
    error.value = null
    try {
      const [discoverResponse, registeredResponse] = await Promise.all([
        authedFetch('/agents'),
        authedFetch('/agents/registered'),
      ])
      if (!discoverResponse.ok) {
        throw new Error(`Failed to discover agents: ${discoverResponse.status}`)
      }
      if (!registeredResponse.ok) {
        throw new Error(`Failed to load registered agents: ${registeredResponse.status}`)
      }
      discovered.value = (await discoverResponse.json()) as DiscoveredAgent[]
      registered.value = (await registeredResponse.json()) as RegisteredAgent[]
    } catch (e) {
      error.value = e instanceof Error ? e.message : String(e)
    } finally {
      isLoading.value = false
    }
  }

  async function register(agent: DiscoveredAgent) {
    pendingIds.value.add(agent.id)
    error.value = null
    try {
      const response = await authedFetch(`/agents/${agent.id}/register`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ name: agent.name }),
      })
      if (!response.ok) {
        const body = await response.json().catch(() => null)
        throw new Error(body?.error || `Failed to register agent: ${response.status}`)
      }
      const reg = (await response.json()) as RegisteredAgent
      registered.value = [...registered.value.filter((r) => r.id !== reg.id), reg]
    } catch (e) {
      error.value = e instanceof Error ? e.message : String(e)
    } finally {
      pendingIds.value.delete(agent.id)
    }
  }

  async function unregister(agentId: string) {
    pendingIds.value.add(agentId)
    error.value = null
    try {
      const response = await authedFetch(`/agents/${agentId}/register`, { method: 'DELETE' })
      if (!response.ok) {
        const body = await response.json().catch(() => null)
        throw new Error(body?.error || `Failed to unregister agent: ${response.status}`)
      }
      registered.value = registered.value.filter((r) => r.id !== agentId)
    } catch (e) {
      error.value = e instanceof Error ? e.message : String(e)
    } finally {
      pendingIds.value.delete(agentId)
    }
  }

  return { discovered, registered, isLoading, error, pendingIds, fetchAll, register, unregister }
})
