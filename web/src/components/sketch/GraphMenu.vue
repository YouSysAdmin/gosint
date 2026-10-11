<script setup lang="ts">
// The right-click menu of the graph: on an entity, on a selection, on a
// relationship, or on the empty canvas. It floats at the cursor, stays
// inside the window and closes on a click outside or Escape.
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { listTransforms, listFlows } from '@/api/catalog'
import { apiErrorMessage } from '@/api/client'
import {
  deleteEdges,
  launchTransform,
  launchFlow,
  launchTemplate,
  updateEdge,
} from '@/api/sketches'
import type { Transform, Flow, GraphEdge, GraphNode } from '@/api/types'
import type { Relatives } from '@/composables/graphSelect'
import { useCatalogStore } from '@/stores/catalog'
import { useNotificationStore } from '@/stores/notification'

export type MenuKind = 'node' | 'selection' | 'edge' | 'background'

const props = defineProps<{
  kind: MenuKind
  x: number
  y: number
  sketchId: string
  nodes: GraphNode[]
  edge?: GraphEdge
  allNodes: GraphNode[]
  canEdit: boolean
}>()

const emit = defineEmits<{
  (e: 'close'): void
  (e: 'launched'): void
  (e: 'configure', transform: Transform): void
  (e: 'addRelation', node: GraphNode): void
  (e: 'linkTo', node: GraphNode): void
  (e: 'clone', node: GraphNode): void
  (e: 'askAi'): void
  (e: 'flag', node: GraphNode, color: string): void
  (e: 'delete'): void
  (e: 'open', node: GraphNode): void
  (e: 'add'): void
  (e: 'import'): void
  (e: 'edgeChanged'): void
  (e: 'selectRelatives', how: Relatives): void
  (e: 'selectType', type: string): void
  (e: 'selectAll'): void
  (e: 'invert'): void
  (e: 'path', a: GraphNode, b: GraphNode): void
}>()

const FLAGS = ['red', 'orange', 'yellow', 'green', 'blue']
const FLAG_COLORS: Record<string, string> = {
  red: '#ef4444',
  orange: '#f97316',
  yellow: '#eab308',
  green: '#22c55e',
  blue: '#3b82f6',
}

const catalog = useCatalogStore()
const toast = useNotificationStore()
const el = ref<HTMLElement | null>(null)
const height = ref(360)
let sizer: ResizeObserver | null = null
const transforms = ref<Transform[]>([])
const flows = ref<Flow[]>([])
const loading = ref(false)
const tab = ref<'transforms' | 'flows'>('transforms')
const filter = ref('')
const edgeLabel = ref(props.edge?.label ?? '')

const node = computed(() => (props.kind === 'node' ? props.nodes[0] : null))
const type = computed(() => {
  const first = props.nodes[0]?.type
  return first && props.nodes.every((n) => n.type === first) ? first : ''
})
const ids = computed(() => props.nodes.map((n) => n.id))

const shownTransforms = computed(() => {
  const q = filter.value.toLowerCase()
  return transforms.value.filter(
    (e) => !q || `${e.name} ${e.description}`.toLowerCase().includes(q),
  )
})
const shownFlows = computed(() => {
  const q = filter.value.toLowerCase()
  return flows.value.filter((f) => !q || f.name.toLowerCase().includes(q))
})

function needsParams(e: Transform): boolean {
  return (e.params ?? []).some((p) => p.type !== 'vaultSecret')
}

function needsKey(e: Transform): boolean {
  return (e.params ?? []).some((p) => p.type === 'vaultSecret' && p.required)
}

async function run(e: Transform) {
  if (needsParams(e)) {
    emit('configure', e)
    return
  }
  try {
    if (e.template_id) await launchTemplate(props.sketchId, e.template_id, ids.value, {})
    else await launchTransform(props.sketchId, e.name, ids.value, {})
    toast.success(`${e.name} started on ${ids.value.length} ${type.value}`)
    emit('launched')
  } catch (err) {
    toast.error(apiErrorMessage(err))
  }
}

async function runFlow(f: Flow) {
  try {
    await launchFlow(props.sketchId, f.id, ids.value)
    toast.success(`flow ${f.name} started`)
    emit('launched')
  } catch (err) {
    toast.error(apiErrorMessage(err))
  }
}

async function copy(text: string) {
  try {
    await navigator.clipboard.writeText(text)
    toast.success('Copied')
  } catch {
    toast.error('The clipboard is not available')
  }
  emit('close')
}

async function saveEdgeLabel() {
  if (!props.edge) return
  const label = edgeLabel.value.trim().toUpperCase().replace(/\s+/g, '_')
  if (!label || label === props.edge.label) {
    emit('close')
    return
  }
  try {
    await updateEdge(props.sketchId, props.edge.id, label)
    toast.success('Relationship updated')
    emit('edgeChanged')
    emit('close')
  } catch (err) {
    toast.error(apiErrorMessage(err))
  }
}

async function removeEdge() {
  if (!props.edge) return
  try {
    await deleteEdges(props.sketchId, [props.edge.id])
    emit('edgeChanged')
    emit('close')
  } catch (err) {
    toast.error(apiErrorMessage(err))
  }
}

// place keeps the menu inside the window. The height is watched, the
// transform list arrives after the menu opened and makes it taller.
const place = computed(() => {
  const w = 320
  const x = Math.min(props.x, window.innerWidth - w - 8)
  const y = Math.min(props.y, window.innerHeight - height.value - 8)
  return { left: Math.max(8, x) + 'px', top: Math.max(8, y) + 'px' }
})

function away(ev: MouseEvent) {
  if (el.value && !el.value.contains(ev.target as Node)) emit('close')
}

function key(ev: KeyboardEvent) {
  if (ev.key === 'Escape') emit('close')
}

async function loadCatalog() {
  if (!props.canEdit || !type.value || (props.kind !== 'node' && props.kind !== 'selection')) return
  loading.value = true
  try {
    const [e, f] = await Promise.all([listTransforms(type.value), listFlows(type.value)])
    transforms.value = e
    flows.value = f
  } catch (err) {
    toast.error(apiErrorMessage(err))
  } finally {
    loading.value = false
  }
}

watch(() => props.nodes, loadCatalog)

onMounted(() => {
  // Deferred, so the click that opened the menu does not close it. In
  // the capture phase, since the canvas stops a mousedown from
  // bubbling and the menu would never see it.
  setTimeout(() => {
    document.addEventListener('mousedown', away, true)
    document.addEventListener('keydown', key)
  }, 0)
  if (el.value) {
    sizer = new ResizeObserver(() => {
      if (el.value) height.value = el.value.offsetHeight
    })
    sizer.observe(el.value)
  }
  void loadCatalog()
  // autofocus fires once per document, so the first field is focused
  // by hand on every open.
  void nextTick(() => first.value?.focus())
})

// first is the field the menu opens on: the search, or the label.
const first = ref<HTMLInputElement | null>(null)

onBeforeUnmount(() => {
  sizer?.disconnect()
  document.removeEventListener('mousedown', away, true)
  document.removeEventListener('keydown', key)
})
</script>

<template>
  <div ref="el" class="menu" :style="place" @contextmenu.prevent>
    <!-- Entity or selection -->
    <template v-if="kind === 'node' || kind === 'selection'">
      <div class="head">
        <span
          class="dot"
          :style="{ background: type ? catalog.color(type) : 'transparent' }"
        ></span>
        <span class="title">
          <template v-if="node">{{ node.label }}</template>
          <template v-else>{{ nodes.length }} selected</template>
        </span>
        <span class="text-xs text-muted">{{
          type ? (catalog.type(type)?.title ?? type) : 'mixed types'
        }}</span>
        <button v-if="node" class="icon" title="Copy the label" @click="copy(node.label)">
          &#10697;
        </button>
        <button v-if="node" class="icon" title="Open details" @click="emit('open', node)">
          &#8599;
        </button>
      </div>

      <div v-if="canEdit" class="actions">
        <button v-if="node" class="menu-item" @click="emit('addRelation', node)">
          Add a related entity
        </button>
        <button v-if="node" class="menu-item" @click="emit('linkTo', node)">Link to...</button>
        <button v-if="node" class="menu-item" @click="emit('clone', node)">Clone</button>
        <button class="menu-item" @click="emit('askAi')">Ask the assistant</button>
        <div v-if="node" class="flags">
          <span class="text-xs text-muted">Flag</span>
          <button
            v-for="f in FLAGS"
            :key="f"
            class="flag"
            :class="{ on: node.flag === f }"
            :style="{ background: FLAG_COLORS[f] }"
            :title="node.flag === f ? 'Remove the flag' : 'Flag ' + f"
            @click="emit('flag', node, node.flag === f ? '' : f)"
          ></button>
        </div>
        <button class="menu-item danger" @click="emit('delete')">
          Delete{{ node ? '' : ` ${nodes.length}` }}
        </button>
      </div>

      <div class="pick">
        <span class="text-xs text-muted">Select</span>
        <button
          class="chip"
          title="Every entity one relationship away"
          @click="emit('selectRelatives', 'neighbours')"
        >
          neighbours
        </button>
        <button
          class="chip"
          title="What relationships come from"
          @click="emit('selectRelatives', 'parents')"
        >
          parents
        </button>
        <button
          class="chip"
          title="What relationships lead to"
          @click="emit('selectRelatives', 'children')"
        >
          children
        </button>
        <button
          class="chip"
          title="Everything joined to this"
          @click="emit('selectRelatives', 'connected')"
        >
          connected
        </button>
        <button
          v-if="type"
          class="chip"
          :title="'Every ' + type + ' in the sketch'"
          @click="emit('selectType', type)"
        >
          same type
        </button>
        <button
          v-if="nodes.length === 2"
          class="chip"
          title="The fewest relationships joining the two"
          @click="emit('path', nodes[0], nodes[1])"
        >
          path between
        </button>
      </div>

      <template v-if="canEdit && type">
        <div class="tabs small">
          <button class="tab" :class="{ active: tab === 'transforms' }" @click="tab = 'transforms'">
            Transforms
          </button>
          <button class="tab" :class="{ active: tab === 'flows' }" @click="tab = 'flows'">
            Flows<span v-if="flows.length" class="text-muted">&nbsp;{{ flows.length }}</span>
          </button>
        </div>
        <input
          v-model="filter"
          class="form-input filter"
          :placeholder="tab === 'transforms' ? 'Search transforms' : 'Search flows'"
          ref="first"
        />
        <div class="list">
          <p v-if="loading" class="text-xs text-muted pad">Loading...</p>
          <template v-else-if="tab === 'transforms'">
            <button
              v-for="e in shownTransforms"
              :key="e.name + e.template_id"
              class="row"
              :title="e.description"
              @click="run(e)"
            >
              <span class="row-name">
                {{ e.name }}
                <span v-if="needsParams(e)" class="text-muted" title="Asks for parameters"
                  >&#9881;</span
                >
                <span v-if="needsKey(e)" class="badge badge-neutral">key</span>
                <span v-if="e.template_id" class="badge badge-secondary">template</span>
                <span v-if="e.file" class="badge badge-secondary" :title="e.file">file</span>
              </span>
              <span class="row-desc text-muted">{{ e.description }}</span>
            </button>
            <p v-if="!shownTransforms.length" class="text-xs text-muted pad">
              No transform takes a {{ type }}.
            </p>
          </template>
          <template v-else>
            <button v-for="f in shownFlows" :key="f.id" class="row" @click="runFlow(f)">
              <span class="row-name">{{ f.name }}</span>
              <span class="row-desc text-muted">{{ f.definition.steps.length }} steps</span>
            </button>
            <p v-if="!shownFlows.length" class="text-xs text-muted pad">
              No flow starts from a {{ type }}.
            </p>
          </template>
        </div>
      </template>
      <p v-else-if="canEdit && !type" class="text-xs text-muted pad">
        Transforms need a selection of one type.
      </p>

      <div v-if="kind === 'selection'" class="foot">
        <button class="menu-item" @click="copy(nodes.map((n) => n.label).join('\n'))">
          Copy {{ nodes.length }} labels as text
        </button>
        <slot name="export" />
      </div>
    </template>

    <!-- Relationship -->
    <template v-else-if="kind === 'edge' && edge">
      <div class="head">
        <span class="title">Relationship</span>
        <button class="icon" title="Copy the label" @click="copy(edge.label)">&#10697;</button>
      </div>
      <form v-if="canEdit" class="edge-form" @submit.prevent="saveEdgeLabel">
        <input
          ref="first"
          v-model="edgeLabel"
          class="form-input"
          @focus="($event.target as HTMLInputElement).select()"
        />
        <button class="btn btn-sm btn-primary">Save</button>
      </form>
      <p v-else class="pad code-font">{{ edge.label }}</p>
      <div v-if="canEdit" class="actions">
        <button class="menu-item danger" @click="removeEdge">Delete the relationship</button>
      </div>
    </template>

    <!-- Empty canvas -->
    <template v-else>
      <div class="actions">
        <button class="menu-item" @click="emit('selectAll')">Select everything shown</button>
        <button class="menu-item" @click="emit('invert')">Invert the selection</button>
      </div>
      <div v-if="canEdit" class="actions">
        <button class="menu-item" @click="emit('add')">Add an entity here</button>
        <button class="menu-item" @click="emit('import')">Import entities</button>
      </div>
      <p v-else class="text-xs text-muted pad">View only</p>
    </template>
  </div>
</template>

<style scoped>
.menu {
  position: fixed;
  z-index: 50;
  width: 320px;
  max-height: calc(100vh - 16px);
  display: flex;
  flex-direction: column;
  background: var(--bg-popover);
  border: 1px solid var(--border-primary);
  border-radius: var(--radius-md);
  box-shadow: var(--shadow-lg);
  font-size: 13px;
  overflow: hidden;
}

.head {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 10px 12px 8px;
  border-bottom: 1px solid var(--border-primary);
}

.title {
  font-weight: 600;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  min-width: 0;
}

.dot {
  width: 10px;
  height: 10px;
  border-radius: 50%;
  flex: none;
}

.icon {
  margin-left: auto;
  border: 0;
  background: none;
  color: var(--text-muted);
  cursor: pointer;
  font-size: 14px;
  padding: 0 4px;
}

.icon + .icon {
  margin-left: 0;
}

.icon:hover {
  color: var(--text-primary);
}

.actions,
.foot {
  display: flex;
  flex-direction: column;
  padding: 4px;
}

.foot {
  border-top: 1px solid var(--border-primary);
}

.menu-item {
  padding: 6px 10px;
  text-align: left;
  border: 0;
  border-radius: var(--radius-sm);
  background: none;
  color: var(--text-primary);
  cursor: pointer;
  font-family: inherit;
  font-size: 13px;
}

.menu-item:hover {
  background: var(--bg-hover);
}

.menu-item.danger {
  color: var(--danger-fg);
}

.flags {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 6px 10px;
}

.pick {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 4px 6px;
  padding: 6px 10px;
  border-top: 1px solid var(--border-primary);
}

.chip {
  padding: 2px 8px;
  border: 1px solid var(--border-primary);
  border-radius: 999px;
  background: none;
  color: var(--text-primary);
  font: inherit;
  font-size: 12px;
  cursor: pointer;
}

.chip:hover {
  background: var(--bg-hover);
}

.flag {
  width: 14px;
  height: 14px;
  border-radius: 50%;
  border: 2px solid transparent;
  cursor: pointer;
  opacity: 0.8;
}

.flag.on {
  border-color: var(--text-primary);
  opacity: 1;
}

.tabs.small {
  margin: 4px 8px 0;
  padding: 2px;
}

.tabs.small .tab {
  padding: 3px 10px;
  font-size: 12px;
}

.filter {
  margin: 6px 8px;
  width: auto;
  font-size: 12px;
  padding: 4px 8px;
}

.list {
  max-height: 260px;
  overflow-y: auto;
  padding: 0 4px 4px;
}

.row {
  display: flex;
  flex-direction: column;
  width: 100%;
  padding: 6px 8px;
  text-align: left;
  border: 0;
  border-radius: var(--radius-sm);
  background: none;
  color: var(--text-primary);
  cursor: pointer;
  font-family: inherit;
}

.row:hover {
  background: var(--bg-hover);
}

.row-name {
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: 13px;
}

.row-desc {
  font-size: 11px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.pad {
  padding: 8px 12px;
  margin: 0;
}

.edge-form {
  display: flex;
  gap: 6px;
  padding: 8px 12px;
}
</style>
