<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { apiErrorMessage } from '@/api/client'
import { exportTemplate, exportTemplates, importTemplates, listTransforms } from '@/api/catalog'
import type { Transform } from '@/api/types'
import LoadingBlock from '@/components/LoadingBlock.vue'
import PageHeader from '@/components/PageHeader.vue'
import { saveBlob } from '@/composables/download'
import { useNotificationStore } from '@/stores/notification'

const router = useRouter()
const toast = useNotificationStore()
const transforms = ref<Transform[]>([])
const loading = ref(true)
const filter = ref('')

const shown = computed(() => {
  const q = filter.value.toLowerCase()
  return transforms.value.filter(
    (e) => !q || `${e.name} ${e.description} ${e.input} ${e.output}`.toLowerCase().includes(q),
  )
})

async function load() {
  try {
    transforms.value = await listTransforms()
  } catch (err) {
    toast.error(apiErrorMessage(err))
  } finally {
    loading.value = false
  }
}

onMounted(load)

// Templates move as YAML files: one, or every template of the account
// in one.
async function save(id?: string) {
  try {
    const d = await (id ? exportTemplate(id) : exportTemplates())
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
    const r = await importTemplates(file)
    toast.success(`${r.imported} template${r.imported === 1 ? '' : 's'} imported`)
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
  <PageHeader title="Transforms">
    <label class="btn btn-secondary">
      Import templates
      <input type="file" accept=".yaml,.yml" hidden @change="pickFile" />
    </label>
    <button v-if="transforms.some((e) => e.template_id)" class="btn btn-secondary" @click="save()">
      Export my templates
    </button>
    <router-link class="btn btn-primary" to="/templates/new">New transform</router-link>
  </PageHeader>
  <input
    v-model="filter"
    class="form-input mb-4 w-search"
    placeholder="Filter by name, type or description"
  />
  <LoadingBlock v-if="loading" />
  <div v-else class="card table-wrapper">
    <table class="table">
      <thead>
        <tr>
          <th>Name</th>
          <th>From</th>
          <th>To</th>
          <th>Keys</th>
          <th>Description</th>
          <th></th>
        </tr>
      </thead>
      <tbody>
        <tr
          v-for="e in shown"
          :key="e.name + e.template_id"
          :class="{ 'row-clickable': e.template_id }"
          @click="e.template_id && router.push(`/templates/${e.template_id}`)"
        >
          <td class="cell-title">
            {{ e.name }} <span v-if="e.template_id" class="badge badge-secondary">template</span>
            <span v-if="e.file" class="badge badge-secondary" :title="e.file">file</span>
          </td>
          <td>{{ e.input || 'any' }}</td>
          <td>{{ e.output }}</td>
          <td class="code-font text-xs">
            {{
              (e.params ?? [])
                .filter((p) => p.type === 'vaultSecret')
                .map((p) => p.name)
                .join(', ')
            }}
          </td>
          <td class="text-muted">{{ e.description }}</td>
          <td class="text-right">
            <button
              v-if="e.template_id"
              class="btn btn-sm btn-secondary"
              @click.stop="save(e.template_id)"
            >
              Export
            </button>
          </td>
        </tr>
      </tbody>
    </table>
  </div>
</template>
