<script setup lang="ts">
// The changes the assistant proposed, as cards to apply one by one or
// all at once. Nothing is written until a click: a model's guess is
// cheaper to refuse here than to find in the graph later. A run card is
// a transform or a flow the model wanted to run that only the person
// may start, since it contacts the target or spends a key.
import { computed, ref } from 'vue'
import { applyChanges } from '@/api/chat'
import { apiErrorMessage } from '@/api/client'
import type { GraphNode, ProposedChange } from '@/api/types'
import { useNotificationStore } from '@/stores/notification'

const props = defineProps<{
  sketchId: string
  chatId: string
  changes: ProposedChange[]
  nodes: GraphNode[]
  canEdit: boolean
}>()
const emit = defineEmits<{ (e: 'applied'): void }>()

const toast = useNotificationStore()
const done = ref(new Set<number>())
const busy = ref(false)

const pending = computed(() => props.changes.map((_, i) => i).filter((i) => !done.value.has(i)))

// nameOf shows an entity reference: the label of the id, or the text
// itself when the model wrote a label instead of an id.
function nameOf(ref: string): string {
  const clean = ref.trim().replace(/[,;.\s]+$/, '')
  return props.nodes.find((n) => n.id === clean)?.label ?? clean
}

function title(c: ProposedChange): string {
  if (c.kind === 'set_properties') return `Fill ${nameOf(c.node_id ?? '')}`
  if (c.kind === 'add_entity') return `Add ${c.type ?? 'entity'}`
  if (c.kind === 'run') {
    const n = c.node_ids?.length ?? 0
    const on = `on ${n} ${n === 1 ? 'entity' : 'entities'}`
    return c.steps?.length ? `Run a ${c.steps.length}-step flow ${on}` : `Run ${c.transform} ${on}`
  }
  return `Link ${nameOf(c.source ?? '')} to ${nameOf(c.target ?? '')}`
}

// chain spells a run's flow as its steps in order.
function chain(c: ProposedChange): string {
  return (c.steps ?? []).map((s) => s.transform).join(' \u2192 ')
}

function paramsOf(c: ProposedChange): string {
  return Object.entries(c.params ?? {})
    .map(([k, v]) => `${k}=${v}`)
    .join(', ')
}

// relation is the label as the graph spells it, which apply does too.
function relation(s?: string): string {
  const t = (s ?? '').trim()
  return t ? t.toUpperCase().replace(/[\s-]+/g, '_') : 'RELATED_TO'
}

function current(c: ProposedChange, key: string): string {
  const n = props.nodes.find((x) => x.id === c.node_id)
  const v = n?.properties[key]
  return v === undefined || v === null || v === '' ? '' : String(v)
}

async function apply(indexes: number[]) {
  if (!indexes.length) return
  busy.value = true
  try {
    const result = await applyChanges(
      props.sketchId,
      props.chatId,
      indexes.map((i) => props.changes[i]),
    )
    for (const i of indexes) done.value.add(i)
    done.value = new Set(done.value)
    if (result.errors.length) toast.error(result.errors.join('. '))
    else if (result.launched && result.launched === result.applied)
      toast.success(`${result.launched} ${result.launched === 1 ? 'run' : 'runs'} started`)
    else toast.success(`${result.applied} ${result.applied === 1 ? 'change' : 'changes'} applied`)
    emit('applied')
  } catch (err) {
    toast.error(apiErrorMessage(err))
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <div class="proposal">
    <div class="head text-xs text-muted">
      <span>Proposed {{ changes.length === 1 ? 'change' : 'changes' }}</span>
      <button
        v-if="canEdit && pending.length > 1"
        class="btn btn-sm btn-primary"
        :disabled="busy"
        @click="apply(pending)"
      >
        Apply all
      </button>
    </div>
    <div v-for="(c, i) in changes" :key="i" class="card" :class="{ done: done.has(i) }">
      <div class="card-head">
        <span class="kind">{{ c.kind.replace('_', ' ') }}</span>
        <span class="fw-medium">{{ title(c) }}</span>
        <span v-if="done.has(i)" class="text-xs text-muted">
          {{ c.kind === 'run' ? 'started' : 'applied' }}
        </span>
        <button
          v-else-if="canEdit"
          class="btn btn-sm btn-secondary"
          :disabled="busy"
          @click="apply([i])"
        >
          {{ c.kind === 'run' ? 'Run' : 'Apply' }}
        </button>
      </div>
      <div v-if="c.kind === 'run' && c.steps?.length" class="text-xs text-muted">
        {{ chain(c) }}
      </div>
      <div v-if="c.kind === 'run' && paramsOf(c)" class="text-xs text-muted code-font">
        {{ paramsOf(c) }}
      </div>
      <dl v-if="c.properties && Object.keys(c.properties).length" class="diff">
        <template v-for="(v, k) in c.properties" :key="k">
          <dt>{{ k }}</dt>
          <dd>
            <span
              v-if="
                c.kind === 'set_properties' &&
                current(c, String(k)) &&
                current(c, String(k)) !== String(v)
              "
              class="was"
            >
              {{ current(c, String(k)) }} &rarr;
            </span>
            {{ typeof v === 'object' ? JSON.stringify(v) : String(v) }}
          </dd>
        </template>
      </dl>
      <div v-if="c.kind === 'add_entity' && c.link_to" class="text-xs text-muted">
        linked to {{ nameOf(c.link_to) }} as {{ relation(c.relation) }}
      </div>
      <div v-if="c.kind === 'link'" class="text-xs text-muted">{{ relation(c.label) }}</div>
      <div v-if="c.reason" class="text-xs text-secondary reason">{{ c.reason }}</div>
    </div>
  </div>
</template>

<style scoped>
.proposal {
  margin-top: 8px;
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.head {
  display: flex;
  align-items: center;
  justify-content: space-between;
}

.card {
  padding: 8px 10px;
  border: 1px solid var(--border-primary);
  border-radius: var(--radius-md);
  background: var(--bg-tertiary);
  font-size: 13px;
}

.card.done {
  opacity: 0.6;
}

.card-head {
  display: flex;
  align-items: center;
  gap: 8px;
}

.card-head .btn {
  margin-left: auto;
}

.kind {
  padding: 1px 6px;
  border-radius: var(--radius-sm);
  background: var(--bg-active);
  font-size: 11px;
  text-transform: uppercase;
  letter-spacing: 0.03em;
  color: var(--text-secondary);
}

.diff {
  display: grid;
  grid-template-columns: max-content 1fr;
  gap: 2px 10px;
  margin: 6px 0 0;
}

.diff dt {
  color: var(--text-muted);
}

.diff dd {
  margin: 0;
  word-break: break-word;
}

.was {
  color: var(--text-muted);
  text-decoration: line-through;
  text-decoration-color: var(--danger-fg);
  margin-right: 4px;
}

.reason {
  margin-top: 4px;
}
</style>
