<script setup lang="ts">
// Where the bytes of attached files live and how large they may be.
// The store is checked before it is saved. Changing it leaves the files
// already stored where they are, moving them is the administrator's.
// THE S3 SECRET KEY IS NEVER READ BACK.
import { computed, onMounted, ref } from 'vue'
import { getFileStore, saveFileStore } from '@/api/admin'
import { apiErrorMessage } from '@/api/client'
import type { FileStore, FileStoreInput } from '@/api/types'
import EmptyState from '@/components/EmptyState.vue'
import FormField from '@/components/FormField.vue'
import LoadingBlock from '@/components/LoadingBlock.vue'
import PageHeader from '@/components/PageHeader.vue'
import AdminNav from '@/views/admin/AdminNav.vue'
import { useConfirm } from '@/composables/useConfirm'
import { useNotificationStore } from '@/stores/notification'

const toast = useNotificationStore()
const { confirm } = useConfirm()
const saved = ref<FileStore | null>(null)
const form = ref<FileStoreInput & { secret_key: string }>({
  backend: 'fs',
  max_size_mb: 32,
  retain_hours: 168,
  dir: '',
  secret_key: '',
  s3: {
    endpoint: '',
    region: 'us-east-1',
    bucket: '',
    prefix: '',
    access_key: '',
    path_style: false,
  },
})
const busy = ref(false)
// The first load shows the spinner, or a retry when it failed, and the
// form only once the settings are in it.
const loaded = ref(false)
const loadError = ref('')

function fill(f: FileStore) {
  saved.value = f
  form.value = {
    backend: f.backend,
    max_size_mb: f.max_size_mb,
    retain_hours: f.retain_hours,
    dir: f.dir,
    secret_key: '',
    s3: {
      endpoint: f.s3.endpoint,
      region: f.s3.region,
      bucket: f.s3.bucket,
      prefix: f.s3.prefix,
      access_key: f.s3.access_key,
      path_style: f.s3.path_style,
    },
  }
}

// The files already stored stay behind when the store moves.
const moved = computed(() => {
  const s = saved.value
  const f = form.value
  if (!s) return false
  if (s.backend !== f.backend) return true
  if (f.backend === 'fs') return s.dir !== f.dir.trim()
  if (f.backend === 's3')
    return (
      s.s3.endpoint !== f.s3.endpoint.trim() ||
      s.s3.bucket !== f.s3.bucket.trim() ||
      s.s3.prefix !== f.s3.prefix.trim()
    )
  return false
})

async function load() {
  try {
    fill(await getFileStore())
    loaded.value = true
    loadError.value = ''
  } catch (err) {
    toast.error(apiErrorMessage(err))
    if (!loaded.value) loadError.value = apiErrorMessage(err)
  }
}

async function save() {
  if (moved.value) {
    const ok = await confirm({
      title: 'Change the file store',
      message:
        'Files already attached stay in the old store and cannot be opened from here until you copy them into the new one yourself.',
      confirmText: 'Change',
      variant: 'danger',
    })
    if (!ok) return
  }

  const f = form.value
  const body: FileStoreInput = {
    backend: f.backend,
    max_size_mb: Number(f.max_size_mb),
    retain_hours: Number(f.retain_hours),
    dir: f.dir.trim(),
    s3: { ...f.s3 },
  }
  // Only a typed key travels, so a save keeps the stored one.
  if (f.secret_key) body.s3.secret_key = f.secret_key
  busy.value = true
  try {
    fill(await saveFileStore(body))
    toast.success('File store saved')
  } catch (err) {
    toast.error(apiErrorMessage(err))
  } finally {
    busy.value = false
  }
}

onMounted(load)
</script>

<template>
  <PageHeader title="Files" />
  <AdminNav />

  <LoadingBlock v-if="!loaded && !loadError" />
  <div v-else-if="!loaded" class="card">
    <EmptyState title="The file store settings could not be loaded">
      <p>{{ loadError }}</p>
      <button class="btn btn-secondary" @click="load">Retry</button>
    </EmptyState>
  </div>
  <form v-else class="card" @submit.prevent="save">
    <div class="card-body">
      <p class="text-sm text-muted mb-3">
        Files attached to entities are sealed before they are stored, wherever that is. The store is
        checked before it is saved.
      </p>
      <div class="grid">
        <FormField label="Largest file, MB">
          <input
            v-model.number="form.max_size_mb"
            class="form-input"
            type="number"
            min="1"
            max="4096"
          />
        </FormField>
        <FormField label="Keep deleted files, hours" hint="So an undo brings them back">
          <input
            v-model.number="form.retain_hours"
            class="form-input"
            type="number"
            min="1"
            max="87600"
          />
        </FormField>
      </div>
      <FormField label="Store">
        <select v-model="form.backend" class="form-select">
          <option value="fs">A directory on the server</option>
          <option value="s3">An S3-compatible bucket</option>
          <option value="db">The database</option>
        </select>
      </FormField>

      <FormField
        v-if="form.backend === 'fs'"
        label="Directory"
        :hint="
          saved?.default_dir
            ? `An absolute path, empty uses ${saved.default_dir}`
            : 'An absolute path every node shares'
        "
      >
        <input
          v-model="form.dir"
          class="form-input code-font"
          maxlength="1024"
          :placeholder="saved?.default_dir"
        />
      </FormField>

      <template v-else-if="form.backend === 's3'">
        <div class="grid">
          <FormField label="Endpoint" hint="Empty for AWS">
            <input
              v-model="form.s3.endpoint"
              class="form-input"
              maxlength="400"
              placeholder="https://s3.example.com"
            />
          </FormField>
          <FormField label="Region">
            <input v-model="form.s3.region" class="form-input" maxlength="60" />
          </FormField>
          <FormField label="Bucket" required>
            <input v-model="form.s3.bucket" class="form-input" maxlength="255" />
          </FormField>
          <FormField label="Prefix" hint="Put in front of every key">
            <input v-model="form.s3.prefix" class="form-input" maxlength="255" />
          </FormField>
          <FormField label="Access key" hint="Empty uses the server's own credentials">
            <input
              v-model="form.s3.access_key"
              class="form-input"
              maxlength="255"
              autocomplete="off"
            />
          </FormField>
          <FormField
            label="Secret key"
            :hint="saved?.s3.has_secret_key ? 'Stored, leave empty to keep it' : ''"
          >
            <input
              v-model="form.secret_key"
              class="form-input"
              type="password"
              autocomplete="new-password"
            />
          </FormField>
        </div>
        <label class="checkbox-label mb-3">
          <input v-model="form.s3.path_style" type="checkbox" />
          Path-style addressing (MinIO and most self-hosted stores)
        </label>
      </template>

      <p v-else class="text-sm text-muted mb-3">
        The bytes go into the database's blobs table, for a postgres fleet with no bucket to share.
      </p>

      <p v-if="moved" class="notice notice-warning text-sm mb-3">
        The files already attached stay in the current store. Copy them into the new one yourself,
        or they cannot be opened.
      </p>
      <div class="flex justify-end">
        <button class="btn btn-primary" :disabled="busy">
          {{ busy ? 'Checking...' : 'Save' }}
        </button>
      </div>
    </div>
  </form>
</template>

<style scoped>
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
