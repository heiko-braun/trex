<template>
  <div class="p-6">
    <div class="mb-6 flex items-center justify-between">
      <h1 class="text-2xl font-bold">Workflow Definitions</h1>
      <button
        @click="store.fetchAll()"
        class="rounded-md border border-neutral-300 px-3 py-1.5 text-sm hover:bg-neutral-50"
      >
        Refresh
      </button>
    </div>

    <p v-if="store.isLoading" class="text-neutral-500">Loading...</p>
    <p v-else-if="store.error" class="text-red-600">{{ store.error }}</p>
    <p v-else-if="store.definitions.length === 0" class="text-neutral-500">No definitions published yet.</p>

    <table v-else class="w-full max-w-4xl border-collapse text-left text-sm">
      <thead>
        <tr class="border-b border-neutral-200 text-neutral-500">
          <th class="py-2 pr-4 font-medium">Tenant</th>
          <th class="py-2 pr-4 font-medium">Name</th>
          <th class="py-2 pr-4 font-medium">Build ID</th>
          <th class="py-2 pr-4 font-medium">Status</th>
          <th class="py-2 pr-4 font-medium">Created</th>
        </tr>
      </thead>
      <tbody>
        <tr
          v-for="def in store.definitions"
          :key="`${def.tenant}/${def.name}/${def.buildId}`"
          class="cursor-pointer border-b border-neutral-100 hover:bg-neutral-50"
          @click="openDetail(def)"
        >
          <td class="py-2 pr-4">{{ def.tenant }}</td>
          <td class="py-2 pr-4">{{ def.name }}</td>
          <td class="py-2 pr-4 font-mono text-xs">{{ def.buildId }}</td>
          <td class="py-2 pr-4">{{ def.status }}</td>
          <td class="py-2 pr-4 text-neutral-500">{{ new Date(def.createdAt).toLocaleString() }}</td>
        </tr>
      </tbody>
    </table>
  </div>
</template>

<script setup lang="ts">
import { onMounted } from 'vue'
import { useRouter } from 'vue-router'
import { useDefinitionsStore } from '@/stores/definitions'
import type { Definition } from '@/types'

const router = useRouter()
const store = useDefinitionsStore()

function openDetail(def: Definition) {
  router.push({ name: 'definition-detail', params: { tenant: def.tenant, name: def.name } })
}

onMounted(() => {
  store.fetchAll()
})
</script>
