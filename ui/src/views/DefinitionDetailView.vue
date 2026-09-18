<template>
  <div class="flex h-full flex-col">
    <header class="shrink-0 border-b border-border bg-card px-6 py-4">
      <nav class="mb-2 flex items-center gap-1.5 text-xs text-muted-foreground">
        <RouterLink :to="{ name: 'definitions' }" class="text-[color:var(--ma-accent-deep)] hover:underline">
          Workflow Definitions
        </RouterLink>
        <span aria-hidden="true">/</span>
        <span class="text-foreground">{{ tenant }}</span>
        <span aria-hidden="true">/</span>
        <span class="text-foreground">{{ name }}</span>
      </nav>

      <div class="flex items-center justify-between">
        <div>
          <h1 class="text-lg font-semibold">{{ tenant }}/{{ name }}</h1>
          <p v-if="selected" class="font-mono text-xs text-muted-foreground">{{ selected.buildId }}</p>
        </div>
        <RouterLink
          :to="{ name: 'definitions' }"
          class="rounded-md bg-primary px-3 py-1.5 text-sm text-primary-foreground hover:opacity-90"
        >
          Back to list
        </RouterLink>
      </div>
    </header>

    <div class="min-h-0 flex-1 overflow-y-auto p-6">
      <p v-if="store.isLoading" class="text-muted-foreground">Loading...</p>
      <p v-else-if="store.error" class="text-destructive">{{ store.error }}</p>
      <p v-else-if="!selected" class="text-muted-foreground">No such definition.</p>

      <template v-else>
        <dl class="mb-4 grid grid-cols-2 gap-2 text-sm">
          <dt class="text-muted-foreground">Status</dt>
          <dd>{{ selected.status }}</dd>
          <dt class="text-muted-foreground">Created</dt>
          <dd>{{ new Date(selected.createdAt).toLocaleString() }}</dd>
        </dl>

        <pre class="overflow-x-auto rounded-md bg-[color:var(--ma-ink)] p-4 text-xs text-white"><code>{{ selected.yaml }}</code></pre>
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
