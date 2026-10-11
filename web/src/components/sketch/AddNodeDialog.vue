<script setup lang="ts">
// Adding an entity by hand. The form is the type's own fields, so a
// location with its city and country can be added like a domain can.
// With an entity selected, the new one is linked to it unless the box
// is cleared.
import { computed, ref, watch } from 'vue'
import { apiErrorMessage } from '@/api/client'
import { addEdge, addNode } from '@/api/sketches'
import type { GraphEdge, GraphNode } from '@/api/types'
import BaseModal from '@/components/BaseModal.vue'
import FormField from '@/components/FormField.vue'
import EntityFields from '@/components/sketch/EntityFields.vue'
import { formFields, toProperties, type FormValues } from '@/composables/entityForm'
import { labelChoices, normalizeLabel } from '@/composables/relationLabels'
import { useCatalogStore } from '@/stores/catalog'
import { useNotificationStore } from '@/stores/notification'

const props = defineProps<{
  sketchId: string
  linkTo?: GraphNode
  edges: GraphEdge[]
  initialType?: string
}>()
const emit = defineEmits<{ (e: 'close'): void; (e: 'added', n: GraphNode): void }>()

// Types a transform produces about another entity. Nobody types in a
// WHOIS record, so they sit in their own group at the end.
const FOUND = new Set([
  'whois',
  'sslcertificate',
  'riskprofile',
  'reputationscore',
  'breach',
  'gravatar',
  'webtracker',
  'technology',
  'port',
  'dnsrecord',
  'cryptowallettransaction',
  'cryptonft',
])

const catalog = useCatalogStore()
const toast = useNotificationStore()
const type = ref(props.initialType || 'domain')
const values = ref<FormValues>({})
const link = ref(true)
const relation = ref('RELATED_TO')
const outward = ref(true)
const error = ref('')
const busy = ref(false)

const grouped = computed(() => [
  ...catalog.categories
    .map((c) => ({
      ...c,
      types: catalog.types.filter((t) => t.category === c.key && !FOUND.has(t.name)),
    }))
    .filter((c) => c.types.length),
  {
    key: 'found',
    title: 'Usually found by transforms',
    types: catalog.types.filter((t) => FOUND.has(t.name)),
  },
])

const fields = computed(() => formFields(catalog.type(type.value)))
const labels = computed(() => labelChoices(props.edges.map((e) => e.label)))

watch(type, () => {
  values.value = {}
  error.value = ''
})

async function submit() {
  error.value = ''
  busy.value = true
  try {
    const n = await addNode(props.sketchId, {
      type: type.value,
      properties: toProperties(fields.value, values.value),
    })
    if (props.linkTo && link.value) {
      const [from, to] = outward.value ? [props.linkTo, n] : [n, props.linkTo]
      await addEdge(props.sketchId, from.id, to.id, normalizeLabel(relation.value))
    }

    emit('added', n)
  } catch (err) {
    error.value = apiErrorMessage(err)
    toast.error(error.value)
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <BaseModal title="Add an entity" size="modal-w480" form @close="emit('close')" @submit="submit">
    <FormField label="Type">
      <select v-model="type" class="form-select">
        <optgroup v-for="c in grouped" :key="c.key" :label="c.title">
          <option v-for="t in c.types" :key="t.name" :value="t.name">{{ t.title }}</option>
        </optgroup>
      </select>
    </FormField>

    <EntityFields v-model="values" :fields="fields" :error="error" autofocus />

    <template v-if="linkTo">
      <hr class="sep" />
      <label class="checkbox-label mb-3">
        <input v-model="link" type="checkbox" />
        Link it to <strong>{{ linkTo.label }}</strong>
      </label>
      <div v-if="link" class="form-row">
        <FormField label="Direction">
          <select v-model="outward" class="form-select">
            <option :value="true">{{ linkTo.label }} to the new entity</option>
            <option :value="false">The new entity to {{ linkTo.label }}</option>
          </select>
        </FormField>
        <FormField label="Relationship">
          <input v-model="relation" class="form-input" list="add-labels" required />
          <datalist id="add-labels">
            <option v-for="l in labels" :key="l" :value="l" />
          </datalist>
        </FormField>
      </div>
    </template>

    <template #footer>
      <button type="button" class="btn btn-secondary" @click="emit('close')">Cancel</button>
      <button class="btn btn-primary" :disabled="busy">
        {{ linkTo && link ? 'Add and link' : 'Add' }}
      </button>
    </template>
  </BaseModal>
</template>

<style scoped>
.sep {
  border: 0;
  border-top: 1px solid var(--border-primary);
  margin: 4px 0 16px;
}
</style>
