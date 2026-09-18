<template>
  <div class="flex h-screen">
    <aside class="w-56 shrink-0 border-r border-neutral-200 bg-neutral-50 p-4">
      <h2 class="mb-4 px-2 text-sm font-semibold text-neutral-500">Zigflow Workflow Server</h2>
      <nav class="space-y-1">
        <RouterLink
          to="/definitions"
          class="block rounded-md px-2 py-1.5 text-sm hover:bg-neutral-200"
          active-class="bg-neutral-200 font-medium"
        >
          Workflow Definitions
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
