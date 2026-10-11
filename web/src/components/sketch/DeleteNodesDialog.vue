<script setup lang="ts">
// Deleting entities, optionally with what was found through them: the
// entities left hanging only from the deleted ones.
import { computed, onMounted, ref } from 'vue'
import { apiErrorMessage } from '@/api/client'
import { deleteNodes, dependents } from '@/api/sketches'
import type { GraphNode } from '@/api/types'
import BaseModal from '@/components/BaseModal.vue'
import { useCatalogStore } from '@/stores/catalog'
import { useNotificationStore } from '@/stores/notification'

const props = defineProps<{ sketchId: string; ids: string[]; nodes: GraphNode[] }>()
const emit = defineEmits<{ (e: 'close'): void; (e: 'deleted', count: number): void }>()

const catalog = useCatalogStore()
const toast = useNotificationStore()
const found = ref<string[] | null>(null)
const withDependents = ref(false)
const busy = ref(false)

const byId = computed(() => new Map(props.nodes.map((n) => [n.id, n])))
const names = computed(() =>
  props.ids
    .map((i) => byId.value.get(i)?.label)
    .filter(Boolean)
    .slice(0, 3)
    .join(', '),
)
const shown = computed(() =>
  (found.value ?? []).map((i) => byId.value.get(i)).filter((n): n is GraphNode => !!n),
)

async function submit() {
  busy.value = true
  try {
    emit('deleted', await deleteNodes(props.sketchId, props.ids, withDependents.value))
  } catch (err) {
    toast.error(apiErrorMessage(err))
  } finally {
    busy.value = false
  }
}

onMounted(async () => {
  try {
    found.value = await dependents(props.sketchId, props.ids)
  } catch (err) {
    found.value = []
    toast.error(apiErrorMessage(err))
  }
})
</script>

<template>
  <BaseModal title="Delete" size="modal-w480" form @close="emit('close')" @submit="submit">
    <p class="mb-4">
      Delete {{ ids.length === 1 ? names : `${ids.length} entities (${names}...)` }} and the
      relationships touching {{ ids.length === 1 ? 'it' : 'them' }}?
    </p>

    <p v-if="found === null" class="text-sm text-muted">Looking for what was found through it...</p>
    <template v-else-if="found.length">
      <label class="checkbox-label mb-2">
        <input v-model="withDependents" type="checkbox" />
        Also delete the {{ found.length }} entities found through
        {{ ids.length === 1 ? 'it' : 'them' }}
      </label>
      <p class="text-sm text-muted mb-2">
        These hang only from what is deleted and none of them existed before it. Entities also
        connected to the rest of the sketch stay.
      </p>
      <ul class="deps" :class="{ dim: !withDependents }">
        <li v-for="n in shown.slice(0, 50)" :key="n.id">
          <span class="dot" :style="{ background: catalog.color(n.type) }"></span>
          <span class="text-muted">{{ n.type }}</span> {{ n.label }}
        </li>
        <li v-if="shown.length > 50" class="text-muted">and {{ shown.length - 50 }} more</li>
      </ul>
    </template>
    <p v-else class="text-sm text-muted">Nothing else hangs only from it.</p>

    <template #footer>
      <button type="button" class="btn btn-secondary" @click="emit('close')">Cancel</button>
      <button class="btn btn-danger" :disabled="busy || found === null">
        Delete {{ withDependents && found ? ids.length + found.length : ids.length }}
      </button>
    </template>
  </BaseModal>
</template>

<style scoped>
.deps {
  max-height: 220px;
  overflow-y: auto;
  margin: 0;
  padding: 8px 12px;
  list-style: none;
  font-size: 12px;
  border: 1px solid var(--border-primary);
  border-radius: var(--radius-sm);
}

.deps.dim {
  opacity: 0.55;
}

.deps li {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 2px 0;
}

.dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  flex: none;
}
</style>
