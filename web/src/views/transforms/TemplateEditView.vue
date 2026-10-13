<script setup lang="ts">
// A YAML transform: edited as text, saved, then tried on one value.
import { computed, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { apiErrorMessage } from '@/api/client'
import {
  deleteTemplate,
  getTemplate,
  saveTemplate,
  testTemplate,
  type TemplateTest,
} from '@/api/catalog'
import FormField from '@/components/FormField.vue'
import YamlEditor from '@/components/YamlEditor.vue'
import PageHeader from '@/components/PageHeader.vue'
import { useConfirm } from '@/composables/useConfirm'
import * as yaml from '@/composables/templateYaml'
import { generateTemplate } from '@/api/chat'
import { useLlmModels } from '@/composables/llmModels'
import { useAuthStore } from '@/stores/auth'
import { useCatalogStore } from '@/stores/catalog'
import { useNotificationStore } from '@/stores/notification'

const STARTER = `name: my-lookup
description: What this template finds
category: IP
version: 1
input:
  type: ip
  key: address
secrets:
  - name: MY_API_KEY
    required: true
request:
  method: GET
  url: "https://api.example.com/lookup/{{address}}"
  headers:
    Authorization: "Bearer {{secrets.MY_API_KEY}}"
response:
  expect: json
  map:
    city: city
    country: country
output:
  type: ip
`

const route = useRoute()
const router = useRouter()
const toast = useNotificationStore()
const auth = useAuthStore()
const catalog = useCatalogStore()
const { confirm } = useConfirm()

const id = computed(() => (route.params.id ? String(route.params.id) : null))
const source = ref(STARTER)
const isPublic = ref(false)
const owner = ref('')
const problem = ref('')
const sample = ref('')
const tested = ref<TemplateTest | null>(null)
const ask = ref('')
const { models, current: model, load: loadModels, pick } = useLlmModels()
// The input and output types, kept in step with the YAML both ways.
const ioInput = ref('')
const ioOutput = ref('')
const generating = ref(false)

// fieldsOf lists a type's field names, the primary one first.
function fieldsOf(type: string): string[] {
  const fields = catalog.type(type)?.fields ?? []
  return [...fields.filter((f) => f.primary), ...fields.filter((f) => !f.primary)].map(
    (f) => f.name,
  )
}

function keyOf(type: string): string {
  return catalog.type(type)?.fields.find((f) => f.primary)?.name ?? ''
}

// Typing in the editor moves the selects, a pick in a select rewrites
// the YAML block, and neither echoes back into the other.
watch(
  source,
  (src) => {
    const t = yaml.read(src)
    if (t.input !== ioInput.value) ioInput.value = t.input
    if (t.output !== ioOutput.value) ioOutput.value = t.output
  },
  { immediate: true },
)

watch(ioInput, (type) => {
  const now = yaml.read(source.value)
  if (!type || type === now.input) return
  const key = keyOf(type)
  source.value = yaml.renameKey(
    yaml.set(source.value, 'input', type, key, fieldsOf(type)),
    now.key,
    key,
  )
})

watch(ioOutput, (type) => {
  if (!type || type === yaml.read(source.value).output) return
  source.value = yaml.set(source.value, 'output', type, '', fieldsOf(type))
})

// generate asks the model for a draft and puts it in the editor, with
// what the parser still refuses shown above, to finish by hand.
async function generate() {
  if (!ask.value.trim()) return
  generating.value = true
  problem.value = ''
  try {
    const out = await generateTemplate(ask.value, ioInput.value, ioOutput.value, model.value?.id)
    source.value = out.source
    if (out.problem) problem.value = `The draft needs a fix: ${out.problem}`
    else toast.success('Draft ready, check it and save')
  } catch (err) {
    problem.value = apiErrorMessage(err)
  } finally {
    generating.value = false
  }
}

const mine = computed(() => !id.value || owner.value === auth.user?.id)

async function save() {
  problem.value = ''
  try {
    const t = await saveTemplate(id.value, source.value, isPublic.value)
    toast.success(`${t.name} saved`)
    if (!id.value) await router.replace(`/templates/${t.id}`)
  } catch (err) {
    problem.value = apiErrorMessage(err)
  }
}

async function test() {
  if (!id.value) return
  try {
    tested.value = await testTemplate(id.value, sample.value)
  } catch (err) {
    toast.error(apiErrorMessage(err))
  }
}

async function remove() {
  if (
    !id.value ||
    !(await confirm({
      title: 'Delete transform',
      message: 'Delete this transform template?',
      variant: 'danger',
    }))
  )
    return
  try {
    await deleteTemplate(id.value)
    await router.push('/transforms')
  } catch (err) {
    toast.error(apiErrorMessage(err))
  }
}

onMounted(async () => {
  await catalog.load().catch(() => {})
  await loadModels().catch(() => {})
  if (!id.value) {
    // A new template starts with its types' fields listed.
    const t = yaml.read(source.value)
    source.value = yaml.set(
      yaml.set(source.value, 'input', t.input, keyOf(t.input), fieldsOf(t.input)),
      'output',
      t.output,
      '',
      fieldsOf(t.output),
    )
    return
  }
  try {
    const t = await getTemplate(id.value)
    source.value = t.source
    isPublic.value = t.is_public
    owner.value = t.owner_id
  } catch (err) {
    toast.error(apiErrorMessage(err))
  }
})
</script>

<template>
  <PageHeader :title="id ? 'Transform template' : 'New transform'">
    <button v-if="id && mine" class="btn btn-danger" @click="remove">Delete</button>
    <button v-if="mine" class="btn btn-primary" @click="save">Save</button>
  </PageHeader>

  <div v-if="problem" class="notice notice-danger mb-4">{{ problem }}</div>

  <div v-if="mine" class="card mb-4">
    <div class="card-header"><h3>Draft it with the model</h3></div>
    <div class="card-body">
      <div class="form-row">
        <FormField label="What should it do" class="flex-1">
          <input
            v-model="ask"
            class="form-input"
            placeholder="Look an IP up on ip-api.com and keep its city, country and ISP"
            @keydown.enter.prevent="generate"
          />
        </FormField>
        <FormField v-if="models.length > 1" label="Model">
          <select
            class="form-select"
            :value="model?.id"
            @change="pick(($event.target as HTMLSelectElement).value)"
          >
            <option v-for="m in models" :key="m.id" :value="m.id">{{ m.name }}</option>
          </select>
        </FormField>
        <button
          class="btn btn-secondary self-end"
          :disabled="generating || !ask.trim()"
          @click="generate"
        >
          {{ generating ? 'Drafting...' : 'Draft' }}
        </button>
      </div>
      <p class="text-sm text-muted">
        The draft replaces the editor below and follows its Input and Output. Read it before saving,
        a model guesses field names.
      </p>
    </div>
  </div>

  <div class="split">
    <div class="card">
      <div class="card-header io">
        <FormField label="Input">
          <select v-model="ioInput" class="form-select" :disabled="!mine">
            <option value="" disabled>Pick a type</option>
            <option v-for="t in catalog.types" :key="t.name" :value="t.name">{{ t.title }}</option>
          </select>
        </FormField>
        <span class="arrow" aria-hidden="true">&rarr;</span>
        <FormField label="Output">
          <select v-model="ioOutput" class="form-select" :disabled="!mine">
            <option value="" disabled>Pick a type</option>
            <option v-for="t in catalog.types" :key="t.name" :value="t.name">{{ t.title }}</option>
          </select>
        </FormField>
        <p class="text-xs text-muted hint">
          A pick rewrites the YAML's input and output, with the type's fields listed in a comment.
        </p>
      </div>
      <div class="card-body">
        <YamlEditor v-model="source" :readonly="!mine" />
        <label v-if="mine" class="checkbox-label mt-3">
          <input v-model="isPublic" type="checkbox" /> Share with every account (one reading secrets
          runs for you only)
        </label>
      </div>
    </div>
    <div class="card">
      <div class="card-header"><h3>Try it</h3></div>
      <div class="card-body">
        <p v-if="!id" class="text-muted">Save the template first.</p>
        <form v-else class="flex gap-2 mb-3" @submit.prevent="test">
          <FormField label="Input value" class="flex-1">
            <input v-model="sample" class="form-input" required />
          </FormField>
          <button class="btn btn-secondary">Run</button>
        </form>
        <template v-if="tested">
          <p v-if="tested.error" class="text-danger">{{ tested.error }}</p>
          <div v-for="(o, i) in tested.outputs" :key="i" class="mb-2">
            <div class="fw-medium">{{ o.label || '(no label)' }}</div>
            <div v-if="o.error" class="text-danger text-sm">{{ o.error }}</div>
            <pre class="code-block">{{ JSON.stringify(o.properties, null, 2) }}</pre>
          </div>
          <details v-if="tested.answer">
            <summary class="text-sm">Raw answer, status {{ tested.answer.status_code }}</summary>
            <pre class="code-block">{{ tested.answer.body.slice(0, 4000) }}</pre>
          </details>
        </template>
      </div>
    </div>
  </div>
</template>

<style scoped>
.io {
  display: flex;
  justify-content: flex-start;
  align-items: flex-end;
  flex-wrap: wrap;
  gap: 10px;
}

.io > * {
  flex: 0 0 auto;
  width: auto;
  margin-bottom: 0;
}

.io .form-select {
  min-width: 180px;
}

.io .arrow {
  padding-bottom: 8px;
  color: var(--text-muted);
}

.io .hint {
  flex-basis: 100%;
  margin: 0;
}

.self-end {
  align-self: flex-end;
  margin-bottom: 16px;
}

.split {
  display: grid;
  grid-template-columns: 1.3fr 1fr;
  gap: 16px;
  align-items: start;
}

.split > .card + .card {
  margin-top: 0;
}

/* The editor card fills the height left under the page header, the
   editor growing inside it. */
.split .card:first-child {
  display: flex;
  flex-direction: column;
  min-height: calc(100vh - var(--topbar) - 140px);
}

.split .card:first-child .card-body {
  flex: 1;
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.split .card:first-child .card-body > :first-child {
  flex: 1;
}
</style>
