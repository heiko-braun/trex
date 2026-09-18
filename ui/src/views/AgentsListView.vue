<template>
  <div class="p-6">
    <div class="mb-6 flex items-center justify-between">
      <h1 class="text-2xl font-bold">Agents</h1>
      <button
        @click="store.fetchAll()"
        class="rounded-md border border-border px-3 py-1.5 text-sm hover:bg-secondary"
      >
        Refresh
      </button>
    </div>

    <p v-if="store.isLoading" class="text-muted-foreground">Loading...</p>
    <p v-else-if="store.error" class="text-destructive">{{ store.error }}</p>

    <template v-else>
      <section class="mb-8">
        <h2 class="mb-3 text-lg font-semibold">Discovered</h2>
        <p v-if="store.discovered.length === 0" class="text-muted-foreground">No agents discovered.</p>
        <table v-else class="w-full max-w-4xl border-collapse text-left text-sm">
          <thead>
            <tr class="border-b border-border text-muted-foreground">
              <th class="py-2 pr-4 font-medium">Name</th>
              <th class="py-2 pr-4 font-medium">Type</th>
              <th class="py-2 pr-4 font-medium">Status</th>
              <th class="py-2 pr-4 font-medium"></th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="agent in store.discovered" :key="agent.id" class="border-b border-border/60">
              <td class="py-2 pr-4">{{ agent.name }}</td>
              <td class="py-2 pr-4">{{ agent.type }}</td>
              <td class="py-2 pr-4">
                <span v-if="isRegistered(agent.id)" class="text-green-600">registered</span>
                <span v-else class="text-muted-foreground">not registered</span>
              </td>
              <td class="py-2 pr-4">
                <button
                  v-if="!isRegistered(agent.id)"
                  @click="store.register(agent)"
                  :disabled="store.pendingIds.has(agent.id)"
                  class="rounded-md border border-border px-2 py-1 text-xs hover:bg-secondary disabled:opacity-50"
                >
                  Register
                </button>
              </td>
            </tr>
          </tbody>
        </table>
      </section>

      <section>
        <h2 class="mb-3 text-lg font-semibold">Registered</h2>
        <p v-if="store.registered.length === 0" class="text-muted-foreground">No agents registered.</p>
        <table v-else class="w-full max-w-4xl border-collapse text-left text-sm">
          <thead>
            <tr class="border-b border-border text-muted-foreground">
              <th class="py-2 pr-4 font-medium">Name</th>
              <th class="py-2 pr-4 font-medium">Task Queue</th>
              <th class="py-2 pr-4 font-medium">Worker</th>
              <th class="py-2 pr-4 font-medium"></th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="agent in store.registered" :key="agent.id" class="border-b border-border/60">
              <td class="py-2 pr-4">{{ agent.name }}</td>
              <td class="py-2 pr-4 font-mono text-xs">{{ agent.taskQueue }}</td>
              <td class="py-2 pr-4">
                <span :class="agent.running ? 'text-green-600' : 'text-muted-foreground'">
                  {{ agent.running ? 'running' : 'stopped' }}
                </span>
              </td>
              <td class="py-2 pr-4">
                <button
                  @click="store.unregister(agent.id)"
                  :disabled="store.pendingIds.has(agent.id)"
                  class="rounded-md border border-border px-2 py-1 text-xs hover:bg-secondary disabled:opacity-50"
                >
                  Unregister
                </button>
              </td>
            </tr>
          </tbody>
        </table>
      </section>
    </template>
  </div>
</template>

<script setup lang="ts">
import { onMounted } from 'vue'
import { useAgentsStore } from '@/stores/agents'

const store = useAgentsStore()

function isRegistered(agentId: string) {
  return store.registered.some((r) => r.id === agentId)
}

onMounted(() => {
  store.fetchAll()
})
</script>
