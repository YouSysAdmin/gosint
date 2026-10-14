<script setup lang="ts">
// The selected relationship: its label, editable, and its two ends.
import { computed, ref, watch } from 'vue'
import { apiErrorMessage } from '@/api/client'
import { deleteEdges, updateEdge } from '@/api/sketches'
import type { GraphEdge, GraphNode } from '@/api/types'
import { describeOrigin, originDate, originOf } from '@/composables/provenance'
import { normalizeLabel } from '@/composables/relationLabels'
import { useConfirm } from '@/composables/useConfirm'
import { useCatalogStore } from '@/stores/catalog'
import { useNotificationStore } from '@/stores/notification'

const props = defineProps<{
  sketchId: string
  edge: GraphEdge
  nodes: GraphNode[]
  canEdit: boolean
}>()
const emit = defineEmits<{
  (e: 'changed'): void
  (e: 'focus', id: string): void
  (e: 'scan', id: string): void
}>()

const catalog = useCatalogStore()
const toast = useNotificationStore()
const { confirm } = useConfirm()
const label = ref(props.edge.label)
const busy = ref(false)
const origin = computed(() => originOf(props.edge.properties))

// The relationship's own properties, its origin shown apart.
const shown = computed(() =>
  Object.entries(props.edge.properties ?? {}).filter(([k]) => k !== 'origin'),
)

watch(
  () => props.edge.id,
  () => (label.value = props.edge.label),
)

function end(id: string): GraphNode | undefined {
  return props.nodes.find((n) => n.id === id)
}

async function save() {
  const next = normalizeLabel(label.value)
  if (!next || next === props.edge.label || busy.value) return
  busy.value = true
  try {
    await updateEdge(props.sketchId, props.edge.id, next)
    toast.success('Relationship updated')
    emit('changed')
  } catch (err) {
    toast.error(apiErrorMessage(err))
  } finally {
    busy.value = false
  }
}

async function remove() {
  if (busy.value) return
  const [from, to] = [props.edge.source, props.edge.target].map((id) => end(id)?.label ?? 'unknown')
  if (
    !(await confirm({
      title: 'Delete relationship',
      message: `Delete the relationship ${props.edge.label} between ${from} and ${to}?`,
      variant: 'danger',
    }))
  )
    return
  busy.value = true
  try {
    await deleteEdges(props.sketchId, [props.edge.id])
    emit('changed')
  } catch (err) {
    toast.error(apiErrorMessage(err))
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <div class="panel">
    <div class="text-sm text-muted mb-1">Relationship</div>
    <form v-if="canEdit" class="flex gap-2 mb-4" @submit.prevent="save">
      <input v-model="label" class="form-input code-font" aria-label="Relationship label" />
      <button
        class="btn btn-sm btn-primary"
        :disabled="busy || normalizeLabel(label) === edge.label"
      >
        Save
      </button>
    </form>
    <h3 v-else class="code-font mb-4">{{ edge.label }}</h3>

    <div class="ends">
      <button
        v-for="id in [edge.source, edge.target]"
        :key="id"
        class="end"
        @click="emit('focus', id)"
      >
        <span class="dot" :style="{ background: catalog.color(end(id)?.type ?? '') }"></span>
        <span class="min-w-0">
          <span class="text-xs text-muted"
            >{{ id === edge.source ? 'From' : 'To' }},
            {{ catalog.type(end(id)?.type ?? '')?.title ?? end(id)?.type }}</span
          >
          <span class="label">{{ end(id)?.label ?? 'unknown' }}</span>
        </span>
      </button>
    </div>

    <dl v-if="shown.length || origin" class="facts stacked mt-4">
      <template v-for="[k, v] in shown" :key="k">
        <dt>{{ k }}</dt>
        <dd>{{ v }}</dd>
      </template>
      <template v-if="origin">
        <dt>Added</dt>
        <dd>
          {{ originDate(origin) }}
          <br />
          <span class="text-secondary">{{ describeOrigin(origin) }}</span>
          <a v-if="origin.scan" href="#" class="more" @click.prevent="emit('scan', origin.scan)"
            >scan</a
          >
        </dd>
      </template>
    </dl>

    <button v-if="canEdit" class="btn btn-sm btn-danger mt-4" :disabled="busy" @click="remove">
      Delete the relationship
    </button>
  </div>
</template>

<style scoped>
.panel {
  padding: 16px;
}

.more {
  margin-left: 6px;
  white-space: nowrap;
}

.ends {
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.end {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 8px 10px;
  border: 1px solid var(--border-primary);
  border-radius: var(--radius-md);
  background: none;
  color: var(--text-primary);
  text-align: left;
  cursor: pointer;
}

.end:hover {
  background: var(--bg-hover);
}

.end .label {
  display: block;
  word-break: break-all;
}

.dot {
  width: 10px;
  height: 10px;
  border-radius: 50%;
  flex: none;
}

.min-w-0 {
  min-width: 0;
}

.facts.stacked {
  grid-template-columns: 1fr;
  gap: 2px 0;
  font-size: 13px;
}

.facts.stacked dt {
  font-size: 12px;
  color: var(--text-muted);
}
</style>
