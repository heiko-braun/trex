<template>
  <div class="p-6">
    <div class="mb-6 flex items-center justify-between">
      <h1 class="text-2xl font-bold">Workflow Definitions</h1>
      <button
        @click="store.fetchAll()"
        class="rounded-md border border-border px-3 py-1.5 text-sm hover:bg-secondary"
      >
        Refresh
      </button>
    </div>

    <p v-if="store.isLoading" class="text-muted-foreground">Loading...</p>
    <p v-else-if="store.error" class="text-destructive">{{ store.error }}</p>
    <p v-else-if="store.definitions.length === 0" class="text-muted-foreground">No definitions published yet.</p>

    <table v-else class="w-full max-w-4xl border-collapse text-left text-sm">
      <thead>
        <tr class="border-b border-border text-muted-foreground">
          <th class="py-2 pr-4 font-medium"></th>
          <th class="py-2 pr-4 font-medium">Tenant</th>
          <th class="py-2 pr-4 font-medium">Name</th>
          <th class="py-2 pr-4 font-medium">Build ID</th>
          <th class="py-2 pr-4 font-medium">Status</th>
          <th class="py-2 pr-4 font-medium">Created</th>
        </tr>
      </thead>
      <tbody v-for="group in groups" :key="`${group.latest.tenant}/${group.latest.name}`">
        <tr
          class="cursor-pointer border-b border-border/60 hover:bg-secondary"
          @click="openDetail(group.latest)"
        >
          <td class="py-2 pr-4">
            <button
              v-if="group.older.length > 0"
              @click.stop="toggle(group.latest)"
              class="text-muted-foreground hover:text-foreground"
              :aria-label="isExpanded(group.latest) ? 'Collapse older versions' : 'Show older versions'"
            >
              {{ isExpanded(group.latest) ? '▾' : '▸' }}
            </button>
          </td>
          <td class="py-2 pr-4">{{ group.latest.tenant }}</td>
          <td class="py-2 pr-4">{{ group.latest.name }}</td>
          <td class="py-2 pr-4 font-mono text-xs">{{ group.latest.buildId }}</td>
          <td class="py-2 pr-4">{{ group.latest.status }}</td>
          <td class="py-2 pr-4 text-muted-foreground">{{ new Date(group.latest.createdAt).toLocaleString() }}</td>
        </tr>
        <template v-if="isExpanded(group.latest)">
          <tr
            v-for="def in group.older"
            :key="`${def.tenant}/${def.name}/${def.buildId}`"
            class="cursor-pointer border-b border-border/60 bg-secondary/30 text-muted-foreground hover:bg-secondary"
            @click="openDetail(def)"
          >
            <td class="py-2 pr-4"></td>
            <td class="py-2 pr-4 pl-4">{{ def.tenant }}</td>
            <td class="py-2 pr-4 pl-4">{{ def.name }}</td>
            <td class="py-2 pr-4 font-mono text-xs">{{ def.buildId }}</td>
            <td class="py-2 pr-4">{{ def.status }}</td>
            <td class="py-2 pr-4">{{ new Date(def.createdAt).toLocaleString() }}</td>
          </tr>
        </template>
      </tbody>
    </table>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { useDefinitionsStore } from '@/stores/definitions'
import type { Definition } from '@/types'

const router = useRouter()
const store = useDefinitionsStore()

const expanded = ref(new Set<string>())

// store.definitions is every revision, newest first (see store.List).
// Group by (tenant, name): the first entry seen per key is the latest.
const groups = computed(() => {
  const byKey = new Map<string, { latest: Definition; older: Definition[] }>()
  for (const def of store.definitions) {
    const key = `${def.tenant}/${def.name}`
    const group = byKey.get(key)
    if (!group) {
      byKey.set(key, { latest: def, older: [] })
    } else {
      group.older.push(def)
    }
  }
  return Array.from(byKey.values())
})

function groupKey(def: Definition) {
  return `${def.tenant}/${def.name}`
}

function isExpanded(def: Definition) {
  return expanded.value.has(groupKey(def))
}

function toggle(def: Definition) {
  const key = groupKey(def)
  if (expanded.value.has(key)) {
    expanded.value.delete(key)
  } else {
    expanded.value.add(key)
  }
}

function openDetail(def: Definition) {
  router.push({ name: 'definition-detail', params: { tenant: def.tenant, name: def.name } })
}

onMounted(() => {
  store.fetchAll()
})
</script>
