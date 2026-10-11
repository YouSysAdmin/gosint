<script setup lang="ts">
// The worker's settings, the same on every node. No worker claims
// anything before the server is unsealed. A node applies a change on its
// next pass, the others within a minute.
import { onMounted, ref } from 'vue'
import { getWorker, saveWorker } from '@/api/admin'
import { apiErrorMessage } from '@/api/client'
import type { WorkerSettings } from '@/api/types'
import FormField from '@/components/FormField.vue'
import PageHeader from '@/components/PageHeader.vue'
import AdminNav from '@/views/admin/AdminNav.vue'
import { useNotificationStore } from '@/stores/notification'

const toast = useNotificationStore()
const form = ref<WorkerSettings | null>(null)
const defaultCacheDir = ref('')
const busy = ref(false)

async function load() {
  try {
    const r = await getWorker()
    form.value = r.worker
    defaultCacheDir.value = r.default_cache_dir
  } catch (err) {
    toast.error(apiErrorMessage(err))
  }
}

async function save() {
  if (!form.value) return
  busy.value = true
  try {
    form.value = await saveWorker({
      enabled: form.value.enabled,
      cache_dir: form.value.cache_dir.trim(),
      concurrency: Number(form.value.concurrency),
      poll_seconds: Number(form.value.poll_seconds),
      lease_seconds: Number(form.value.lease_seconds),
      max_attempts: Number(form.value.max_attempts),
    })
    toast.success('Worker settings saved')
  } catch (err) {
    toast.error(apiErrorMessage(err))
  } finally {
    busy.value = false
  }
}

onMounted(load)
</script>

<template>
  <PageHeader title="Worker" />
  <AdminNav />

  <form v-if="form" class="card" @submit.prevent="save">
    <div class="card-body">
      <p class="text-sm text-muted mb-3">
        How transforms and flows run, the same on every node. Nothing runs before the server is
        unsealed.
      </p>
      <label class="checkbox-label">
        <input v-model="form.enabled" type="checkbox" />
        Run transforms and flows
      </label>
      <p class="text-xs text-muted mb-3 hint">
        Off, launched scans wait in the queue until it is on again.
      </p>
      <FormField
        label="Cache directory"
        :hint="`Where transforms keep downloaded reference data (CDN ranges), an absolute path. Empty uses ${defaultCacheDir}`"
      >
        <input
          v-model="form.cache_dir"
          class="form-input code-font"
          maxlength="1024"
          :placeholder="defaultCacheDir"
        />
      </FormField>
      <div class="grid">
        <FormField label="Concurrency" hint="Scans running at once on each node, 1 to 64">
          <input
            v-model.number="form.concurrency"
            class="form-input"
            type="number"
            min="1"
            max="64"
          />
        </FormField>
        <FormField
          label="Queue check, seconds"
          hint="How often a node looks for scans queued on another one"
        >
          <input
            v-model.number="form.poll_seconds"
            class="form-input"
            type="number"
            min="1"
            max="300"
          />
        </FormField>
        <FormField
          label="Lease, seconds"
          hint="A scan whose node stopped renewing it runs again after this"
        >
          <input
            v-model.number="form.lease_seconds"
            class="form-input"
            type="number"
            min="10"
            max="3600"
          />
        </FormField>
        <FormField label="Attempts" hint="How often a scan whose node died is tried before failing">
          <input
            v-model.number="form.max_attempts"
            class="form-input"
            type="number"
            min="1"
            max="20"
          />
        </FormField>
      </div>
      <div class="flex justify-end">
        <button class="btn btn-primary" :disabled="busy">Save</button>
      </div>
    </div>
  </form>
</template>

<style scoped>
.hint {
  margin-left: 24px;
}

.grid {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 0 16px;
}

@media (max-width: 600px) {
  .grid {
    grid-template-columns: 1fr;
  }
}
</style>
