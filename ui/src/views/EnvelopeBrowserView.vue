<template>
  <div class="p-6">
    <h1 class="mb-6 text-2xl font-bold">Task Envelope Browser</h1>

    <form @submit.prevent="lookup" class="mb-6 flex max-w-xl gap-2">
      <input
        v-model="workflowIdInput"
        type="text"
        placeholder="Workflow execution ID, e.g. pod-memory-trend-1789797895"
        class="flex-1 rounded-md border border-border bg-background px-3 py-1.5 text-sm"
      />
      <button
        type="submit"
        :disabled="!workflowIdInput || store.isLoading"
        class="rounded-md bg-primary px-3 py-1.5 text-sm text-primary-foreground hover:opacity-90 disabled:opacity-50"
      >
        Look up
      </button>
    </form>

    <p v-if="store.isLoading" class="text-muted-foreground">Loading...</p>
    <p v-else-if="store.error" class="text-destructive">{{ store.error }}</p>

    <template v-else-if="store.index">
      <h2 class="mb-3 text-lg font-semibold">Slots for {{ lookedUpWorkflowId }}</h2>
      <table class="w-full max-w-4xl border-collapse text-left text-sm">
        <thead>
          <tr class="border-b border-border text-muted-foreground">
            <th class="py-2 pr-4 font-medium">Slot</th>
            <th class="py-2 pr-4 font-medium">Digest</th>
            <th class="py-2 pr-4 font-medium">Size</th>
            <th class="py-2 pr-4 font-medium">Media Type</th>
            <th class="py-2 pr-4 font-medium"></th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="[slot, ref] in Object.entries(store.index)" :key="slot" class="border-b border-border/60">
            <td class="py-2 pr-4 font-medium">{{ slot }}</td>
            <td class="py-2 pr-4 font-mono text-xs">{{ ref.Digest }}</td>
            <td class="py-2 pr-4">{{ ref.Size }} B</td>
            <td class="py-2 pr-4 text-xs text-muted-foreground">{{ ref.MediaType || '-' }}</td>
            <td class="py-2 pr-4">
              <button
                @click="inspect(ref.Digest)"
                :disabled="store.isLoadingBlob"
                class="rounded-md border border-border px-2 py-1 text-xs hover:bg-secondary disabled:opacity-50"
              >
                Inspect
              </button>
            </td>
          </tr>
        </tbody>
      </table>

      <div v-if="selectedDigest" class="mt-6">
        <h3 class="mb-2 text-sm font-semibold text-muted-foreground">Content of {{ selectedDigest }}</h3>
        <p v-if="store.isLoadingBlob" class="text-muted-foreground">Loading...</p>
        <pre
          v-else
          class="max-h-[32rem] overflow-auto rounded-md bg-[color:var(--ma-ink)] p-4 text-xs text-white"
        ><code>{{ store.blobContent }}</code></pre>
      </div>
    </template>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useEnvelopesStore } from '@/stores/envelopes'

const store = useEnvelopesStore()
const route = useRoute()
const router = useRouter()

const workflowIdInput = ref((route.params.workflowId as string) || '')
const lookedUpWorkflowId = ref('')
const selectedDigest = ref<string | null>(null)

function lookup() {
  selectedDigest.value = null
  lookedUpWorkflowId.value = workflowIdInput.value
  router.replace({ name: 'envelopes', params: { workflowId: workflowIdInput.value } })
  store.fetchEnvelope(workflowIdInput.value)
}

function inspect(digest: string) {
  selectedDigest.value = digest
  store.fetchBlob(lookedUpWorkflowId.value, digest)
}

onMounted(() => {
  if (workflowIdInput.value) {
    lookedUpWorkflowId.value = workflowIdInput.value
    store.fetchEnvelope(workflowIdInput.value)
  }
})
</script>
