<script setup lang="ts">
// The entity types an administrator declares beside the built-in ones:
// a name, a title, a palette category, an icon and a colour, and the
// fields, one of them the primary (the entity's name). A type with
// entities in any sketch cannot be deleted.
import { onMounted, ref } from 'vue'
import {
  deleteCustomType,
  listCustomTypes,
  saveCustomType,
  type CustomTypeInput,
} from '@/api/admin'
import { apiErrorMessage } from '@/api/client'
import type { CustomField, CustomType } from '@/api/types'
import BaseModal from '@/components/BaseModal.vue'
import EntityIcon from '@/components/sketch/EntityIcon.vue'
import FormField from '@/components/FormField.vue'
import PageHeader from '@/components/PageHeader.vue'
import AdminNav from '@/views/admin/AdminNav.vue'
import { iconFor, iconNames } from '@/composables/entityIcons'
import { useConfirm } from '@/composables/useConfirm'
import { useCatalogStore } from '@/stores/catalog'
import { useNotificationStore } from '@/stores/notification'

const catalog = useCatalogStore()
const toast = useNotificationStore()
const { confirm } = useConfirm()

const list = ref<CustomType[]>([])
const loading = ref(true)
// A field in the form keeps its options as the text typed, one per
// line, parsed into the list only on save, so Enter is not swallowed by
// a round trip through the list while typing.
type FieldDraft = CustomField & { optionsText: string }
type TypeDraft = Omit<CustomTypeInput, 'fields'> & { fields: FieldDraft[] }

const editing = ref<{ id: string | null; t: TypeDraft } | null>(null)
const busy = ref(false)
const kinds = ['string', 'int', 'float', 'bool', 'datetime', 'url', 'email', 'list']

// The primary field is the entity's name, so it holds text.
const primaryKinds = ['string', 'url', 'email']

function kindsFor(f: FieldDraft): string[] {
  return f.primary ? primaryKinds : kinds
}

async function load() {
  try {
    list.value = await listCustomTypes()
  } catch (err) {
    toast.error(apiErrorMessage(err))
  } finally {
    loading.value = false
  }
}

onMounted(async () => {
  await Promise.all([load(), catalog.load()])
})

function blankField(primary = false): FieldDraft {
  return {
    name: '',
    title: '',
    description: '',
    kind: 'string',
    required: primary,
    primary,
    long: false,
    options: null,
    optionsText: '',
  }
}

function draftOf(f: CustomField): FieldDraft {
  return { ...f, optionsText: (f.options ?? []).join('\n') }
}

function fieldOf(d: FieldDraft): CustomField {
  const { optionsText, ...f } = d
  const opts = optionsText
    .split('\n')
    .map((s) => s.trim())
    .filter(Boolean)

  return { ...f, options: opts.length ? opts : null }
}

function open(t: CustomType | null) {
  editing.value = t
    ? { id: t.id, t: { ...t, fields: t.fields.map(draftOf) } }
    : {
        id: null,
        t: {
          name: '',
          title: '',
          description: '',
          category: 'global',
          icon: 'Tag',
          color: '#6b7280',
          fields: [blankField(true)],
        },
      }
}

function setPrimary(i: number) {
  if (!editing.value) return
  editing.value.t.fields.forEach((f, j) => (f.primary = j === i))
  const f = editing.value.t.fields[i]
  if (!primaryKinds.includes(f.kind)) f.kind = 'string'
  f.required = true
}

function removeField(i: number) {
  if (!editing.value) return
  const fields = editing.value.t.fields
  const wasPrimary = fields[i].primary
  fields.splice(i, 1)
  if (wasPrimary && fields.length) setPrimary(0)
}

// setColor takes the hex typed beside the swatch once it is a colour.
function setColor(text: string) {
  if (!editing.value) return
  const v = text.trim()
  if (/^#[0-9a-fA-F]{6}$/.test(v)) editing.value.t.color = v.toLowerCase()
}

// optionRows sizes the options box to its lines, three at least.
function optionRows(f: FieldDraft): number {
  return Math.max(3, f.optionsText.split('\n').length + 1)
}

async function save() {
  if (!editing.value) return
  busy.value = true
  try {
    const t = { ...editing.value.t, fields: editing.value.t.fields.map(fieldOf) }
    await saveCustomType(editing.value.id, t)
    toast.success(editing.value.id ? 'Type updated' : 'Type declared')
    editing.value = null
    await Promise.all([load(), catalog.reload()])
  } catch (err) {
    toast.error(apiErrorMessage(err))
  } finally {
    busy.value = false
  }
}

async function remove(t: CustomType) {
  if (!(await confirm({ title: 'Delete the type', message: `Delete the type ${t.title}?` }))) return
  try {
    await deleteCustomType(t.id)
    await Promise.all([load(), catalog.reload()])
  } catch (err) {
    toast.error(apiErrorMessage(err))
  }
}
</script>

<template>
  <PageHeader title="Entity types">
    <button class="btn btn-primary" @click="open(null)">Declare a type</button>
  </PageHeader>
  <AdminNav />

  <div class="card">
    <div class="card-body">
      <p v-if="loading" class="text-sm text-muted">Loading...</p>
      <p v-else-if="!list.length" class="text-sm text-muted">
        No type of your own yet. The built-in ones cover the usual ground, a type declared here
        joins them in the palette, the forms and the import for every account.
      </p>
      <table v-else class="table">
        <thead>
          <tr>
            <th>Type</th>
            <th>Category</th>
            <th>Fields</th>
            <th></th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="t in list" :key="t.id">
            <td>
              <span class="flex items-center gap-2">
                <EntityIcon :type="t.name" :size="16" />
                <span class="fw-medium">{{ t.title }}</span>
                <span class="code-font text-xs text-muted">{{ t.name }}</span>
              </span>
              <div v-if="t.description" class="text-xs text-muted">{{ t.description }}</div>
            </td>
            <td>{{ catalog.categories.find((c) => c.key === t.category)?.title ?? t.category }}</td>
            <td class="text-sm">
              {{ t.fields.map((f) => f.name + (f.primary ? ' *' : '')).join(', ') }}
            </td>
            <td class="text-right cell-nowrap">
              <button class="btn btn-sm btn-secondary" @click="open(t)">Edit</button>
              <button class="btn btn-sm btn-danger ml-2" @click="remove(t)">Delete</button>
            </td>
          </tr>
        </tbody>
      </table>
    </div>
  </div>

  <BaseModal
    v-if="editing"
    :title="editing.id ? 'Edit the type' : 'Declare a type'"
    size="modal-w760"
    @close="editing = null"
  >
    <form id="type-form" @submit.prevent="save">
      <div class="grid-2">
        <FormField
          label="Name"
          hint="Lowercase, letters, digits and underscores, fixed once made"
          required
        >
          <input
            v-model="editing.t.name"
            class="form-input code-font"
            :disabled="!!editing.id"
            required
            pattern="[a-z][a-z0-9_]*"
          />
        </FormField>
        <FormField label="Title" required>
          <input v-model="editing.t.title" class="form-input" required />
        </FormField>
        <FormField label="Category">
          <select v-model="editing.t.category" class="form-select">
            <option v-for="c in catalog.categories" :key="c.key" :value="c.key">
              {{ c.title }}
            </option>
          </select>
        </FormField>
        <FormField label="Icon">
          <div class="icon-pick">
            <span class="icon-preview" :style="{ color: editing.t.color }">
              <component :is="iconFor(editing.t.icon)" :size="18" :stroke-width="2" />
            </span>
            <select v-model="editing.t.icon" class="form-select">
              <option v-for="i in iconNames" :key="i" :value="i">{{ i }}</option>
            </select>
          </div>
        </FormField>
        <FormField label="Colour">
          <div class="colour-pick">
            <input v-model="editing.t.color" type="color" class="colour-swatch" />
            <input
              class="form-input code-font"
              :value="editing.t.color"
              maxlength="7"
              placeholder="#6b7280"
              @change="setColor(($event.target as HTMLInputElement).value)"
            />
          </div>
        </FormField>
        <FormField label="Description">
          <input v-model="editing.t.description" class="form-input" />
        </FormField>
      </div>

      <h4 class="mt-3 mb-2">Fields</h4>
      <p class="text-xs text-muted mb-3">
        The primary field is the entity's name and what a typed value fills.
      </p>
      <div
        v-for="(f, i) in editing.t.fields"
        :key="i"
        class="field-card"
        :class="{ primary: f.primary }"
      >
        <div class="field-head">
          <label class="radio-label">
            <input type="radio" name="primary" :checked="f.primary" @change="setPrimary(i)" />
            Primary
          </label>
          <span v-if="f.primary" class="text-xs text-muted">the entity's name</span>
          <button
            type="button"
            class="btn btn-sm btn-secondary ml-auto"
            :disabled="editing.t.fields.length === 1"
            @click="removeField(i)"
          >
            Remove
          </button>
        </div>
        <div class="field-grid">
          <FormField label="Name" required>
            <input
              v-model="f.name"
              class="form-input code-font"
              placeholder="first_name"
              required
              pattern="[a-z][a-z0-9_]*"
            />
          </FormField>
          <FormField label="Title">
            <input v-model="f.title" class="form-input" placeholder="First name" />
          </FormField>
          <FormField label="Kind">
            <select v-model="f.kind" class="form-select">
              <option v-for="k in kindsFor(f)" :key="k" :value="k">{{ k }}</option>
            </select>
          </FormField>
        </div>
        <FormField label="Description" hint="Shown under the input when the entity is edited">
          <input v-model="f.description" class="form-input" />
        </FormField>
        <FormField
          v-if="f.kind === 'string'"
          label="Options"
          hint="One per line, the field becomes a select. Empty, it is free text."
        >
          <textarea v-model="f.optionsText" class="form-textarea" :rows="optionRows(f)" />
        </FormField>
        <div class="field-flags">
          <label class="checkbox-label">
            <input v-model="f.required" type="checkbox" :disabled="f.primary" /> Required
          </label>
          <label v-if="f.kind === 'string'" class="checkbox-label">
            <input v-model="f.long" type="checkbox" /> Long text
          </label>
        </div>
      </div>
      <button
        type="button"
        class="btn btn-sm btn-secondary"
        @click="editing.t.fields.push(blankField())"
      >
        Add a field
      </button>
    </form>
    <template #footer>
      <button class="btn btn-secondary" @click="editing = null">Cancel</button>
      <button class="btn btn-primary" form="type-form" :disabled="busy">Save</button>
    </template>
  </BaseModal>
</template>

<style scoped>
.grid-2 {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 0 16px;
}

.icon-pick,
.colour-pick {
  display: flex;
  align-items: center;
  gap: 8px;
}

.icon-preview {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  flex: none;
  width: var(--control-h);
  height: var(--control-h);
  border: 1px solid var(--border-input);
  border-radius: var(--radius);
  background: var(--bg-input);
}

.colour-swatch {
  flex: none;
  width: var(--control-h);
  height: var(--control-h);
  padding: 2px;
  border: 1px solid var(--border-input);
  border-radius: var(--radius);
  background: var(--bg-input);
  cursor: pointer;
}

.field-card {
  padding: 12px 14px 0;
  margin-bottom: 12px;
  border: 1px solid var(--border-primary);
  border-radius: var(--radius);
}

.field-card.primary {
  border-color: var(--primary-600);
}

.field-head {
  display: flex;
  align-items: center;
  gap: 10px;
  margin-bottom: 12px;
}

.radio-label {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 14px;
  font-weight: 500;
  color: var(--text-secondary);
  cursor: pointer;
}

.field-grid {
  display: grid;
  grid-template-columns: 1fr 1fr 140px;
  gap: 0 12px;
}

.field-card .form-textarea {
  min-height: 0;
}

.field-flags {
  display: flex;
  gap: 20px;
  margin-bottom: 16px;
}

@media (max-width: 760px) {
  .grid-2,
  .field-grid {
    grid-template-columns: 1fr;
  }
}
</style>
