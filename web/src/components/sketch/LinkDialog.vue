<script setup lang="ts">
// Drawing a relationship by hand between two entities of the sketch.
// The other end is found by its label, the direction can be swapped,
// and the label is typed or picked from the ones the sketch and the
// transforms already use.
import { computed, ref } from 'vue'
import { apiErrorMessage } from '@/api/client'
import { addEdge } from '@/api/sketches'
import type { GraphEdge, GraphNode } from '@/api/types'
import BaseModal from '@/components/BaseModal.vue'
import FormField from '@/components/FormField.vue'
import { labelChoices, normalizeLabel } from '@/composables/relationLabels'
import { useCatalogStore } from '@/stores/catalog'
import { useNotificationStore } from '@/stores/notification'

const props = defineProps<{
  sketchId: string
  from: GraphNode
  to?: GraphNode
  nodes: GraphNode[]
  edges: GraphEdge[]
}>()
const emit = defineEmits<{ (e: 'close'): void; (e: 'linked', edge: GraphEdge): void }>()

const catalog = useCatalogStore()
const toast = useNotificationStore()
const source = ref<GraphNode>(props.from)
const target = ref<GraphNode | null>(props.to ?? null)
const query = ref('')
const label = ref('RELATED_TO')
const busy = ref(false)

const matches = computed(() => {
  const q = query.value.trim().toLowerCase()
  if (!q) return []

  return props.nodes
    .filter((n) => n.id !== source.value.id && n.label.toLowerCase().includes(q))
    .slice(0, 8)
})

const labels = computed(() => labelChoices(props.edges.map((e) => e.label)))

function pick(n: GraphNode) {
  target.value = n
  query.value = ''
}

function swap() {
  if (!target.value) return
  ;[source.value, target.value] = [target.value, source.value]
}

async function submit() {
  if (!target.value) return
  busy.value = true
  try {
    const e = await addEdge(
      props.sketchId,
      source.value.id,
      target.value.id,
      normalizeLabel(label.value),
    )
    emit('linked', e)
  } catch (err) {
    toast.error(apiErrorMessage(err))
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <BaseModal title="Link entities" size="modal-w480" form @close="emit('close')" @submit="submit">
    <div class="ends">
      <div class="end">
        <span class="dot" :style="{ background: catalog.color(source.type) }"></span>
        <div>
          <div class="text-xs text-muted">
            {{ catalog.type(source.type)?.title ?? source.type }}
          </div>
          <div class="fw-medium">{{ source.label }}</div>
        </div>
      </div>
      <button
        type="button"
        class="btn btn-sm btn-secondary"
        :disabled="!target"
        title="Swap direction"
        @click="swap"
      >
        &#8597;
      </button>
      <div v-if="target" class="end">
        <span class="dot" :style="{ background: catalog.color(target.type) }"></span>
        <div>
          <div class="text-xs text-muted">
            {{ catalog.type(target.type)?.title ?? target.type }}
          </div>
          <div class="fw-medium">{{ target.label }}</div>
        </div>
        <button
          type="button"
          class="btn btn-sm btn-secondary ml-auto"
          title="Pick another"
          @click="target = null"
        >
          Change
        </button>
      </div>
      <div v-else class="end find">
        <input v-model="query" class="form-input" placeholder="Find the other entity" autofocus />
        <div v-if="matches.length" class="hits">
          <button v-for="m in matches" :key="m.id" type="button" class="hit" @click="pick(m)">
            <span class="dot" :style="{ background: catalog.color(m.type) }"></span>
            <span class="text-muted text-xs">{{ m.type }}</span>
            {{ m.label }}
          </button>
        </div>
        <p v-else-if="query.trim()" class="text-xs text-muted mt-1">No entity matches.</p>
      </div>
    </div>

    <FormField label="Relationship" hint="Reads from the first entity to the second">
      <input v-model="label" class="form-input" list="link-labels" required />
      <datalist id="link-labels">
        <option v-for="l in labels" :key="l" :value="l" />
      </datalist>
    </FormField>

    <template #footer>
      <button type="button" class="btn btn-secondary" @click="emit('close')">Cancel</button>
      <button class="btn btn-primary" :disabled="busy || !target || !label.trim()">Link</button>
    </template>
  </BaseModal>
</template>

<style scoped>
.ends {
  display: flex;
  flex-direction: column;
  gap: 8px;
  margin-bottom: 16px;
}

.ends > .btn {
  align-self: center;
}

.end {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 8px 12px;
  border: 1px solid var(--border-primary);
  border-radius: var(--radius-md);
}

.end.find {
  display: block;
  position: relative;
  padding: 0;
  border: 0;
}

.hits {
  position: absolute;
  left: 0;
  right: 0;
  z-index: var(--z-raised);
  margin-top: 4px;
  background: var(--bg-popover);
  border: 1px solid var(--border-primary);
  border-radius: var(--radius-md);
  box-shadow: var(--shadow-md);
  overflow: hidden;
}

.hit {
  display: flex;
  align-items: center;
  gap: 8px;
  width: 100%;
  padding: 6px 10px;
  background: none;
  border: 0;
  color: var(--text-primary);
  text-align: left;
  cursor: pointer;
}

.hit:hover {
  background: var(--bg-hover);
}

.dot {
  width: 10px;
  height: 10px;
  border-radius: 50%;
  flex: none;
}
</style>
