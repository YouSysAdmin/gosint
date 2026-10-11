<script setup lang="ts">
// The custom field rows of an entity form. A stored row keeps its name
// and kind, a new one picks both. Removing a row is the parent's, which
// may have to clear a stored field.
import { CUSTOM_KINDS, setKind, type CustomKind, type CustomRow } from '@/composables/customFields'

defineProps<{ error?: string }>()
const rows = defineModel<CustomRow[]>({ required: true })
const emit = defineEmits<{ (e: 'remove', i: number): void }>()

function add() {
  rows.value = [...rows.value, { key: '', value: '', kind: 'string' }]
}
</script>

<template>
  <div class="custom mb-3">
    <div class="custom-head">
      <span class="text-sm fw-medium">Custom fields</span>
      <button type="button" class="btn btn-sm btn-secondary" @click="add">Add a field</button>
    </div>
    <p v-if="!rows.length" class="text-xs text-muted">
      Anything the type does not cover, a name, a kind and a value. A date is kept as YYYY-MM-DD
      text. Filters find them too.
    </p>
    <div v-for="(r, i) in rows" :key="i" class="custom-row">
      <input
        v-if="r.was === undefined"
        v-model="r.key"
        class="form-input"
        placeholder="Name"
        maxlength="64"
      />
      <span v-else class="custom-key text-sm" :title="r.key">{{ r.key }}</span>
      <select
        v-if="r.was === undefined"
        class="form-select"
        :value="r.kind"
        title="What the value is"
        @change="setKind(r, ($event.target as HTMLSelectElement).value as CustomKind)"
      >
        <option v-for="k in CUSTOM_KINDS" :key="k.value" :value="k.value">
          {{ k.label }}
        </option>
      </select>
      <span v-else class="custom-kind text-xs text-muted">
        {{ CUSTOM_KINDS.find((k) => k.value === r.kind)?.label }}
      </span>
      <select v-if="r.kind === 'bool'" v-model="r.value" class="form-select custom-value">
        <option value="true">Yes</option>
        <option value="false">No</option>
      </select>
      <input
        v-else-if="r.kind === 'date'"
        v-model="r.value"
        type="date"
        class="form-input custom-value"
        title="Stored as YYYY-MM-DD text"
      />
      <input
        v-else
        v-model="r.value"
        class="form-input custom-value"
        :aria-invalid="r.error ? 'true' : undefined"
        :inputmode="r.kind === 'number' ? 'decimal' : undefined"
        :placeholder="r.kind === 'number' ? '0' : 'Value'"
      />
      <button type="button" class="unlink" title="Remove the field" @click="emit('remove', i)">
        &times;
      </button>
      <p v-if="r.error" class="form-error custom-error">{{ r.key }} is {{ r.error }}</p>
    </div>
    <p v-if="error" class="text-xs text-danger mt-1">{{ error }}</p>
  </div>
</template>

<style scoped>
.custom {
  padding-top: 10px;
  border-top: 1px solid var(--border-secondary);
}

.custom-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 6px;
}

.custom-row {
  display: grid;
  grid-template-columns: 1fr 128px auto;
  gap: 6px;
  align-items: center;
  margin-bottom: 8px;
  padding: 8px;
  border: 1px solid var(--border-secondary);
  border-radius: var(--radius-sm);
}

.custom-row .custom-value {
  grid-row: 2;
  grid-column: 1 / -1;
}

.custom-error {
  grid-column: 1 / -1;
  margin: 0;
}

.custom-row .form-input[aria-invalid='true'] {
  border-color: var(--danger-500);
}

.unlink {
  grid-row: 1;
  grid-column: 3;
  padding: 0 4px;
  border: 0;
  background: none;
  color: var(--text-muted);
  font-size: 14px;
  line-height: 1;
  cursor: pointer;
}

.custom-key {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  color: var(--text-secondary);
}
</style>
