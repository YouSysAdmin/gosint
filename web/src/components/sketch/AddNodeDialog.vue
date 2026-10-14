<script setup lang="ts">
// Adding an entity by hand. The form is the type's own fields, so a
// location with its city and country can be added like a domain can.
// With an entity selected, the new one is linked to it unless the box
// is cleared. A clone starts from another entity's type, custom field
// names and look, with every value left to fill in.
import { computed, ref, watch } from 'vue'
import { apiErrorMessage } from '@/api/client'
import { addEdge, addNode } from '@/api/sketches'
import type { GraphEdge, GraphNode } from '@/api/types'
import BaseModal from '@/components/BaseModal.vue'
import FormField from '@/components/FormField.vue'
import CustomFields from '@/components/sketch/CustomFields.vue'
import EntityFields from '@/components/sketch/EntityFields.vue'
import { CUSTOM_KEY, customOf, typed, type CustomRow } from '@/composables/customFields'
import { formFields, toProperties, type FormValues } from '@/composables/entityForm'
import { labelChoices, normalizeLabel } from '@/composables/relationLabels'
import { useCatalogStore } from '@/stores/catalog'
import { useNotificationStore } from '@/stores/notification'

const props = defineProps<{
  sketchId: string
  linkTo?: GraphNode
  edges: GraphEdge[]
  initialType?: string
  cloneOf?: GraphNode
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
const type = ref(props.cloneOf?.type || props.initialType || 'domain')
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
const declared = computed(() => new Set(catalog.type(type.value)?.fields.map((f) => f.name) ?? []))
const custom = ref<CustomRow[]>(cloneRows())
const customError = ref('')

// cloneRows is the clone's custom fields as new rows, names and kinds
// only.
function cloneRows(): CustomRow[] {
  const n = props.cloneOf
  if (!n) return []
  return customOf(n.properties ?? {}, declared.value).map((r) => ({
    key: r.key,
    value: '',
    kind: r.kind,
  }))
}

// customProperties reads the custom rows, or a message naming a bad
// one. A named row left empty is kept as an empty text, so the field
// is there to fill in later.
function customProperties(): Record<string, unknown> | string {
  const out: Record<string, unknown> = {}
  for (const r of custom.value) r.error = undefined
  for (const r of custom.value) {
    const key = r.key.trim()
    if (!key && !r.value.trim()) continue
    if (!CUSTOM_KEY.test(key))
      return `"${key}" is not a field name, use letters, digits, spaces, - and _`
    if (declared.value.has(key)) return `${key} is a field of the type, set it above`
    if (key in out) return `${key} is there twice`
    if (r.value.trim() === '') {
      out[key] = ''
      continue
    }

    const t = typed(r)
    if ('error' in t) {
      r.error = t.error
      return `${key} is ${t.error}`
    }
    out[key] = t.value
  }

  return out
}
const labels = computed(() => labelChoices(props.edges.map((e) => e.label)))

watch(type, () => {
  values.value = {}
  error.value = ''
})

async function submit() {
  error.value = ''
  customError.value = ''
  const extra = customProperties()
  if (typeof extra === 'string') {
    customError.value = extra
    return
  }

  const look = props.cloneOf
  busy.value = true
  try {
    const n = await addNode(props.sketchId, {
      type: type.value,
      properties: { ...extra, ...toProperties(fields.value, values.value) },
      ...(look && {
        color: look.color,
        icon: look.icon,
        shape: look.shape,
        size: look.size ?? undefined,
      }),
    })
    // The entity exists once addNode answered: a failed link is reported
    // and the dialog still closes, or a retry would add it twice.
    if (props.linkTo && link.value) {
      const [from, to] = outward.value ? [props.linkTo, n] : [n, props.linkTo]
      try {
        await addEdge(props.sketchId, from.id, to.id, normalizeLabel(relation.value))
      } catch (err) {
        toast.error('Entity added, the link failed: ' + apiErrorMessage(err))
      }
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
  <BaseModal
    :title="cloneOf ? `Clone ${cloneOf.label}` : 'Add an entity'"
    size="modal-w480"
    form
    @close="emit('close')"
    @submit="submit"
  >
    <FormField label="Type">
      <select v-model="type" class="form-select">
        <optgroup v-for="c in grouped" :key="c.key" :label="c.title">
          <option v-for="t in c.types" :key="t.name" :value="t.name">{{ t.title }}</option>
        </optgroup>
      </select>
    </FormField>

    <EntityFields v-model="values" :fields="fields" :error="error" autofocus />
    <CustomFields
      v-if="cloneOf || custom.length"
      v-model="custom"
      :error="customError"
      @remove="(i) => (custom = custom.filter((_, j) => j !== i))"
    />

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
