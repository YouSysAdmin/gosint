<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { apiErrorMessage } from '@/api/client'
import { deleteKey, listModels, listTransforms, listKeys, putKey } from '@/api/catalog'
import type { LlmModel, Transform, VaultKey } from '@/api/types'
import EmptyState from '@/components/EmptyState.vue'
import FormField from '@/components/FormField.vue'
import LoadingBlock from '@/components/LoadingBlock.vue'
import PageHeader from '@/components/PageHeader.vue'
import { timeAgo } from '@/composables/formatDate'
import { useConfirm } from '@/composables/useConfirm'
import { useNotificationStore } from '@/stores/notification'

const toast = useNotificationStore()
const { confirm } = useConfirm()
const keys = ref<VaultKey[]>([])
const transforms = ref<Transform[]>([])
const models = ref<LlmModel[]>([])
const name = ref('')
// The first load shows the spinner, or a retry when it failed. A
// refresh after a change keeps the page in place.
const loaded = ref(false)
const loadError = ref('')

// usage maps a key name to the transforms asking for it.
const usage = computed(() => {
  const m = new Map<string, string[]>()
  for (const e of transforms.value)
    for (const p of e.params ?? [])
      if (p.type === 'vaultSecret') m.set(p.name, [...(m.get(p.name) ?? []), e.name])

  for (const l of models.value)
    if (l.needs_key) m.set(l.key_name, [...(m.get(l.key_name) ?? []), `the assistant (${l.name})`])

  return m
})

// wanted are the secret names transforms ask for and none is stored under.
const wanted = computed(() => {
  const have = new Set(keys.value.map((k) => k.name))

  return [...usage.value.keys()].filter((k) => !have.has(k)).sort()
})
const value = ref('')
const valueInput = ref<HTMLInputElement | null>(null)

// pick fills the form with a name a transform asks for, ready for its value.
function pick(k: string) {
  name.value = k
  valueInput.value?.focus()
}

async function loadKeys() {
  keys.value = await listKeys()
}

async function load() {
  try {
    const [k, t, m] = await Promise.all([listKeys(), listTransforms(), listModels()])
    keys.value = k
    transforms.value = t
    models.value = m
    loaded.value = true
    loadError.value = ''
  } catch (err) {
    toast.error(apiErrorMessage(err))
    if (!loaded.value) loadError.value = apiErrorMessage(err)
  }
}

async function save() {
  try {
    await putKey(name.value, value.value)
    toast.success(`${name.value} stored`)
    name.value = ''
    value.value = ''
    await loadKeys()
  } catch (err) {
    toast.error(apiErrorMessage(err))
  }
}

async function remove(k: VaultKey) {
  if (!(await confirm({ title: 'Delete secret', message: `Delete ${k.name}?`, variant: 'danger' })))
    return
  try {
    await deleteKey(k.id)
    await loadKeys()
  } catch (err) {
    toast.error(apiErrorMessage(err))
  }
}

onMounted(load)
</script>

<template>
  <PageHeader title="Secrets" />
  <LoadingBlock v-if="!loaded && !loadError" />
  <div v-else-if="!loaded" class="card">
    <EmptyState title="The secrets could not be loaded">
      <p>{{ loadError }}</p>
      <button class="btn btn-secondary" @click="load">Retry</button>
    </EmptyState>
  </div>
  <template v-else>
    <p class="text-secondary mb-4">
      API keys and tokens your transforms, templates and the assistant use, such as WHOXY_API_KEY. A
      transform finds the one named like its parameter, a template reads any by name. They are
      encrypted, used for your runs only, and never shown again.
    </p>
    <form class="card mb-4" @submit.prevent="save">
      <div class="card-body form-row items-end">
        <FormField label="Name" required>
          <input
            v-model="name"
            class="form-input"
            required
            pattern="[A-Za-z0-9][A-Za-z0-9_.\-]*"
            placeholder="WHOXY_API_KEY"
          />
        </FormField>
        <FormField label="Value" required>
          <input
            ref="valueInput"
            v-model="value"
            class="form-input"
            type="password"
            autocomplete="off"
            required
          />
        </FormField>
        <button class="btn btn-primary">Store</button>
      </div>
    </form>
    <div v-if="wanted.length" class="card mb-4">
      <div class="card-header">
        <h3>Asked for, not stored yet</h3>
      </div>
      <div class="table-wrapper">
        <table class="table">
          <tbody>
            <tr v-for="k in wanted" :key="k">
              <td class="code-font">{{ k }}</td>
              <td class="text-muted text-sm">{{ usage.get(k)?.join(', ') }}</td>
              <td class="text-right">
                <button class="btn btn-sm btn-secondary" @click="pick(k)">Add</button>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>
    <div class="card">
      <EmptyState v-if="!keys.length" title="No secret stored yet" />
      <div v-else class="table-wrapper">
        <table class="table">
          <tbody>
            <tr v-for="k in keys" :key="k.id">
              <td class="code-font">{{ k.name }}</td>
              <td class="text-muted text-sm">
                {{ usage.get(k.name)?.join(', ') || 'no built-in transform asks for it' }}
              </td>
              <td class="text-muted cell-nowrap">changed {{ timeAgo(k.updated_at) }}</td>
              <td class="text-right">
                <button class="btn btn-sm btn-secondary" @click="remove(k)">Delete</button>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>
  </template>
</template>
