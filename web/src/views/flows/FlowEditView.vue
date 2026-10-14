<script setup lang="ts">
// The flow editor: a canvas of the step tree. The input node is the
// root, a wire from a node into a step makes that step run after it.
// A step has one parent, so a new wire into it replaces the old one.
import { Background } from '@vue-flow/background'
import { Controls } from '@vue-flow/controls'
import {
  Handle,
  Position,
  VueFlow,
  useVueFlow,
  type Connection,
  type Edge,
  type EdgeChange,
  type Node,
  type NodeChange,
  type NodeDragEvent,
} from '@vue-flow/core'
import '@vue-flow/core/dist/style.css'
import '@vue-flow/core/dist/theme-default.css'
import '@vue-flow/controls/dist/style.css'
import { computed, onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { apiErrorMessage } from '@/api/client'
import { checkFlow, deleteFlow, getFlow, listTransforms, saveFlow } from '@/api/catalog'
import type { Transform, FlowStep } from '@/api/types'
import FormField from '@/components/FormField.vue'
import EntityIcon from '@/components/sketch/EntityIcon.vue'
import LoadingBlock from '@/components/LoadingBlock.vue'
import PageHeader from '@/components/PageHeader.vue'
import { useConfirm } from '@/composables/useConfirm'
import { useCatalogStore } from '@/stores/catalog'
import { useNotificationStore } from '@/stores/notification'

const ROOT = 'input'
const COL = 280
const ROW = 110

interface XY {
  x: number
  y: number
}

const route = useRoute()
const router = useRouter()
const toast = useNotificationStore()
const catalog = useCatalogStore()
const { confirm } = useConfirm()

const id = computed(() => (route.params.id ? String(route.params.id) : null))
const loading = ref(true)
const name = ref('')
const description = ref('')
const input = ref('domain')
const steps = ref<FlowStep[]>([])
const positions = ref<Record<string, XY>>({})
const transforms = ref<Transform[]>([])
const selected = ref<string>(ROOT)
const selectedEdge = ref('')
const problem = ref('')
const saving = ref(false)
const paletteSearch = ref('')
const { screenToFlowCoordinate } = useVueFlow()

const builtins = computed(() => transforms.value.filter((e) => !e.template_id))
const byName = computed(() => new Map(builtins.value.map((e) => [e.name, e])))
const byId = computed(() => new Map(steps.value.map((s) => [s.id, s])))
const current = computed(() => byId.value.get(selected.value) ?? null)

// outputOf is the type a step produces, the input type for the root.
function outputOf(stepId: string): string {
  if (stepId === ROOT) return input.value
  const s = byId.value.get(stepId)

  return (s && byName.value.get(s.transform)?.output) ?? ''
}

function choicesFor(step: FlowStep) {
  const from = outputOf(step.after)
  if (!from) return builtins.value

  return builtins.value.filter((e) => !e.input || e.input === from)
}

// fits is false for a step whose transform does not take what its
// parent gives, which happens when a wire or the input type changes.
function fits(step: FlowStep): boolean {
  const e = byName.value.get(step.transform)
  if (!e || !step.after) return false

  const from = outputOf(step.after)

  return !e.input || !from || e.input === from
}

function subtitle(step: FlowStep): string {
  const e = byName.value.get(step.transform)
  if (!e) return step.id

  return `${step.id}, ${e.input || 'any'} to ${e.output || '?'}`
}

// palette groups the transforms by category for the left column, the
// ones taking the selected step's output first.
const palette = computed(() => {
  const q = paletteSearch.value.trim().toLowerCase()
  const from = outputOf(selected.value)
  const groups = new Map<string, Transform[]>()
  for (const e of builtins.value) {
    if (q && !`${e.name} ${e.description} ${e.input} ${e.output}`.toLowerCase().includes(q))
      continue
    const key = e.category || 'Other'
    groups.set(key, [...(groups.get(key) ?? []), e])
  }

  return [...groups.entries()]
    .map(([category, items]) => ({
      category,
      items: items.sort((a, b) => a.name.localeCompare(b.name)),
      fitting: items.some((e) => !e.input || e.input === from),
    }))
    .sort((a, b) => Number(b.fitting) - Number(a.fitting) || a.category.localeCompare(b.category))
})

function takesSelected(e: Transform): boolean {
  const from = outputOf(selected.value)
  return !from || !e.input || e.input === from
}

function keyParams(e: Transform): string[] {
  return (e.params ?? []).filter((p) => p.type === 'vaultSecret').map((p) => p.name)
}

function dragTransform(ev: DragEvent, e: Transform) {
  ev.dataTransfer?.setData('application/x-gosint-transform', e.name)
  if (ev.dataTransfer) ev.dataTransfer.effectAllowed = 'copy'
}

// addTransform adds a step running e. Clicked, it goes after the
// selected node when e takes what that node gives. Dropped, it lands
// where it was dropped and is wired to the selected node on the same
// condition, or left for a wire.
function addTransform(e: Transform, at?: XY) {
  const parent = takesSelected(e) && outputOf(selected.value) ? selected.value : ''
  counter++
  while (steps.value.some((s) => s.id === `s${counter}`)) counter++
  const s: FlowStep = { id: `s${counter}`, transform: e.name, after: parent, params: {} }
  positions.value[s.id] = at ?? autoPlace(s.id, parent || ROOT)
  steps.value.push(s)
  selected.value = s.id
}

function onDrop(ev: DragEvent) {
  const name = ev.dataTransfer?.getData('application/x-gosint-transform')
  const e = name ? byName.value.get(name) : undefined
  if (!e) return
  ev.preventDefault()
  const p = screenToFlowCoordinate({ x: ev.clientX, y: ev.clientY })
  addTransform(e, { x: p.x - 100, y: p.y - 20 })
}

function editableParams(step: FlowStep) {
  return (byName.value.get(step.transform)?.params ?? []).filter((p) => p.type !== 'vaultSecret')
}

// isBelow tells whether node is in the subtree of root, which a new
// wire from node into root would turn into a cycle.
function isBelow(node: string, root: string): boolean {
  let at = byId.value.get(node)
  const seen = new Set<string>()
  while (at && !seen.has(at.id)) {
    if (at.after === root) return true
    seen.add(at.id)
    at = byId.value.get(at.after)
  }

  return false
}

// autoPlace puts a node without a saved position right of its parent,
// below the siblings already placed.
function autoPlace(stepId: string, after: string): XY {
  const parent = positions.value[after] ?? { x: 0, y: 0 }
  const taken = steps.value
    .filter((s) => s.after === after && s.id !== stepId && positions.value[s.id])
    .map((s) => positions.value[s.id].y)
  const y = taken.length ? Math.max(...taken) + ROW : parent.y

  return { x: parent.x + COL, y }
}

function layoutAll() {
  positions.value = { [ROOT]: positions.value[ROOT] ?? { x: 0, y: 0 } }
  const pending = [...steps.value]
  let progress = true
  while (pending.length && progress) {
    progress = false
    for (let i = 0; i < pending.length; i++) {
      const s = pending[i]
      if (s.after && !positions.value[s.after] && s.after !== ROOT) continue
      positions.value[s.id] = autoPlace(s.id, s.after || ROOT)
      pending.splice(i--, 1)
      progress = true
    }
  }

  for (const s of pending) positions.value[s.id] = autoPlace(s.id, ROOT)
}

const nodes = computed<Node[]>(() => [
  {
    id: ROOT,
    type: 'source',
    position: positions.value[ROOT] ?? { x: 0, y: 0 },
    data: {},
    selected: selected.value === ROOT,
  },
  ...steps.value.map((s) => ({
    id: s.id,
    type: 'step',
    position: positions.value[s.id] ?? { x: COL, y: 0 },
    data: { step: s },
    selected: selected.value === s.id,
  })),
])

const edges = computed<Edge[]>(() =>
  steps.value
    .filter((s) => s.after)
    .map((s) => ({
      id: `${s.after}->${s.id}`,
      source: s.after,
      target: s.id,
      animated: false,
      class: fits(s) ? '' : 'edge-bad',
      selected: selectedEdge.value === `${s.after}->${s.id}`,
    })),
)

function pick(nodeId: string) {
  selected.value = nodeId
  selectedEdge.value = ''
}

function pickEdge(edgeId: string) {
  selectedEdge.value = edgeId
}

let counter = 0
function addStep(after = ROOT) {
  counter++
  while (steps.value.some((s) => s.id === `s${counter}`)) counter++
  const s: FlowStep = { id: `s${counter}`, transform: '', after, params: {} }
  positions.value[s.id] = autoPlace(s.id, after)
  steps.value.push(s)
  selected.value = s.id
}

// removeStep hands the step's children to its own parent, so cutting a
// link out of a chain keeps the rest of the chain.
function removeStep(stepId: string) {
  const gone = steps.value.find((s) => s.id === stepId)
  if (!gone) return
  steps.value = steps.value.filter((s) => s.id !== stepId)
  for (const s of steps.value) if (s.after === stepId) s.after = gone.after
  delete positions.value[stepId]
  if (selected.value === stepId) selected.value = ROOT
}

function onConnect(c: Connection) {
  const child = steps.value.find((s) => s.id === c.target)
  if (!child || c.source === c.target) return
  if (isBelow(c.source, child.id)) {
    toast.error(`${c.source} runs after ${child.id} already`)
    return
  }

  child.after = c.source
  selected.value = child.id
}

// A position change during a drag is left to the canvas, which moves
// the node itself, and the spot is recorded once the drag ends: writing
// it on every frame rebuilt every node and wire. A keyboard nudge comes
// as a change that is not dragging.
function onNodesChange(changes: NodeChange[]) {
  for (const ch of changes) {
    if (ch.type === 'position') {
      if (!ch.dragging && ch.position) positions.value[ch.id] = { ...ch.position }
    } else if (ch.type === 'remove' && ch.id !== ROOT) removeStep(ch.id)
  }
}

function onDragStop({ nodes: moved }: NodeDragEvent) {
  for (const n of moved) positions.value[n.id] = { x: n.position.x, y: n.position.y }
}

// A removed wire leaves the step unconnected until it is wired again.
function onEdgesChange(changes: EdgeChange[]) {
  for (const ch of changes) {
    if (ch.type !== 'remove') continue
    const s = steps.value.find((x) => `${x.after}->${x.id}` === ch.id)
    if (s) s.after = ''
  }
}

const definition = computed(() => ({
  input: input.value,
  steps: steps.value,
  layout: { positions: positions.value },
}))

async function check(): Promise<boolean> {
  const loose = steps.value.filter((s) => !s.after).map((s) => s.id)
  if (loose.length) {
    problem.value = `Not connected: ${loose.join(', ')}`
    return false
  }

  try {
    await checkFlow(definition.value)
    problem.value = ''
    return true
  } catch (err) {
    problem.value = apiErrorMessage(err)
    return false
  }
}

async function save() {
  if (saving.value) return
  saving.value = true
  try {
    if (!(await check())) return
    const f = await saveFlow(id.value, name.value, description.value, definition.value)
    toast.success('Flow saved')
    if (!id.value) await router.replace(`/flows/${f.id}`)
  } catch (err) {
    toast.error(apiErrorMessage(err))
  } finally {
    saving.value = false
  }
}

async function remove() {
  if (
    !id.value ||
    !(await confirm({ title: 'Delete flow', message: `Delete ${name.value}?`, variant: 'danger' }))
  )
    return
  try {
    await deleteFlow(id.value)
    await router.push('/flows')
  } catch (err) {
    toast.error(apiErrorMessage(err))
  }
}

// savedPositions reads the layout back, ignoring anything malformed.
function savedPositions(layout: Record<string, unknown> | null): Record<string, XY> {
  const raw = layout?.positions
  if (!raw || typeof raw !== 'object') return {}
  const out: Record<string, XY> = {}
  for (const [k, v] of Object.entries(raw as Record<string, unknown>)) {
    const p = v as Partial<XY>
    if (typeof p?.x === 'number' && typeof p?.y === 'number') out[k] = { x: p.x, y: p.y }
  }

  return out
}

onMounted(async () => {
  try {
    await catalog.load()
    transforms.value = await listTransforms()
    if (id.value) {
      const f = await getFlow(id.value)
      name.value = f.name
      description.value = f.description
      input.value = f.definition.input
      steps.value = f.definition.steps.map((s) => ({ ...s, params: s.params ?? {} }))
      positions.value = savedPositions(f.definition.layout)
      if (steps.value.some((s) => !positions.value[s.id])) layoutAll()
    } else {
      positions.value = { [ROOT]: { x: 0, y: 0 } }
      addStep()
    }
  } catch (err) {
    toast.error(apiErrorMessage(err))
  } finally {
    loading.value = false
  }
})
</script>

<template>
  <LoadingBlock v-if="loading" />
  <template v-else>
    <PageHeader :title="id ? name || 'Flow' : 'New flow'">
      <button v-if="id" class="btn btn-danger" @click="remove">Delete</button>
      <button class="btn btn-secondary" @click="check">Check</button>
      <button class="btn btn-primary" :disabled="saving" @click="save">Save</button>
    </PageHeader>

    <div class="card mb-4">
      <div class="card-body form-row">
        <FormField label="Name" required>
          <input v-model="name" class="form-input" required />
        </FormField>
        <FormField label="Description">
          <input v-model="description" class="form-input" />
        </FormField>
      </div>
    </div>

    <div v-if="problem" class="notice notice-danger mb-4">{{ problem }}</div>

    <div class="editor">
      <aside class="card palette">
        <div class="card-body palette-body">
          <input v-model="paletteSearch" class="form-input mb-2" placeholder="Search transforms" />
          <p class="text-xs text-muted mb-2">
            Click one to add it after the selected step, or drag it onto the canvas.
          </p>
          <div class="palette-list">
            <template v-for="g in palette" :key="g.category">
              <div class="group text-xs text-muted">{{ g.category }}</div>
              <button
                v-for="e in g.items"
                :key="e.name"
                class="item"
                :class="{ dim: !takesSelected(e) }"
                draggable="true"
                :title="e.description"
                @click="addTransform(e)"
                @dragstart="dragTransform($event, e)"
              >
                <span class="item-name">{{ e.name }}</span>
                <span class="item-io text-xs text-muted">
                  <EntityIcon v-if="e.input" :type="e.input" :size="11" />{{
                    e.input || 'any'
                  }}
                  &rarr; <EntityIcon :type="e.output" :size="11" />{{ e.output }}
                  <span v-if="keyParams(e).length" class="badge badge-neutral">key</span>
                </span>
              </button>
            </template>
            <p v-if="!palette.length" class="text-xs text-muted">No transform matches.</p>
          </div>
        </div>
      </aside>
      <div class="canvas" @dragover.prevent @drop="onDrop">
        <VueFlow
          :nodes="nodes"
          :edges="edges"
          :apply-default="false"
          :min-zoom="0.3"
          :max-zoom="2"
          fit-view-on-init
          @connect="onConnect"
          @nodes-change="onNodesChange"
          @node-drag-stop="onDragStop"
          @edges-change="onEdgesChange"
          @node-click="({ node }) => pick(node.id)"
          @edge-click="({ edge }) => pickEdge(edge.id)"
          @pane-click="pick(ROOT)"
        >
          <template #node-source>
            <div class="fnode fnode-source" :class="{ picked: selected === ROOT }">
              <span class="dot" :style="{ background: catalog.color(input) }"></span>
              <div>
                <div class="fnode-title">Input</div>
                <div class="fnode-sub">{{ catalog.type(input)?.title ?? input }}</div>
              </div>
              <Handle type="source" :position="Position.Right" />
            </div>
          </template>

          <template #node-step="{ data }">
            <div
              class="fnode"
              :class="{
                picked: selected === data.step.id,
                bad: data.step.transform && !fits(data.step),
              }"
            >
              <Handle type="target" :position="Position.Left" />
              <span
                class="dot"
                :style="{ background: catalog.color(outputOf(data.step.id)) }"
              ></span>
              <div>
                <div class="fnode-title">{{ data.step.transform || 'Pick a transform' }}</div>
                <div class="fnode-sub">{{ subtitle(data.step) }}</div>
              </div>
              <Handle type="source" :position="Position.Right" />
            </div>
          </template>

          <Background :gap="20" />
          <Controls :show-interactive="false" />
        </VueFlow>
      </div>

      <aside class="card panel">
        <div class="card-body">
          <template v-if="!current">
            <h3 class="mb-4">Input</h3>
            <FormField label="Starts from">
              <select v-model="input" class="form-select">
                <option v-for="t in catalog.types" :key="t.name" :value="t.name">
                  {{ t.title }}
                </option>
              </select>
            </FormField>
            <div class="flex gap-2 mb-4">
              <button class="btn btn-sm btn-secondary" @click="addStep(ROOT)">Add a step</button>
              <button class="btn btn-sm btn-secondary" @click="layoutAll">Tidy layout</button>
            </div>
            <p class="text-sm text-muted">
              Drag from a node's right edge onto a step to make it run after that node. Select a
              wire and press Backspace to cut it. The secrets a transform needs are the ones of
              whoever runs the flow.
            </p>
          </template>

          <template v-else>
            <h3 class="mb-4">Step {{ current.id }}</h3>
            <p v-if="!current.after" class="notice notice-danger mb-4">
              Not connected. Wire it from the input or another step.
            </p>
            <FormField :label="`Transform, on ${outputOf(current.after) || '?'}`">
              <select v-model="current.transform" class="form-select">
                <option value="" disabled>Pick one</option>
                <option v-for="e in choicesFor(current)" :key="e.name" :value="e.name">
                  {{ e.name }}, gives {{ e.output }}
                </option>
              </select>
            </FormField>
            <div v-if="current.transform && byName.get(current.transform)" class="about mb-4">
              <p class="text-sm text-secondary">{{ byName.get(current.transform)?.description }}</p>
              <p class="text-xs text-muted">
                Takes {{ byName.get(current.transform)?.input || 'anything' }}, gives
                {{ byName.get(current.transform)?.output }}.
                <template v-if="keyParams(byName.get(current.transform)!).length">
                  Needs the secret{{
                    keyParams(byName.get(current.transform)!).length > 1 ? 's' : ''
                  }}
                  <span class="code-font">{{
                    keyParams(byName.get(current.transform)!).join(', ')
                  }}</span>
                  of whoever runs the flow.
                </template>
              </p>
            </div>
            <FormField
              v-for="p in editableParams(current)"
              :key="p.name"
              :label="p.name"
              :hint="p.description"
            >
              <select
                v-if="p.type === 'bool'"
                :value="current.params![p.name] ?? ''"
                class="form-select"
                @change="current.params![p.name] = ($event.target as HTMLSelectElement).value"
              >
                <option value="">default ({{ p.default === 'true' ? 'yes' : 'no' }})</option>
                <option value="true">yes</option>
                <option value="false">no</option>
              </select>
              <select
                v-else-if="p.type === 'select'"
                :value="current.params![p.name] ?? ''"
                class="form-select"
                @change="current.params![p.name] = ($event.target as HTMLSelectElement).value"
              >
                <option value="">default ({{ p.default }})</option>
                <option v-for="o in p.options ?? []" :key="o.value" :value="o.value">
                  {{ o.label }}
                </option>
              </select>
              <input
                v-else
                v-model="current.params![p.name]"
                class="form-input"
                :placeholder="p.default"
              />
            </FormField>
            <div class="flex gap-2">
              <button class="btn btn-sm btn-secondary" @click="addStep(current.id)">
                Add a step after
              </button>
              <button class="btn btn-sm btn-secondary" @click="removeStep(current.id)">
                Remove
              </button>
            </div>
          </template>
        </div>
      </aside>
    </div>
  </template>
</template>

<style scoped>
.editor {
  display: grid;
  grid-template-columns: 260px 1fr 320px;
  gap: 16px;
  height: calc(100vh - 300px);
  min-height: 420px;
}

.palette {
  overflow: hidden;
  display: flex;
  flex-direction: column;
}

.palette-body {
  display: flex;
  flex-direction: column;
  min-height: 0;
  height: 100%;
}

.palette-list {
  flex: 1;
  min-height: 0;
  overflow-y: auto;
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.group {
  margin-top: 8px;
}

.item {
  display: flex;
  flex-direction: column;
  gap: 2px;
  padding: 6px 8px;
  border: 1px solid var(--border-primary);
  border-radius: var(--radius-sm);
  background: var(--bg-primary);
  color: var(--text-primary);
  text-align: left;
  cursor: grab;
}

.item:hover {
  background: var(--bg-hover);
}

.item.dim {
  opacity: 0.55;
}

.item-name {
  font-size: 12px;
  font-weight: 500;
}

.item-io {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  flex-wrap: wrap;
}

.about p {
  margin: 0 0 4px;
}

.canvas {
  border: 1px solid var(--border-primary);
  border-radius: var(--radius-md);
  background: var(--bg-primary);
  overflow: hidden;
}

.panel {
  overflow-y: auto;
}

.fnode {
  display: flex;
  align-items: center;
  gap: 10px;
  min-width: 200px;
  padding: 10px 14px;
  border: 1px solid var(--border-strong);
  border-radius: var(--radius-md);
  background: var(--bg-popover);
  color: var(--text-primary);
  box-shadow: var(--shadow);
}

.fnode.picked {
  border-color: var(--border-focus);
  box-shadow: var(--shadow-focus);
}

.fnode.bad {
  border-color: var(--danger-500);
}

.fnode-source {
  background: var(--bg-tertiary);
}

.fnode-title {
  font-size: 13px;
  font-weight: 600;
}

.fnode-sub {
  font-size: 11px;
  color: var(--text-muted);
}

.dot {
  width: 10px;
  height: 10px;
  border-radius: 50%;
  flex: none;
}

:deep(.vue-flow__edge.edge-bad .vue-flow__edge-path) {
  stroke: var(--danger-500);
}

:deep(.vue-flow__controls-button) {
  background: var(--bg-popover);
  border-color: var(--border-primary);
  fill: var(--text-secondary);
}

@media (max-width: 1100px) {
  .editor {
    grid-template-columns: 1fr;
    height: auto;
  }

  .palette {
    max-height: 240px;
  }

  .canvas {
    height: 60vh;
  }
}
</style>
