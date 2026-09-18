<template>
  <div class="flex h-screen bg-background text-foreground">
    <aside class="w-56 shrink-0 border-r border-sidebar-border bg-sidebar p-4">
      <h2 class="mb-4 px-2 text-sm font-semibold text-muted-foreground">Zigflow Workflow Server</h2>
      <nav class="space-y-1">
        <RouterLink
          to="/definitions"
          class="block rounded-md border-l-[3px] border-transparent px-2 py-1.5 text-sm text-muted-foreground hover:bg-secondary"
          active-class="border-accent bg-sidebar-accent font-medium text-sidebar-accent-foreground hover:bg-sidebar-accent"
        >
          Workflow Definitions
        </RouterLink>
        <RouterLink
          to="/agents"
          class="block rounded-md border-l-[3px] border-transparent px-2 py-1.5 text-sm text-muted-foreground hover:bg-secondary"
          active-class="border-accent bg-sidebar-accent font-medium text-sidebar-accent-foreground hover:bg-sidebar-accent"
        >
          Agents
        </RouterLink>
      </nav>
    </aside>

    <main class="min-w-0 flex-1 overflow-y-auto">
      <RouterView />
    </main>
  </div>
</template>

<script setup lang="ts">
import { RouterLink, RouterView } from 'vue-router'
</script>
