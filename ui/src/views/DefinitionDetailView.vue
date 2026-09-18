<template>
  <div class="flex h-full flex-col">
    <header class="shrink-0 border-b border-neutral-200 bg-white px-6 py-4">
      <nav class="mb-2 flex items-center gap-1.5 text-xs text-neutral-500">
        <RouterLink :to="{ name: 'definitions' }" class="hover:text-neutral-900 hover:underline">
          Workflow Definitions
        </RouterLink>
        <span aria-hidden="true">/</span>
        <span class="text-neutral-900">{{ tenant }}</span>
        <span aria-hidden="true">/</span>
        <span class="text-neutral-900">{{ name }}</span>
      </nav>

      <div class="flex items-center justify-between">
        <div>
          <h1 class="text-lg font-semibold">{{ tenant }}/{{ name }}</h1>
          <p v-if="selected" class="font-mono text-xs text-neutral-500">{{ selected.buildId }}</p>
        </div>
        <RouterLink
          :to="{ name: 'definitions' }"
          class="rounded-md border border-neutral-300 px-3 py-1.5 text-sm hover:bg-neutral-50"
        >
          Back to list
        </RouterLink>
      </div>
    </header>

    <div class="min-h-0 flex-1 overflow-y-auto p-6">
      <p v-if="store.isLoading" class="text-neutral-500">Loading...</p>
      <p v-else-if="store.error" class="text-red-600">{{ store.error }}</p>
      <p v-else-if="!selected" class="text-neutral-500">No such definition.</p>

      <template v-else>
        <dl class="mb-4 grid grid-cols-2 gap-2 text-sm">
          <dt class="text-neutral-500">Status</dt>
          <dd>{{ selected.status }}</dd>
          <dt class="text-neutral-500">Created</dt>
          <dd>{{ new Date(selected.createdAt).toLocaleString() }}</dd>
        </dl>

        <pre class="overflow-x-auto rounded-md bg-neutral-900 p-4 text-xs text-neutral-100"><code>{{ selected.yaml }}</code></pre>
      </template>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted } from 'vue'
import { useRoute, RouterLink } from 'vue-router'
import { useDefinitionsStore } from '@/stores/definitions'
import type { Definition } from '@/types'

const route = useRoute()
const store = useDefinitionsStore()

const tenant = computed(() => route.params.tenant as string)
const name = computed(() => route.params.name as string)

const selected = computed<Definition | undefined>(() =>
  store.definitions.find((d) => d.tenant === tenant.value && d.name === name.value),
)

onMounted(() => {
  if (store.definitions.length === 0) {
    store.fetchAll()
  }
})
</script>
