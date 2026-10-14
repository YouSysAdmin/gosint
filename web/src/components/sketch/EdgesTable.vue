<script setup lang="ts">
// The relationships as a table, from one entity through a label to
// another. A row picks the relationship, an end jumps to its entity.
import { computed, ref } from 'vue'
import type { GraphEdge, GraphNode } from '@/api/types'
import EntityIcon from '@/components/sketch/EntityIcon.vue'
import { downloadCSV } from '@/composables/csv'
import { formatDate } from '@/composables/formatDate'

const props = defineProps<{ nodes: GraphNode[]; edges: GraphEdge[]; selectedEdge: string }>()
const emit = defineEmits<{ (e: 'pick', id: string): void; (e: 'focus', id: string): void }>()

const search = ref('')
const labelFilter = ref('')

const byId = computed(() => new Map(props.nodes.map((n) => [n.id, n])))
const labels = computed(() => [...new Set(props.edges.map((e) => e.label))].sort())

// exportCSV writes the shown relationships, each end by its type and
// label.
function exportCSV() {
  downloadCSV(
    'relationships.csv',
    ['from_type', 'from', 'relationship', 'to_type', 'to', 'properties'],
    rows.value.map(({ e, from, to }) => [
      from?.type,
      from?.label,
      e.label,
      to?.type,
      to?.label,
      Object.keys(e.properties ?? {}).length ? e.properties : '',
    ]),
  )
}

const rows = computed(() => {
  const q = search.value.trim().toLowerCase()
  return props.edges
    .map((e) => ({ e, from: byId.value.get(e.source), to: byId.value.get(e.target) }))
    .filter(({ e }) => !labelFilter.value || e.label === labelFilter.value)
    .filter(
      ({ e, from, to }) =>
        !q ||
        e.label.toLowerCase().includes(q) ||
        from?.label.toLowerCase().includes(q) ||
        to?.label.toLowerCase().includes(q),
    )
    .sort((a, b) => a.e.label.localeCompare(b.e.label))
})
</script>

<template>
  <div class="table-view">
    <div class="bar">
      <input
        v-model="search"
        class="form-input"
        placeholder="Search relationships and entities"
        aria-label="Search relationships and entities"
      />
      <select v-model="labelFilter" class="form-select" aria-label="Only one relationship">
        <option value="">All relationships</option>
        <option v-for="l in labels" :key="l" :value="l">{{ l }}</option>
      </select>
      <span class="text-sm text-muted">{{ rows.length }} of {{ edges.length }}</span>
      <button class="btn btn-sm btn-secondary" :disabled="!rows.length" @click="exportCSV">
        CSV
      </button>
    </div>
    <div class="table-wrapper grow">
      <table class="table">
        <thead>
          <tr>
            <th>From</th>
            <th>Relationship</th>
            <th>To</th>
            <th class="cell-nowrap">Added</th>
          </tr>
        </thead>
        <tbody>
          <tr
            v-for="{ e, from, to } in rows"
            :key="e.id"
            class="row-clickable"
            :class="{ on: e.id === selectedEdge }"
            tabindex="0"
            @click="emit('pick', e.id)"
            @keydown.enter.self="emit('pick', e.id)"
          >
            <td>
              <a v-if="from" href="#" class="end" @click.prevent.stop="emit('focus', from.id)">
                <EntityIcon :type="from.type" />{{ from.label }}
              </a>
            </td>
            <td class="code-font text-sm">{{ e.label }}</td>
            <td>
              <a v-if="to" href="#" class="end" @click.prevent.stop="emit('focus', to.id)">
                <EntityIcon :type="to.type" />{{ to.label }}
              </a>
            </td>
            <td class="cell-nowrap text-muted text-sm">{{ formatDate(e.created_at) }}</td>
          </tr>
          <tr v-if="!rows.length">
            <td colspan="4" class="text-muted">
              {{
                edges.length ? 'No relationship matches.' : 'This sketch has no relationship yet.'
              }}
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

.end {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  color: var(--text-primary);
}

tr.on td {
  background: var(--bg-active);
}

tr.row-clickable:focus-visible {
  outline: 2px solid var(--border-focus);
  outline-offset: -2px;
}
</style>
