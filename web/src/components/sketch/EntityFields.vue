<script setup lang="ts">
// The fields of one entity type as a form. Required fields are always
// shown, the rest behind a toggle, which starts open when any of them
// has a value already. Every text and number input writes a string, so
// a number field compares like the rest and toProperties types it.
import { computed, onMounted, ref, watch } from 'vue'
import { listKeys } from '@/api/catalog'
import type { TypeField } from '@/api/types'
import FormField from '@/components/FormField.vue'
import { control, hint, rows, type FormValues } from '@/composables/entityForm'

const props = defineProps<{ fields: TypeField[]; error?: string; autofocus?: boolean }>()
const values = defineModel<FormValues>({ required: true })

// A field naming a secret offers the account's secrets. When the list
// cannot be read the field stays a text input.
const keyNames = ref<string[]>([])
async function loadKeys() {
  if (!props.fields.some((f) => f.vault_key)) return
  try {
    keyNames.value = (await listKeys()).map((k) => k.name)
  } catch {
    keyNames.value = []
  }
}

function keyOptions(f: TypeField): string[] {
  const current = String(values.value[f.name] ?? '')
  return current && !keyNames.value.includes(current)
    ? [current, ...keyNames.value]
    : keyNames.value
}

onMounted(loadKeys)

// The primary field is always shown, required or not: it is what names
// the entity.
const required = computed(() => props.fields.filter((f) => f.required || f.primary))
const optional = computed(() => props.fields.filter((f) => !f.required && !f.primary))
const more = ref(optional.value.some((f) => values.value[f.name] !== undefined))

watch(
  () => props.fields,
  () => {
    more.value = optional.value.some((f) => values.value[f.name] !== undefined)
    void loadKeys()
  },
)
</script>

<template>
  <template v-for="(f, i) in required" :key="f.name">
    <label v-if="control(f) === 'checkbox'" class="checkbox-label mb-3">
      <input v-model="values[f.name]" type="checkbox" />
      {{ f.title }}
    </label>
    <FormField
      v-else
      :label="f.title"
      :hint="hint(f)"
      :error="f.primary ? error : ''"
      :required="f.required || f.primary"
    >
      <select v-if="control(f) === 'select'" v-model="values[f.name]" class="form-select" required>
        <option value="" disabled>Pick one</option>
        <option v-for="o in f.options" :key="o" :value="o">{{ o }}</option>
      </select>
      <select
        v-else-if="control(f) === 'vaultkey' && keyOptions(f).length"
        v-model="values[f.name]"
        class="form-select"
      >
        <option value="">None</option>
        <option v-for="o in keyOptions(f)" :key="o" :value="o">{{ o }}</option>
      </select>
      <textarea
        v-else-if="control(f) === 'textarea'"
        class="form-textarea"
        :rows="rows(values[f.name], 4)"
        :value="String(values[f.name] ?? '')"
        :autofocus="autofocus && i === 0"
        :required="f.required || f.primary"
        @input="values[f.name] = ($event.target as HTMLTextAreaElement).value"
      />
      <input
        v-else
        class="form-input"
        :type="control(f) === 'number' ? 'number' : 'text'"
        :step="f.kind === 'float' ? 'any' : undefined"
        :value="String(values[f.name] ?? '')"
        :autofocus="autofocus && i === 0"
        :required="f.required || f.primary"
        @input="values[f.name] = ($event.target as HTMLInputElement).value"
      />
    </FormField>
  </template>

  <template v-if="optional.length">
    <button type="button" class="btn btn-sm btn-secondary mb-3" @click="more = !more">
      {{ more ? 'Fewer fields' : `More fields (${optional.length})` }}
    </button>
    <template v-if="more">
      <template v-for="f in optional" :key="f.name">
        <label v-if="control(f) === 'checkbox'" class="checkbox-label mb-3">
          <input v-model="values[f.name]" type="checkbox" />
          {{ f.title }}
        </label>
        <FormField v-else :label="f.title" :hint="hint(f)">
          <select v-if="control(f) === 'select'" v-model="values[f.name]" class="form-select">
            <option value="">Not set</option>
            <option v-for="o in f.options" :key="o" :value="o">{{ o }}</option>
          </select>
          <select
            v-else-if="control(f) === 'vaultkey' && keyOptions(f).length"
            v-model="values[f.name]"
            class="form-select"
          >
            <option value="">None</option>
            <option v-for="o in keyOptions(f)" :key="o" :value="o">{{ o }}</option>
          </select>
          <textarea
            v-else-if="control(f) === 'textarea'"
            class="form-textarea"
            :rows="rows(values[f.name], 3)"
            :value="String(values[f.name] ?? '')"
            @input="values[f.name] = ($event.target as HTMLTextAreaElement).value"
          />
          <input
            v-else
            class="form-input"
            :type="control(f) === 'number' ? 'number' : 'text'"
            :step="f.kind === 'float' ? 'any' : undefined"
            :value="String(values[f.name] ?? '')"
            @input="values[f.name] = ($event.target as HTMLInputElement).value"
          />
        </FormField>
      </template>
    </template>
  </template>
</template>
