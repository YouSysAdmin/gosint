<script setup lang="ts">
// The language models the assistant and the template generator may
// use. Each person picks one, the default answers otherwise. THE API
// KEY IS NEVER READ BACK: an empty field on an edit keeps the stored
// one and the body omits it.
import { onMounted, ref, watch } from 'vue'
import { deleteModel, listModels, saveModel } from '@/api/admin'
import { apiErrorMessage } from '@/api/client'
import type { LlmModelAdmin, LlmModelInput } from '@/api/types'
import BaseModal from '@/components/BaseModal.vue'
import FormField from '@/components/FormField.vue'
import PageHeader from '@/components/PageHeader.vue'
import AdminNav from '@/views/admin/AdminNav.vue'
import { useConfirm } from '@/composables/useConfirm'
import { useNotificationStore } from '@/stores/notification'

const toast = useNotificationStore()
const { confirm } = useConfirm()
const models = ref<LlmModelAdmin[]>([])
const busy = ref('')

interface Form {
  name: string
  api: 'openai' | 'anthropic'
  base_url: string
  model: string
  tools: 'auto' | 'on' | 'off'
  key_name: string
  api_key: string
  clear_key: boolean
  default: boolean
}

// A new model starts as LM Studio on this machine, which needs no key.
const blank = (): Form => ({
  name: '',
  api: 'openai',
  base_url: 'http://localhost:1234/v1',
  model: '',
  tools: 'auto',
  key_name: 'LLM_API_KEY',
  api_key: '',
  clear_key: false,
  default: false,
})

// The dialog: open, the model edited (empty for a new one), its form.
const editing = ref(false)
const editId = ref('')
const hasKey = ref(false)
const form = ref<Form>(blank())

async function load() {
  try {
    models.value = await listModels()
  } catch (err) {
    toast.error(apiErrorMessage(err))
  }
}

function open(m: LlmModelAdmin | null) {
  editId.value = m?.id ?? ''
  hasKey.value = m?.has_api_key ?? false
  form.value = m
    ? {
        name: m.name,
        api: m.api,
        base_url: m.base_url,
        model: m.model,
        tools: m.tools,
        key_name: m.key_name,
        api_key: '',
        clear_key: false,
        default: m.default,
      }
    : blank()
  editing.value = true
}

// Anthropic is reached at its host, the OpenAI style at its /v1 root.
watch(
  () => form.value.api,
  (api) => {
    const f = form.value
    if (api === 'anthropic' && (f.base_url === '' || f.base_url === blank().base_url))
      f.base_url = 'https://api.anthropic.com'
    else if (api === 'openai' && f.base_url === 'https://api.anthropic.com')
      f.base_url = blank().base_url
  },
)

async function save() {
  const f = form.value
  const body: LlmModelInput = {
    name: f.name,
    api: f.api,
    base_url: f.base_url,
    model: f.model,
    tools: f.tools,
    key_name: f.key_name,
    default: f.default,
  }
  // Only a typed key travels, so an edit keeps the stored one.
  if (f.api_key) body.api_key = f.api_key
  else if (f.clear_key) body.api_key = ''
  busy.value = 'save'
  try {
    await saveModel(editId.value || null, body)
    editing.value = false
    toast.success('Model saved')
    await load()
  } catch (err) {
    toast.error(apiErrorMessage(err))
  } finally {
    busy.value = ''
  }
}

async function makeDefault(m: LlmModelAdmin) {
  busy.value = m.id
  try {
    await saveModel(m.id, { default: true })
    await load()
  } catch (err) {
    toast.error(apiErrorMessage(err))
  } finally {
    busy.value = ''
  }
}

async function remove(m: LlmModelAdmin) {
  const ok = await confirm({
    title: 'Delete model',
    message: `Delete ${m.name}? Anyone who picked it gets the default model.`,
    confirmText: 'Delete',
    variant: 'danger',
  })
  if (!ok) return
  try {
    await deleteModel(m.id)
    await load()
  } catch (err) {
    toast.error(apiErrorMessage(err))
  }
}

onMounted(load)
</script>

<template>
  <PageHeader title="Models">
    <button class="btn btn-primary" @click="open(null)">Add a model</button>
  </PageHeader>
  <AdminNav />

  <div class="card">
    <div class="card-body">
      <p v-if="!models.length" class="text-sm text-muted">
        No model yet. Add one and the assistant and the template generator can answer. Each person
        picks among them, the default answers otherwise.
      </p>
      <div v-for="m in models" :key="m.id" class="row">
        <div class="head">
          <div>
            <strong>{{ m.name }}</strong>
            <span v-if="m.default" class="badge badge-info ml-2">default</span>
            <span v-if="m.local" class="badge badge-neutral ml-2">local</span>
          </div>
          <div class="actions">
            <button
              v-if="!m.default"
              class="btn btn-sm btn-secondary"
              :disabled="busy === m.id"
              @click="makeDefault(m)"
            >
              Make default
            </button>
            <button class="btn btn-sm btn-secondary" @click="open(m)">Edit</button>
            <button class="btn btn-sm btn-danger" @click="remove(m)">Delete</button>
          </div>
        </div>
        <div class="text-xs text-muted mt-1">
          {{ m.api === 'anthropic' ? 'Anthropic' : 'OpenAI style' }} at
          <code>{{ m.base_url }}</code>
          <template v-if="m.model"
            >, <code>{{ m.model }}</code></template
          >
          <template v-else>, the model loaded on the server</template>
          <template v-if="!m.local">
            , each person's <code>{{ m.key_name }}</code>
            <template v-if="m.has_api_key">or the server's key</template>
          </template>
        </div>
      </div>
    </div>
  </div>

  <BaseModal
    v-if="editing"
    :title="editId ? 'Edit model' : 'Add a model'"
    size="modal-w560"
    form
    @close="editing = false"
    @submit="save"
  >
    <FormField label="Name" required hint="What people see in the picker">
      <input
        v-model="form.name"
        class="form-input"
        required
        maxlength="100"
        placeholder="GPT-4o mini"
      />
    </FormField>
    <div class="two">
      <FormField label="API">
        <select v-model="form.api" class="form-select">
          <option value="openai">
            OpenAI style (OpenAI, Mistral, OpenRouter, Ollama, LM Studio)
          </option>
          <option value="anthropic">Anthropic</option>
        </select>
      </FormField>
      <FormField label="Tools" hint="Auto offers them to a remote service only">
        <select v-model="form.tools" class="form-select">
          <option value="auto">Auto</option>
          <option value="on">On</option>
          <option value="off">Off</option>
        </select>
      </FormField>
    </div>
    <FormField
      label="Base URL"
      required
      hint="The API root, https://api.openai.com/v1 or the host alone for Anthropic"
    >
      <input v-model="form.base_url" class="form-input" required maxlength="400" />
    </FormField>
    <FormField label="Model" hint="Empty on a local server answers with the model it has loaded">
      <input v-model="form.model" class="form-input" maxlength="200" placeholder="gpt-4o-mini" />
    </FormField>
    <div class="two">
      <FormField label="Secret name" hint="Each person stores their key in Secrets under it">
        <input v-model="form.key_name" class="form-input code-font" required maxlength="128" />
      </FormField>
      <FormField
        label="Server key"
        :hint="
          hasKey
            ? 'Stored, leave empty to keep it'
            : 'Optional, used by anyone with no key of their own'
        "
      >
        <input v-model="form.api_key" class="form-input" type="password" autocomplete="off" />
      </FormField>
    </div>
    <label v-if="hasKey && !form.api_key" class="checkbox-label mb-2">
      <input v-model="form.clear_key" type="checkbox" /> Remove the stored server key
    </label>
    <label class="checkbox-label">
      <input v-model="form.default" type="checkbox" />
      The default model, for anyone who picked none
    </label>
    <template #footer>
      <button type="button" class="btn btn-secondary" @click="editing = false">Cancel</button>
      <button class="btn btn-primary" :disabled="busy === 'save' || !form.name.trim()">Save</button>
    </template>
  </BaseModal>
</template>

<style scoped>
.row {
  padding: 12px 0;
  border-bottom: 1px solid var(--border-primary);
}

.row:last-child {
  border-bottom: 0;
}

.head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
}

.actions {
  display: flex;
  gap: 6px;
}

.row code {
  overflow-wrap: anywhere;
}

.two {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 12px;
}

@media (max-width: 600px) {
  .two {
    grid-template-columns: 1fr;
  }
}
</style>
