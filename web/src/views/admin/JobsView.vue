<script setup lang="ts">
// The maintenance jobs of this node: what each sweeps, when it last
// ran and when it runs next, with a button to run one now. Every node
// of a team install runs every job, this page shows the node answering.
import { onMounted, ref } from 'vue'
import { listJobs, runJob } from '@/api/admin'
import { apiErrorMessage } from '@/api/client'
import { seal } from '@/api/sys'
import { useConfirm } from '@/composables/useConfirm'
import type { Job } from '@/api/types'
import PageHeader from '@/components/PageHeader.vue'
import AdminNav from '@/views/admin/AdminNav.vue'
import { formatDate } from '@/composables/formatDate'
import { useNotificationStore } from '@/stores/notification'

const toast = useNotificationStore()
const { confirm } = useConfirm()
const jobs = ref<Job[]>([])
const loading = ref(true)
const running = ref('')

const what: Record<string, string> = {
  'blob-release': 'Deletes the bytes of files gone for longer than files.retain_deleted',
  'undo-journal': 'Drops edits older than files.retain_deleted from the undo history',
  sessions: 'Removes expired sign-in sessions',
  'password-resets': 'Removes spent and expired password reset links',
  'custom-types': 'Reads the entity types declared on another node',
  settings: 'Reads the mail relay saved on another node',
}

async function load() {
  try {
    jobs.value = (await listJobs()).jobs
  } catch (err) {
    toast.error(apiErrorMessage(err))
  } finally {
    loading.value = false
  }
}

async function run(j: Job) {
  running.value = j.name
  try {
    jobs.value = (await runJob(j.name)).jobs
    toast.success(`${j.name} ran`)
  } catch (err) {
    toast.error(apiErrorMessage(err))
    await load()
  } finally {
    running.value = ''
  }
}

// Forgets the key on this node: everything stops until someone enters
// the unseal key again.
async function sealNow() {
  const ok = await confirm({
    title: 'Seal this server',
    message:
      'The key is forgotten on this node and nothing opens until the unseal key is entered again. Running scans fail.',
    confirmText: 'Seal',
    variant: 'danger',
  })
  if (!ok) return

  try {
    await seal()
    location.assign(`${import.meta.env.BASE_URL}unseal?next=${encodeURIComponent('/admin/jobs')}`)
  } catch (err) {
    toast.error(apiErrorMessage(err))
  }
}

onMounted(load)
</script>

<template>
  <PageHeader title="Maintenance">
    <button class="btn btn-danger" @click="sealNow">Seal this server</button>
  </PageHeader>
  <AdminNav />

  <div class="card">
    <div class="card-body">
      <p v-if="loading" class="text-sm text-muted">Loading...</p>
      <table v-else class="table">
        <thead>
          <tr>
            <th>Job</th>
            <th>Schedule</th>
            <th>Last run</th>
            <th>Next run</th>
            <th></th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="j in jobs" :key="j.name">
            <td>
              <div class="fw-medium">{{ j.name }}</div>
              <div class="text-xs text-muted">{{ what[j.name] ?? '' }}</div>
            </td>
            <td class="code-font text-sm">{{ j.schedule }}</td>
            <td class="text-sm">
              {{ formatDate(j.last_run_at, 'Not yet') }}
              <span v-if="j.last_run_at" class="text-muted">, {{ j.last_duration_ms }} ms</span>
              <div v-if="j.last_error" class="text-xs text-danger">{{ j.last_error }}</div>
            </td>
            <td class="text-sm">{{ formatDate(j.next_run_at) }}</td>
            <td class="text-right">
              <button
                class="btn btn-sm btn-secondary"
                :disabled="j.running || running === j.name"
                @click="run(j)"
              >
                {{ j.running || running === j.name ? 'Running...' : 'Run now' }}
              </button>
            </td>
          </tr>
        </tbody>
      </table>
    </div>
  </div>
</template>
