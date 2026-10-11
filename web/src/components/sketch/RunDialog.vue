<script setup lang="ts">
// Runs a transform, a template or a flow on the selected nodes.
import { computed, onMounted, ref } from 'vue'
import { apiErrorMessage } from '@/api/client'
import { listTransforms, listFlows, listKeys } from '@/api/catalog'
import { launchTransform, launchFlow, launchTemplate } from '@/api/sketches'
import type { Transform, Flow, GraphNode } from '@/api/types'
import BaseModal from '@/components/BaseModal.vue'
import FormField from '@/components/FormField.vue'
import LoadingBlock from '@/components/LoadingBlock.vue'
import { useNotificationStore } from '@/stores/notification'

const props = defineProps<{ sketchId: string; nodes: GraphNode[]; preselect?: string }>()
const emit = defineEmits<{ (e: 'close'): void; (e: 'launched'): void }>()

const toast = useNotificationStore()
const type = computed(() => props.nodes[0]?.type ?? '')
const transforms = ref<Transform[]>([])
const flows = ref<Flow[]>([])
const stored = ref(new Set<string>())
const loading = ref(true)
const picked = ref<Transform | null>(null)
const params = ref<Record<string, string>>({})
const filter = ref('')

const shown = computed(() =>
  transforms.value.filter(
    (e) =>
      !filter.value || (e.name + e.description).toLowerCase().includes(filter.value.toLowerCase()),
  ),
)

onMounted(async () => {
  try {
    const [e, f, k] = await Promise.all([
      listTransforms(type.value),
      listFlows(type.value),
      listKeys().catch(() => []),
    ])
    transforms.value = e
    flows.value = f
    stored.value = new Set(k.map((x) => x.name))
    if (props.preselect) {
      const hit = e.find((x) => x.name === props.preselect || x.template_id === props.preselect)
      if (hit) choose(hit)
    }
  } catch (err) {
    toast.error(apiErrorMessage(err))
  } finally {
    loading.value = false
  }
})

// missing are the required secrets a transform wants that are not stored.
function missing(e: Transform): string[] {
  return (e.params ?? [])
    .filter((p) => p.type === 'vaultSecret' && p.required && !stored.value.has(p.name))
    .map((p) => p.name)
}

function choose(e: Transform) {
  picked.value = e
  params.value = {}
  for (const p of e.params ?? [])
    if (p.type !== 'vaultSecret') params.value[p.name] = p.default ?? ''
  // Nothing to fill runs at once, unless a secret is missing: the form
  // then shows the notice instead of a launch the server refuses.
  if (!(e.params ?? []).some((p) => p.type !== 'vaultSecret') && !missing(e).length) run()
}

const ids = computed(() => props.nodes.map((n) => n.id))

async function run() {
  if (!picked.value) return
  const given = Object.fromEntries(Object.entries(params.value).filter(([, v]) => v !== ''))
  try {
    if (picked.value.template_id)
      await launchTemplate(props.sketchId, picked.value.template_id, ids.value, given)
    else await launchTransform(props.sketchId, picked.value.name, ids.value, given)
    toast.success(`${picked.value.name} started`)
    emit('launched')
  } catch (err) {
    toast.error(apiErrorMessage(err))
  }
}

async function runFlow(f: Flow) {
  try {
    await launchFlow(props.sketchId, f.id, ids.value)
    toast.success(`flow ${f.name} started`)
    emit('launched')
  } catch (err) {
    toast.error(apiErrorMessage(err))
  }
}
</script>

<template>
  <BaseModal
    :title="`Run a transform on ${nodes.length} ${type}`"
    size="modal-w640"
    @close="emit('close')"
  >
    <LoadingBlock v-if="loading" />
    <template v-else-if="!picked">
      <input v-model="filter" class="form-input mb-3" placeholder="Filter transforms" />
      <div class="choices">
        <button v-for="e in shown" :key="e.name + e.template_id" class="choice" @click="choose(e)">
          <span class="fw-medium">{{ e.name }}</span>
          <span v-if="e.template_id" class="badge badge-secondary">template</span>
          <span v-if="e.file" class="badge badge-secondary" :title="e.file">file</span>
          <span v-if="missing(e).length" class="badge badge-warning"
            >key missing: {{ missing(e).join(', ') }}</span
          >
          <span
            v-else-if="(e.params ?? []).some((p) => p.type === 'vaultSecret' && p.required)"
            class="badge badge-neutral"
            >key</span
          >
          <span class="text-sm text-muted">{{ e.description }}</span>
        </button>
        <p v-if="!shown.length" class="text-muted">No transform takes a {{ type }}.</p>
      </div>
      <template v-if="flows.length">
        <h4 class="mt-4 mb-2">Flows</h4>
        <div class="choices">
          <button v-for="f in flows" :key="f.id" class="choice" @click="runFlow(f)">
            <span class="fw-medium">{{ f.name }}</span>
            <span class="text-sm text-muted">{{ f.definition.steps.length }} steps</span>
          </button>
        </div>
      </template>
    </template>
    <form v-else @submit.prevent="run">
      <p class="text-secondary mb-3">{{ picked.description }}</p>
      <p v-if="missing(picked).length" class="notice notice-warning mb-3">
        No secret named {{ missing(picked).join(', ') }} is stored.
        <router-link to="/secrets">Add it in Secrets</router-link> first, or name another secret
        below.
      </p>
      <template v-for="p in picked.params ?? []" :key="p.name">
        <label v-if="p.type === 'bool'" class="checkbox-label mb-3" :title="p.description">
          <input
            type="checkbox"
            :checked="params[p.name] === 'true'"
            @change="
              params[p.name] = ($event.target as HTMLInputElement).checked ? 'true' : 'false'
            "
          />
          {{ p.description || p.name }}
        </label>
        <FormField v-else-if="p.type === 'select'" :label="p.name" :hint="p.description">
          <select v-model="params[p.name]" class="form-select">
            <option v-for="o in p.options ?? []" :key="o.value" :value="o.value">
              {{ o.label }}
            </option>
          </select>
        </FormField>
        <FormField
          v-else-if="p.type === 'vaultSecret'"
          :label="p.name"
          :hint="`Secret, ${p.name} by default. ${p.description}`"
        >
          <input v-model="params[p.name]" class="form-input" :placeholder="p.name" />
        </FormField>
        <FormField v-else :label="p.name" :hint="p.description" :required="p.required">
          <input
            v-model="params[p.name]"
            class="form-input"
            :type="p.type === 'number' ? 'number' : 'text'"
            :required="p.required"
          />
        </FormField>
      </template>
      <div class="flex gap-2 justify-end mt-4">
        <button type="button" class="btn btn-secondary" @click="picked = null">Back</button>
        <button class="btn btn-primary">Run</button>
      </div>
    </form>
  </BaseModal>
</template>

<style scoped>
.choices {
  display: flex;
  flex-direction: column;
  gap: 4px;
  max-height: 50vh;
  overflow: auto;
}

.choice {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 6px 8px;
  text-align: left;
  padding: 8px 10px;
  border: 1px solid var(--border-primary);
  border-radius: var(--radius-md);
  background: var(--bg-primary);
  color: var(--text-primary);
  cursor: pointer;
}

.choice:hover {
  background: var(--bg-hover);
}

.choice .text-muted {
  flex-basis: 100%;
}
</style>
