<script setup lang="ts">
// The entities as a table: searchable, filtered by type, tickable, with
// the properties summed up in one column.
import { computed, ref } from 'vue'
import type { GraphNode } from '@/api/types'
import EntityIcon from '@/components/sketch/EntityIcon.vue'
import { downloadCSV } from '@/composables/csv'
import { formatDate } from '@/composables/formatDate'
import { useCatalogStore } from '@/stores/catalog'
import { useNotificationStore } from '@/stores/notification'

const props = defineProps<{ nodes: GraphNode[]; selected: Set<string> }>()
const emit = defineEmits<{
  (e: 'focus', id: string): void
  (e: 'toggle', id: string): void
  (e: 'selectAll', ids: string[]): void
  (e: 'unselect', ids: string[]): void
}>()

const catalog = useCatalogStore()
const toast = useNotificationStore()
const search = ref('')
const typeFilter = ref('')

const types = computed(() => [...new Set(props.nodes.map((n) => n.type))].sort())

const shown = computed(() => {
  const q = search.value.trim().toLowerCase()
  return props.nodes
    .filter((n) => !typeFilter.value || n.type === typeFilter.value)
    .filter((n) => !q || n.label.toLowerCase().includes(q) || summary(n).toLowerCase().includes(q))
    .sort((a, b) => a.type.localeCompare(b.type) || a.label.localeCompare(b.label))
})

const allShown = computed(
  () => shown.value.length > 0 && shown.value.every((n) => props.selected.has(n.id)),
)
const someShown = computed(
  () => !allShown.value && shown.value.some((n) => props.selected.has(n.id)),
)

// summary is the properties as "key: value" pairs, the primary one left
// out since the label already shows it.
function summary(n: GraphNode): string {
  const primary = catalog.type(n.type)?.fields.find((f) => f.primary)?.name
  return Object.entries(n.properties ?? {})
    .filter(([k, v]) => k !== primary && v !== null && v !== '' && typeof v !== 'object')
    .map(([k, v]) => `${k}: ${v}`)
    .join(', ')
}

// exportCSV writes the shown entities: type and label first, then every
// scalar property any of them has, a list or object as JSON.
function exportCSV() {
  const keys = new Set<string>()
  for (const n of shown.value) for (const k of Object.keys(n.properties ?? {})) keys.add(k)
  const cols = [...keys].sort()
  downloadCSV(
    'entities.csv',
    ['id', 'type', 'label', ...cols],
    shown.value.map((n) => [n.id, n.type, n.label, ...cols.map((k) => n.properties?.[k])]),
  )
}

async function copy(n: GraphNode) {
  try {
    await navigator.clipboard.writeText(n.label)
    toast.success('Copied')
  } catch {
    toast.error('The clipboard is not available')
  }
}
</script>

<template>
  <div class="table-view">
    <div class="bar">
      <input
        v-model="search"
        class="form-input"
        placeholder="Search entities and their properties"
      />
      <select v-model="typeFilter" class="form-select">
        <option value="">All types</option>
        <option v-for="t in types" :key="t" :value="t">{{ catalog.type(t)?.title ?? t }}</option>
      </select>
      <span class="text-sm text-muted">{{ shown.length }} of {{ nodes.length }}</span>
      <button class="btn btn-sm btn-secondary" :disabled="!shown.length" @click="exportCSV">
        CSV
      </button>
    </div>
    <div class="table-wrapper grow">
      <table class="table">
        <thead>
          <tr>
            <th class="tick">
              <input
                type="checkbox"
                :checked="allShown"
                :indeterminate="someShown"
                :disabled="!shown.length"
                @change="
                  allShown
                    ? emit(
                        'unselect',
                        shown.map((n) => n.id),
                      )
                    : emit(
                        'selectAll',
                        shown.map((n) => n.id),
                      )
                "
              />
            </th>
            <th>Type</th>
            <th>Label</th>
            <th>Properties</th>
            <th class="cell-nowrap">Added</th>
            <th></th>
          </tr>
        </thead>
        <tbody>
          <tr
            v-for="n in shown"
            :key="n.id"
            class="row-clickable"
            :class="{ on: selected.has(n.id) }"
            @click="emit('focus', n.id)"
          >
            <td class="tick" @click.stop>
              <input type="checkbox" :checked="selected.has(n.id)" @change="emit('toggle', n.id)" />
            </td>
            <td class="cell-nowrap">
              <span class="type"
                ><EntityIcon :type="n.type" />{{ catalog.type(n.type)?.title ?? n.type }}</span
              >
            </td>
            <td class="cell-title">{{ n.label }}</td>
            <td class="text-muted text-sm props">{{ summary(n) }}</td>
            <td class="cell-nowrap text-muted text-sm">{{ formatDate(n.created_at) }}</td>
            <td class="cell-nowrap" @click.stop>
              <button class="btn btn-sm btn-secondary" title="Copy the label" @click="copy(n)">
                Copy
              </button>
            </td>
          </tr>
          <tr v-if="!shown.length">
            <td colspan="6" class="text-muted">
              {{ nodes.length ? 'No entity matches.' : 'This sketch has no entity yet.' }}
            </td>
          </tr>
        </tbody>
      </table>
    </div>
  </div>
</template>

<style scoped>
.table-view {
  display: flex;
  flex-direction: column;
  height: 100%;
}

.bar {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 10px 16px;
}

.bar .form-input {
  max-width: 360px;
}

.bar .form-select {
  width: auto;
}

.grow {
  flex: 1;
  min-height: 0;
  overflow: auto;
  border: 0;
  border-top: 1px solid var(--border-primary);
  border-radius: 0;
}

.tick {
  width: 32px;
}

.type {
  display: inline-flex;
  align-items: center;
  gap: 6px;
}

.props {
  max-width: 480px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

tr.on td {
  background: var(--bg-active);
}
</style>
