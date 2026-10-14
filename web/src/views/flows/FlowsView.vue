<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { apiErrorMessage } from '@/api/client'
import { exportFlow, exportFlows, importFlows, listFlows } from '@/api/catalog'
import type { Flow } from '@/api/types'
import EmptyState from '@/components/EmptyState.vue'
import LoadingBlock from '@/components/LoadingBlock.vue'
import PageHeader from '@/components/PageHeader.vue'
import { saveBlob } from '@/composables/download'
import { timeAgo } from '@/composables/formatDate'
import { useNotificationStore } from '@/stores/notification'

const router = useRouter()
const toast = useNotificationStore()
const flows = ref<Flow[]>([])
const loading = ref(true)

async function load() {
  try {
    flows.value = await listFlows()
  } catch (err) {
    toast.error(apiErrorMessage(err))
  } finally {
    loading.value = false
  }
}

onMounted(load)

// openRow is the row as a mouse target, the title is the link. A click
// on a link or button inside is theirs.
function openRow(ev: MouseEvent, f: Flow) {
  if ((ev.target as HTMLElement).closest('a, button')) return
  void router.push(`/flows/${f.id}`)
}

// Flows move as files: one, or every flow of the account in one.
async function save(id?: string) {
  try {
    const d = await (id ? exportFlow(id) : exportFlows())
    saveBlob(d.blob, d.name)
  } catch (err) {
    toast.error(apiErrorMessage(err))
  }
}

async function pickFile(ev: Event) {
  const input = ev.target as HTMLInputElement
  const file = input.files?.[0]
  if (!file) return
  try {
    const r = await importFlows(file)
    toast.success(`${r.imported} flow${r.imported === 1 ? '' : 's'} imported`)
    for (const s of r.skipped) toast.error(s)
    await load()
  } catch (err) {
    toast.error(apiErrorMessage(err))
  } finally {
    input.value = ''
  }
}
</script>

<template>
  <PageHeader title="Flows">
    <label class="btn btn-secondary">
      Import
      <input type="file" accept=".json,application/json" hidden @change="pickFile" />
    </label>
    <button v-if="flows.length" class="btn btn-secondary" @click="save()">Export all</button>
    <router-link class="btn btn-primary" to="/flows/new">New flow</router-link>
  </PageHeader>
  <LoadingBlock v-if="loading" />
  <div v-else-if="!flows.length" class="card">
    <EmptyState
      title="No flow yet"
      text="A flow chains transforms, each running on what the step before it found."
    />
  </div>
  <div v-else class="card table-wrapper">
    <table class="table">
      <thead>
        <tr>
          <th>Name</th>
          <th>Starts from</th>
          <th>Steps</th>
          <th>Changed</th>
          <th></th>
        </tr>
      </thead>
      <tbody>
        <tr v-for="f in flows" :key="f.id" class="row-clickable" @click="openRow($event, f)">
          <td class="cell-title">
            <router-link class="row-link" :to="`/flows/${f.id}`">{{ f.name }}</router-link>
          </td>
          <td>{{ f.definition.input }}</td>
          <td>{{ f.definition.steps.length }}</td>
          <td class="cell-nowrap">{{ timeAgo(f.updated_at) }}</td>
          <td class="text-right">
            <button class="btn btn-sm btn-secondary" @click.stop="save(f.id)">Export</button>
          </td>
        </tr>
      </tbody>
    </table>
  </div>
</template>
