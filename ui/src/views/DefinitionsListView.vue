<template>
  <div class="flex h-full">
    <section class="w-full max-w-2xl shrink-0 overflow-y-auto border-r border-neutral-200 p-6" :class="{ 'hidden md:block': selected }">
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

      <table v-else class="w-full border-collapse text-left text-sm">
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
            :class="{ 'bg-neutral-100': isSelected(def) }"
            @click="select(def)"
          >
            <td class="py-2 pr-4">{{ def.tenant }}</td>
            <td class="py-2 pr-4">{{ def.name }}</td>
            <td class="py-2 pr-4 font-mono text-xs">{{ def.buildId }}</td>
            <td class="py-2 pr-4">{{ def.status }}</td>
            <td class="py-2 pr-4 text-neutral-500">{{ new Date(def.createdAt).toLocaleString() }}</td>
          </tr>
        </tbody>
      </table>
    </section>

    <section v-if="selected" class="flex min-w-0 flex-1 flex-col overflow-hidden">
      <header class="shrink-0 border-b border-neutral-200 bg-white px-6 py-4">
        <nav class="mb-2 flex items-center gap-1.5 text-xs text-neutral-500">
          <RouterLink :to="{ name: 'definitions' }" class="hover:text-neutral-900 hover:underline">
            Workflow Definitions
          </RouterLink>
          <span aria-hidden="true">/</span>
          <RouterLink :to="{ name: 'definitions' }" class="hover:text-neutral-900 hover:underline">
            {{ selected.tenant }}
          </RouterLink>
          <span aria-hidden="true">/</span>
          <span class="text-neutral-900">{{ selected.name }}</span>
        </nav>

        <div class="flex items-center justify-between">
          <div>
            <h2 class="text-lg font-semibold">{{ selected.tenant }}/{{ selected.name }}</h2>
            <p class="font-mono text-xs text-neutral-500">{{ selected.buildId }}</p>
          </div>
          <RouterLink
            :to="{ name: 'definitions' }"
            class="rounded-md border border-neutral-300 px-3 py-1.5 text-sm hover:bg-neutral-50"
          >
            Close
          </RouterLink>
        </div>
      </header>

      <div class="min-h-0 flex-1 overflow-y-auto p-6">
        <dl class="mb-4 grid grid-cols-2 gap-2 text-sm">
          <dt class="text-neutral-500">Status</dt>
          <dd>{{ selected.status }}</dd>
          <dt class="text-neutral-500">Created</dt>
          <dd>{{ new Date(selected.createdAt).toLocaleString() }}</dd>
        </dl>

        <pre class="overflow-x-auto rounded-md bg-neutral-900 p-4 text-xs text-neutral-100"><code>{{ selected.yaml }}</code></pre>
      </div>
    </section>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted } from 'vue'
import { useRoute, useRouter, RouterLink } from 'vue-router'
import { useDefinitionsStore } from '@/stores/definitions'
import type { Definition } from '@/types'

const route = useRoute()
const router = useRouter()
const store = useDefinitionsStore()

const selected = computed<Definition | undefined>(() =>
  store.definitions.find((d) => d.tenant === route.params.tenant && d.name === route.params.name),
)

function isSelected(def: Definition) {
  return def.tenant === route.params.tenant && def.name === route.params.name
}

function select(def: Definition) {
  router.push({ name: 'definition-detail', params: { tenant: def.tenant, name: def.name } })
}

onMounted(() => {
  store.fetchAll()
})
</script>
