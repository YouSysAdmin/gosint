<script setup lang="ts">
import {
  computed,
  defineAsyncComponent,
  nextTick,
  onBeforeUnmount,
  onMounted,
  ref,
  watch,
} from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { apiErrorMessage } from '@/api/client'
import { getInvestigation } from '@/api/investigations'
import {
  ArrowLeftRight,
  ChevronDown,
  ChevronUp,
  Crosshair,
  Funnel,
  ChevronLeft,
  ChevronRight,
  Image as ImageIcon,
  Layers,
  MapPin,
  Redo2,
  Undo2,
  Maximize,
  Network,
  Share2,
  SquareTerminal,
  SquareDashedMousePointer,
  Table2,
  Workflow,
  ZoomIn,
  ZoomOut,
} from 'lucide-vue-next'
import {
  cancelScan,
  clearSketch,
  createSchedule,
  deleteSchedule,
  deleteSketch,
  exportSketch,
  getChanges,
  getGraph,
  getSketch,
  listLogs,
  listSchedules,
  listSketches,
  listScans,
  mergeNodes,
  openEvents,
  redo,
  runSchedule,
  setPositions,
  undo,
  updateNode,
  updateSchedule,
  type Changes,
  type GraphChange,
  type Schedule,
} from '@/api/sketches'
import type {
  Flow,
  Transform,
  GraphEdge,
  GraphNode,
  LogEntry,
  Role,
  Scan,
  Sketch,
} from '@/api/types'
import GraphCanvas, { type Point } from '@/components/GraphCanvas.vue'
import { isModalOpen } from '@/components/BaseModal.vue'
import LoadingBlock from '@/components/LoadingBlock.vue'
import MenuButton from '@/components/MenuButton.vue'
import StatusBadge from '@/components/StatusBadge.vue'
import AddNodeDialog from '@/components/sketch/AddNodeDialog.vue'
import DeleteNodesDialog from '@/components/sketch/DeleteNodesDialog.vue'
import EdgePanel from '@/components/sketch/EdgePanel.vue'
import EdgesTable from '@/components/sketch/EdgesTable.vue'
import FilterPopover from '@/components/sketch/FilterPopover.vue'
import LeftPanel from '@/components/sketch/LeftPanel.vue'
import NodesTable from '@/components/sketch/NodesTable.vue'
import GraphMenu, { type MenuKind } from '@/components/sketch/GraphMenu.vue'
import QuickAdd from '@/components/sketch/QuickAdd.vue'
import ShortcutsDialog from '@/components/sketch/ShortcutsDialog.vue'
import SketchSettingsDialog from '@/components/sketch/SketchSettingsDialog.vue'
import LinkDialog from '@/components/sketch/LinkDialog.vue'
import ExportDialog from '@/components/ExportDialog.vue'
import { blobOfDataURL, saveBlob, saveJSON } from '@/composables/download'
import NodePanel from '@/components/sketch/NodePanel.vue'
import RunDialog from '@/components/sketch/RunDialog.vue'
import { formatTimeParts } from '@/composables/formatDate'
import { fields as filterFields, hiddenBy, type Rule } from '@/composables/graphFilters'
import {
  storageGet,
  storageGetJSON,
  storageRemove,
  storageSet,
  storageSetJSON,
} from '@/composables/storage'
import { modKey } from '@/composables/modKey'
import { useConfirm } from '@/composables/useConfirm'
import { useCatalogStore } from '@/stores/catalog'
import { relativesOf, shortestPath, type Relatives } from '@/composables/graphSelect'
import { originOf, seenOf } from '@/composables/provenance'
import { listFlows } from '@/api/catalog'
import { useAuthStore } from '@/stores/auth'
import { useNotificationStore } from '@/stores/notification'

const route = useRoute()
const router = useRouter()
const toast = useNotificationStore()
const auth = useAuthStore()
const catalog = useCatalogStore()
const { confirm } = useConfirm()
const id = computed(() => String(route.params.id))

const sketch = ref<Sketch | null>(null)
const role = ref<Role>('viewer')
// The investigation's name and its other sketches, for the breadcrumb.
const investigationName = ref('')
const siblings = ref<Sketch[]>([])
const switching = ref(false)
const nodes = ref<GraphNode[]>([])
const edges = ref<GraphEdge[]>([])
const selected = ref(new Set<string>())
const logs = ref<LogEntry[]>([])
const scans = ref<Scan[]>([])
const dialog = ref<
  '' | 'add' | 'run' | 'import' | 'export' | 'delete' | 'link' | 'settings' | 'shortcuts'
>('')
const linkFrom = ref<GraphNode | null>(null)
const linkTo = ref<GraphNode | null>(null)
// The drawer starts closed, the scans badge tells what is running.
const drawer = ref<'logs' | 'scans' | 'schedules' | ''>('')

// Schedules: flows run again every so often on this sketch, listed
// with what the last run found. Loaded when the tab opens and again
// when a scan settles.
const schedules = ref<Schedule[]>([])
const flowsForSchedule = ref<Flow[]>([])
const newSchedule = ref<{ flow_id: string; every: number; scope: 'all' | 'selection' } | null>(null)
const everyOptions = [
  { label: 'every hour', value: 3600 },
  { label: 'every 6 hours', value: 6 * 3600 },
  { label: 'every 12 hours', value: 12 * 3600 },
  { label: 'every day', value: 24 * 3600 },
  { label: 'every week', value: 7 * 24 * 3600 },
]

async function loadSchedules() {
  try {
    schedules.value = await listSchedules(id.value)
  } catch (err) {
    toast.error(apiErrorMessage(err))
  }
}

watch(drawer, (d) => {
  if (d === 'schedules') void loadSchedules()
})

async function startSchedule() {
  try {
    flowsForSchedule.value = await listFlows()
  } catch (err) {
    toast.error(apiErrorMessage(err))
    return
  }
  newSchedule.value = {
    flow_id: flowsForSchedule.value[0]?.id ?? '',
    every: 24 * 3600,
    scope: selected.value.size ? 'selection' : 'all',
  }
}

async function saveSchedule() {
  const s = newSchedule.value
  if (!s || !s.flow_id) return
  try {
    await createSchedule(id.value, {
      flow_id: s.flow_id,
      node_ids: s.scope === 'selection' ? [...selected.value] : [],
      every_seconds: s.every,
    })
    newSchedule.value = null
    await loadSchedules()
  } catch (err) {
    toast.error(apiErrorMessage(err))
  }
}

async function toggleSchedule(s: Schedule) {
  try {
    await updateSchedule(id.value, s.id, { every_seconds: s.every_seconds, enabled: !s.enabled })
    await loadSchedules()
  } catch (err) {
    toast.error(apiErrorMessage(err))
  }
}

async function runNow(s: Schedule) {
  try {
    await runSchedule(id.value, s.id)
    toast.success(`${s.flow_name} queued`)
    await loadSchedules()
  } catch (err) {
    toast.error(apiErrorMessage(err))
  }
}

async function removeSchedule(s: Schedule) {
  try {
    await deleteSchedule(id.value, s.id)
    await loadSchedules()
  } catch (err) {
    toast.error(apiErrorMessage(err))
  }
}

function everyLabel(seconds: number): string {
  return (
    everyOptions.find((o) => o.value === seconds)?.label ?? `every ${Math.round(seconds / 60)} min`
  )
}

// scopeOf reads a schedule's selection: a fixed set or the whole type.
function scopeOf(s: Schedule): string {
  const flow = flowsForSchedule.value.find((f) => f.id === s.flow_id)
  const input = flow?.definition.input ?? 'input'
  return s.node_ids.length ? `${s.node_ids.length} selected` : `every ${input}`
}
// The newest log line, shown in the closed console's strip.
const lastLog = computed(() => logs.value[logs.value.length - 1] ?? null)
const search = ref('')
const canvas = ref<InstanceType<typeof GraphCanvas> | null>(null)
// The map and MapLibre with it load the first time the Map view opens,
// most sketches never show it. The notes (the TipTap editor), the chat
// and the import dialog the same, each the first time it is asked for.
const MapCanvas = defineAsyncComponent(() => import('@/components/MapCanvas.vue'))
const NotesPanel = defineAsyncComponent(() => import('@/components/sketch/NotesPanel.vue'))
const ChatPanel = defineAsyncComponent(() => import('@/components/sketch/ChatPanel.vue'))
const ImportDialog = defineAsyncComponent(() => import('@/components/sketch/ImportDialog.vue'))
type MapView = InstanceType<typeof import('@/components/MapCanvas.vue').default>
const mapView = ref<MapView | null>(null)
const view = ref<'graph' | 'table' | 'relationships' | 'map'>('graph')
const showLeft = ref(storageGet('gosint_left') !== 'off')

// Collections: many leaves of one kind under one parent drawn as one
// node. On unless switched off, remembered.
const grouping = ref(storageGet('gosint_groups') !== 'off')
watch(grouping, (on) => storageSet('gosint_groups', on ? 'on' : 'off'))

// Pictures hosted elsewhere are fetched only once asked, per sketch:
// each fetch tells the host who opened the sketch and when.
const remoteImagesKey = `gosint.remoteImages.${id.value}`
const remoteImages = ref(storageGet(remoteImagesKey) === 'on')
watch(remoteImages, (on) => storageSet(remoteImagesKey, on ? 'on' : 'off'))

// groupMenu is the right click on a collection: its members become the
// selection and the selection menu opens on them.
function groupMenu(ids: string[], at: Point) {
  selectMany(ids, false)
  menu.value = { kind: 'selection', x: at.x, y: at.y, ids }
}
const showFilters = ref(false)
const rules = ref<Rule[]>([])
const hiddenIds = computed(() => hiddenBy(nodes.value, rules.value))
const shownNodes = computed(() => nodes.value.filter((n) => !hiddenIds.value.has(n.id)))
const ruleFields = computed(() => filterFields(nodes.value))

// The filter rules are kept per sketch in this browser, a reload must
// not undo a triage view.
const rulesKey = computed(() => `gosint.filters.${id.value}`)
rules.value = storageGetJSON<Rule[]>(rulesKey.value, [])
watch(
  rules,
  (r) => {
    if (r.length) storageSetJSON(rulesKey.value, r)
    else storageRemove(rulesKey.value)
  },
  { deep: true },
)

// The log can be narrowed by level and to one scan, and a line leads to
// its scan.
const logLevel = ref('')
const logScan = ref('')
const litScan = ref('')

// foundBy is what a scan found: the entities whose origin names it, or
// that it found again (seen). The lit scan's are ringed on the canvas.
function foundBy(scanId: string): string[] {
  return nodes.value
    .filter((n) => originOf(n.metadata)?.scan === scanId || seenOf(n.metadata)?.scan === scanId)
    .map((n) => n.id)
}

const litIds = computed(() => (litScan.value ? new Set(foundBy(litScan.value)) : undefined))

function selectFoundBy(s: Scan) {
  const ids = foundBy(s.id)
  if (!ids.length) {
    toast.info('Nothing in the sketch came from this scan')
    return
  }
  litScan.value = s.id
  selectMany(ids, false)
  canvas.value?.zoomToSelection(new Set(ids))
}
const logLevels = computed(() => [...new Set(logs.value.map((l) => l.level))].sort())
// The newest lines first, at most LOG_SHOWN drawn: a thousand rows of
// text in the DOM made every new line cost a layout of them all.
const LOG_SHOWN = 400
const matchingLogs = computed(() =>
  [...logs.value]
    .reverse()
    .filter((l) => !logLevel.value || l.level === logLevel.value)
    .filter((l) => !logScan.value || l.scan_id === logScan.value),
)
const shownLogs = computed(() => matchingLogs.value.slice(0, LOG_SHOWN))
const scanName = (scanId: string) => scans.value.find((s) => s.id === scanId)?.name ?? 'scan'

function toScan(l: LogEntry) {
  if (!l.scan_id) return
  showScan(l.scan_id)
}

// showScan opens the Scans drawer on one scan, lit and scrolled to.
function showScan(id: string) {
  litScan.value = id
  drawer.value = 'scans'
  nextTick(() => document.getElementById('scan-' + id)?.scrollIntoView({ block: 'center' }))
}

function logOf(s: Scan) {
  logScan.value = s.id
  logLevel.value = ''
  drawer.value = 'logs'
}
const dropAt = ref<Point | null>(null)
const addType = ref('')
const cloneOf = ref<GraphNode | null>(null)
const showChat = ref(false)
const showNotes = ref(false)
const sideTab = ref<'entity' | 'chat' | 'notes'>('entity')
const selectedEdge = ref('')
const boxSelect = ref(false)
const preselect = ref('')
const menu = ref<{
  kind: MenuKind
  x: number
  y: number
  ids: string[]
  edgeId?: string
  at?: Point
} | null>(null)
const quick = ref<{ x: number; y: number; at: Point } | null>(null)
let events: EventSource | null = null
// gone is set on unmount so nothing awaited lands on a dead page.
let gone = false
const clock: Intl.DateTimeFormatOptions = { hour: '2-digit', minute: '2-digit', second: '2-digit' }

const canEdit = computed(() => role.value !== 'viewer')
const isOwner = computed(() => role.value === 'owner')
const picked = computed(() => nodes.value.filter((n) => selected.value.has(n.id)))
const one = computed(() => (picked.value.length === 1 ? picked.value[0] : null))
const edgeSelected = computed(() => edges.value.find((e) => e.id === selectedEdge.value) ?? null)
const menuNodes = computed(() => {
  const ids = new Set(menu.value?.ids ?? [])
  return ids.size ? nodes.value.filter((n) => ids.has(n.id)) : []
})
const menuEdge = computed(() => edges.value.find((e) => e.id === menu.value?.edgeId))
const sameType = computed(
  () => picked.value.length > 0 && picked.value.every((n) => n.type === picked.value[0].type),
)
const running = computed(
  () => scans.value.filter((s) => s.status === 'pending' || s.status === 'running').length,
)
const matches = computed(() => {
  const q = search.value.trim().toLowerCase()
  if (!q) return []
  return nodes.value.filter((n) => n.label.toLowerCase().includes(q)).slice(0, 8)
})

// history is what Undo and Redo would do next, read with the graph:
// every hand edit reloads it.
const history = ref<Changes>({ undo: '', redo: '' })

async function loadGraph() {
  const sid = id.value
  const g = await getGraph(sid)
  // The page may have moved on while the graph travelled.
  if (gone || sid !== id.value) return
  nodes.value = g.nodes
  edges.value = g.edges
  if (canEdit.value)
    getChanges(id.value)
      .then((h) => (history.value = h))
      .catch(() => {})
  for (const s of [...selected.value])
    if (!g.nodes.some((n) => n.id === s)) selected.value.delete(s)
  if (selectedEdge.value && !g.edges.some((e) => e.id === selectedEdge.value))
    selectedEdge.value = ''
}

async function load() {
  try {
    await catalog.load()
    sketch.value = await getSketch(id.value)
    const inv = (await getInvestigation(sketch.value.investigation_id)).investigation
    role.value = inv.current_user_role
    investigationName.value = inv.name
    listSketches()
      .then((all) => {
        siblings.value = all.filter((s) => s.investigation_id === sketch.value?.investigation_id)
      })
      .catch(() => {})
    await loadGraph()
    logs.value = await listLogs(id.value)
    scans.value = await listScans(id.value)
  } catch (err) {
    toast.error(apiErrorMessage(err))
  }
}

function select(nodeId: string, additive: boolean) {
  const next = new Set(additive ? selected.value : [])
  if (additive && next.has(nodeId)) next.delete(nodeId)
  else next.add(nodeId)
  selected.value = next
  selectedEdge.value = ''
}

function selectMany(ids: string[], additive: boolean) {
  const next = new Set(additive ? selected.value : [])
  for (const id of ids) next.add(id)
  selected.value = next
  selectedEdge.value = ''
}

function unselectMany(ids: string[]) {
  const next = new Set(selected.value)
  for (const id of ids) next.delete(id)
  selected.value = next
}

function selectEdge(edgeId: string) {
  selectedEdge.value = edgeId
  selected.value = new Set()
}

// clearSelection also drops a scan's highlight: a click on the
// background or Escape leaves nothing ringed.
function clearSelection() {
  selected.value = new Set()
  selectedEdge.value = ''
  litScan.value = ''
}

function selectAll() {
  selectMany(visibleIds(), false)
}

function visibleIds(): string[] {
  return canvas.value?.nodeIdsVisible() ?? nodes.value.map((n) => n.id)
}

// The selection tools of the menu: relatives of what the menu was
// opened on, every entity of a type, the inverse of the selection, and
// the shortest path between two entities, which replaces the selection
// so the wires along it light up.
function selectRelatives(how: Relatives) {
  const from = new Set(menu.value?.ids ?? selected.value)
  menu.value = null
  const found = relativesOf(from, edges.value, how)
  if (!found.length) {
    toast.info(`No ${how === 'connected' ? 'connected entity' : how} here`)
    return
  }
  selectMany(found, false)
}

function selectType(type: string) {
  menu.value = null
  selectMany(
    nodes.value.filter((n) => n.type === type).map((n) => n.id),
    false,
  )
}

function invertSelection() {
  menu.value = null
  const was = selected.value
  selectMany(
    visibleIds().filter((id) => !was.has(id)),
    false,
  )
}

function pathBetween(a: GraphNode, b: GraphNode) {
  menu.value = null
  const path = shortestPath(a.id, b.id, edges.value)
  if (!path) {
    toast.info(`Nothing joins ${a.label} and ${b.label}`)
    return
  }
  selectMany(path, false)
  toast.success(
    `${path.length - 1} relationship${path.length === 2 ? '' : 's'} from ${a.label} to ${b.label}`,
  )
}

// --- Right-click menus and quick add ---------------------------------

// A right click opens the menu and leaves the selection alone, the
// menu has its own button for the details.
function nodeMenu(nodeId: string, at: Point) {
  if (selected.value.has(nodeId) && selected.value.size > 1) {
    menu.value = { kind: 'selection', x: at.x, y: at.y, ids: [...selected.value] }
    return
  }
  menu.value = { kind: 'node', x: at.x, y: at.y, ids: [nodeId] }
}

// menuDelete removes what the menu was opened on.
function menuDelete() {
  if (menu.value) selected.value = new Set(menu.value.ids)
  menu.value = null
  removeSelected()
}

function toggleLeft() {
  showLeft.value = !showLeft.value
  storageSet('gosint_left', showLeft.value ? 'on' : 'off')
}

function toggle(nodeId: string) {
  select(nodeId, true)
}

// openAdd starts the Add dialog, with a type when a card was clicked.
function openAdd(type = '') {
  addType.value = type
  cloneOf.value = null
  dropAt.value = null
  dialog.value = 'add'
}

// openClone starts the Add dialog from an entity, the copy landing
// beside it.
function openClone(n: GraphNode) {
  menu.value = null
  addType.value = ''
  cloneOf.value = n
  dropAt.value = n.x !== null && n.y !== null ? { x: n.x + 30, y: n.y + 30 } : null
  dialog.value = 'add'
}

// dropType is a type card let go on the canvas: the dialog opens for
// that type and the new entity lands where it was dropped.
function dropType(type: string, graph: Point) {
  addType.value = type
  cloneOf.value = null
  dropAt.value = graph
  dialog.value = 'add'
}

function edgeMenu(edgeId: string, at: Point) {
  menu.value = { kind: 'edge', x: at.x, y: at.y, ids: [], edgeId }
}

function backgroundMenu(at: Point, graph: Point) {
  if (selected.value.size > 1) {
    menu.value = { kind: 'selection', x: at.x, y: at.y, ids: [...selected.value] }
    return
  }
  menu.value = { kind: 'background', x: at.x, y: at.y, ids: [], at: graph }
}

function quickAdd(at: Point, graph: Point) {
  if (!canEdit.value) return
  menu.value = null
  quick.value = { x: at.x, y: at.y, at: graph }
}

// quickAdded puts the new entity where the canvas was clicked.
async function quickAdded(n: GraphNode) {
  const at = quick.value?.at
  quick.value = null
  if (at) await moved([{ node_id: n.id, x: at.x, y: at.y }])
  await loadGraph()
  selected.value = new Set([n.id])
}

function menuAddHere() {
  const at = menu.value?.at
  const x = menu.value?.x ?? 0
  const y = menu.value?.y ?? 0
  menu.value = null
  if (at) quick.value = { x, y, at }
}

// The run dialog and the assistant work on the selection, so what the
// menu was opened on becomes the selection first.
function selectMenuNodes() {
  if (menu.value?.ids.length) {
    selected.value = new Set(menu.value.ids)
    selectedEdge.value = ''
  }
  menu.value = null
}

function configure(e: Transform) {
  selectMenuNodes()
  preselect.value = e.template_id || e.name
  dialog.value = 'run'
}

function askAi() {
  selectMenuNodes()
  showChat.value = true
  sideTab.value = 'chat'
}

function addRelation(n: GraphNode) {
  menu.value = null
  cloneOf.value = null
  selected.value = new Set([n.id])
  dialog.value = 'add'
}

async function flag(n: GraphNode, color: string) {
  menu.value = null
  try {
    await updateNode(id.value, n.id, { flag: color })
    await loadGraph()
  } catch (err) {
    toast.error(apiErrorMessage(err))
  }
}

function exportSelection() {
  const set = new Set(selected.value)
  const data = {
    nodes: picked.value,
    edges: edges.value.filter((e) => set.has(e.source) && set.has(e.target)),
  }
  saveJSON(data, `${sketch.value?.title ?? 'sketch'}-selection.json`)
  menu.value = null
}

function linkDrag(fromId: string, toId: string) {
  const from = nodes.value.find((n) => n.id === fromId)
  const to = nodes.value.find((n) => n.id === toId)
  if (from && to) link(from, to)
}

// --- Layouts -----------------------------------------------------------

async function relayout(kind: 'force' | 'hierarchy') {
  if (
    !(await confirm({
      title: kind === 'force' ? 'Force layout' : 'Hierarchy layout',
      message: 'Every entity is placed again. The current positions are lost.',
      confirmText: 'Apply',
    }))
  )
    return
  if (kind === 'force') canvas.value?.layoutForce()
  else canvas.value?.layoutHierarchy()
}

function goSketch(sketchId: string) {
  switching.value = false
  if (sketchId !== id.value) router.push(`/sketches/${sketchId}`)
}

function fit() {
  if (view.value === 'map') mapView.value?.fit()
  else canvas.value?.fit()
}

function focusFromList(nodeId: string) {
  selected.value = new Set([nodeId])
  selectedEdge.value = ''
  if (view.value === 'graph') canvas.value?.center(nodeId)
}

function focus(nodeId: string) {
  selected.value = new Set([nodeId])
  selectedEdge.value = ''
  menu.value = null
  canvas.value?.center(nodeId)
  search.value = ''
}

async function moved(positions: { node_id: string; x: number; y: number }[]) {
  // Kept on the entities too, the neighbour map draws by them.
  const byId = new Map(nodes.value.map((n) => [n.id, n]))
  for (const p of positions) {
    const n = byId.get(p.node_id)
    if (n) {
      n.x = p.x
      n.y = p.y
    }
  }
  if (!canEdit.value) return
  try {
    await setPositions(id.value, positions)
  } catch {
    // a lost position is not worth interrupting anybody for
  }
}

function toggleChat() {
  showChat.value = !showChat.value
  if (showChat.value) {
    showNotes.value = false
    sideTab.value = 'chat'
  } else sideTab.value = 'entity'
}

// closeSide shuts the right panel whatever it shows.
function closeSide() {
  clearSelection()
  showChat.value = false
  showNotes.value = false
  sideTab.value = 'entity'
}

function toggleNotes() {
  showNotes.value = !showNotes.value
  if (showNotes.value) {
    showChat.value = false
    sideTab.value = 'notes'
  } else sideTab.value = 'entity'
}

function link(from: GraphNode, to: GraphNode | null = null) {
  linkFrom.value = from
  linkTo.value = to
  dialog.value = 'link'
}

async function linked() {
  dialog.value = ''
  await loadGraph()
}

function removeSelected() {
  dialog.value = 'delete'
}

async function removed(count: number) {
  dialog.value = ''
  selected.value = new Set()
  toast.success(`Deleted ${count} entities`)
  await loadGraph()
}

async function clearAll() {
  if (
    !(await confirm({
      title: 'Clear sketch',
      message: `Delete all ${nodes.value.length} entities and ${edges.value.length} relationships of ${sketch.value?.title}? Scans and the log stay.`,
      confirmText: 'Clear',
      variant: 'danger',
    }))
  )
    return
  try {
    await clearSketch(id.value)
    selected.value = new Set()
    await loadGraph()
  } catch (err) {
    toast.error(apiErrorMessage(err))
  }
}

async function removeSketch() {
  const s = sketch.value
  if (
    !s ||
    !(await confirm({
      title: 'Delete sketch',
      message: `Delete ${s.title} with its graph, scans and log? This cannot be undone.`,
      confirmText: 'Delete sketch',
      variant: 'danger',
    }))
  )
    return
  try {
    await deleteSketch(id.value)
    await router.push(`/investigations/${s.investigation_id}`)
  } catch (err) {
    toast.error(apiErrorMessage(err))
  }
}

async function mergeSelected() {
  const keep = picked.value[0]
  if (
    !(await confirm({
      title: 'Merge',
      message: `Merge ${picked.value.length} entities into ${keep.label}?`,
    }))
  )
    return
  try {
    await mergeNodes(id.value, [...selected.value], keep.id)
    selected.value = new Set([keep.id])
    await loadGraph()
  } catch (err) {
    toast.error(apiErrorMessage(err))
  }
}

function download() {
  dialog.value = 'export'
}

// downloadPNG saves the canvas as drawn. An entity image fetched
// without CORS makes the canvas unreadable, in which case it says so.
function downloadPNG() {
  const data = canvas.value?.toPNG() ?? ''
  if (!data) {
    toast.error('The canvas cannot be exported while an entity shows an image from another site')
    return
  }
  saveBlob(blobOfDataURL(data), `${sketch.value?.title ?? 'sketch'}.png`)
}

async function cancel(s: Scan) {
  try {
    await cancelScan(id.value, s.id)
  } catch (err) {
    toast.error(apiErrorMessage(err))
  }
}

async function added(n: GraphNode) {
  dialog.value = ''
  if (dropAt.value) await moved([{ node_id: n.id, x: dropAt.value.x, y: dropAt.value.y }])
  dropAt.value = null
  await loadGraph()
  focus(n.id)
}

// revert undoes or redoes the caller's last hand edit and says what it
// was. A transform's findings are not an edit: delete them instead.
async function revert(back: boolean) {
  if (!canEdit.value || !(back ? history.value.undo : history.value.redo)) return
  try {
    const r = await (back ? undo : redo)(id.value)
    history.value = { undo: r.undo, redo: r.redo }
    toast.success(`${back ? 'Undid' : 'Redid'}: ${r.reverted}`)
    await loadGraph()
  } catch (err) {
    toast.error(apiErrorMessage(err))
  }
}

function launched() {
  dialog.value = ''
  menu.value = null
  preselect.value = ''
}

const openSteps = ref(new Set<string>())

function toggleSteps(id: string) {
  const next = new Set(openSteps.value)
  if (next.has(id)) next.delete(id)
  else next.add(id)
  openSteps.value = next
}

// outcome sums a settled scan up: what it wrote and how long it took.
function outcome(s: Scan): string {
  const parts: string[] = []
  const d = s.details ?? {}
  if (typeof d.nodes === 'number') parts.push(`${d.nodes} entities, ${d.edges ?? 0} relationships`)
  if (s.started_at && s.completed_at) {
    const secs = Math.round((Date.parse(s.completed_at) - Date.parse(s.started_at)) / 1000)
    parts.push(secs < 60 ? `${secs}s` : `${Math.floor(secs / 60)}m ${secs % 60}s`)
  }

  return parts.join(', ')
}

// stepsOf reads a flow scan's per-step summary: a count line, or the
// reason a step was skipped or failed.
function stepsOf(s: Scan): { id: string; text: string; failed: boolean }[] {
  const steps = s.details?.steps
  if (!steps || typeof steps !== 'object') return []

  return Object.entries(steps as Record<string, unknown>).map(([id, r]) => {
    if (typeof r === 'string') return { id, text: r, failed: r.startsWith('failed') }
    const c = (r ?? {}) as Record<string, unknown>
    let text = `${c.inputs ?? 0} in, ${c.nodes ?? 0} entities, ${c.edges ?? 0} relationships`
    // A filter writes nothing and hands its kept inputs on.
    if (typeof c.passed === 'number' && c.passed !== c.nodes) text += `, ${c.passed} passed on`

    return { id, text, failed: false }
  })
}

// Delete removes the selection, Escape clears it, unless a field or a
// dialog has the keyboard. isModalOpen covers the dialogs this page does
// not own, the confirm among them: Backspace under "Clear the sketch?"
// once opened the delete dialog on top of it.
function onKey(ev: KeyboardEvent) {
  const t = ev.target as HTMLElement | null
  if (
    dialog.value ||
    quick.value ||
    isModalOpen() ||
    t?.closest('input, textarea, select, [contenteditable]')
  )
    return
  if ((ev.key === 'Delete' || ev.key === 'Backspace') && canEdit.value && selected.value.size) {
    ev.preventDefault()
    removeSelected()
  } else if (
    (ev.metaKey || ev.ctrlKey) &&
    (ev.key === 'a' || ev.key === 'A') &&
    view.value === 'graph'
  ) {
    ev.preventDefault()
    selectAll()
  } else if ((ev.metaKey || ev.ctrlKey) && (ev.key === 'z' || ev.key === 'Z')) {
    ev.preventDefault()
    void revert(!ev.shiftKey)
  } else if (ev.ctrlKey && (ev.key === 'y' || ev.key === 'Y')) {
    ev.preventDefault()
    void revert(false)
  } else if ((ev.metaKey || ev.ctrlKey) && (ev.key === 'b' || ev.key === 'B')) {
    ev.preventDefault()
    toggleLeft()
  } else if ((ev.metaKey || ev.ctrlKey) && (ev.key === 'd' || ev.key === 'D')) {
    ev.preventDefault()
    drawer.value = drawer.value ? '' : 'logs'
  } else if ((ev.metaKey || ev.ctrlKey) && ev.shiftKey && (ev.key === 'l' || ev.key === 'L')) {
    // Shift is in, the browser keeps a plain Cmd/Ctrl+L for its address bar.
    ev.preventDefault()
    toggleNotes()
  } else if ((ev.metaKey || ev.ctrlKey) && (ev.key === 'g' || ev.key === 'G') && canEdit.value) {
    ev.preventDefault()
    dialog.value = 'settings'
  } else if ((ev.metaKey || ev.ctrlKey) && (ev.key === 'k' || ev.key === 'K')) {
    ev.preventDefault()
    dialog.value = 'shortcuts'
  } else if (ev.key === 'Escape') {
    menu.value = null
    if (selected.value.size || selectedEdge.value || litScan.value) clearSelection()
    else if (showChat.value || showNotes.value) closeSide()
  }
}

function onScan(s: Scan) {
  const i = scans.value.findIndex((x) => x.id === s.id)
  if (i >= 0) scans.value[i] = s
  else scans.value.unshift(s)
  if (s.status === 'completed' || s.status === 'failed') {
    loadGraph().catch(() => {})
    if (drawer.value === 'schedules') void loadSchedules()
  }
}

// Another member's edit arrives as a graph event: the graph reloads a
// moment later (several edits in a row reload once) and the header
// says who, for a while. One's own edits reload on their own.
const live = ref<{ name: string; kind: string; at: number } | null>(null)
let reloadTimer = 0
let liveTimer = 0
let focusTimer = 0

function onGraph(g: GraphChange) {
  if (g.by === auth.user?.id) return
  live.value = { name: g.name || 'Someone', kind: g.kind, at: Date.now() }
  window.clearTimeout(liveTimer)
  liveTimer = window.setTimeout(() => (live.value = null), 12000)
  window.clearTimeout(reloadTimer)
  reloadTimer = window.setTimeout(() => {
    loadGraph().catch(() => {})
    if (drawer.value === 'schedules') void loadSchedules()
  }, 400)
}

function onLog(e: LogEntry) {
  logs.value.push(e)
  if (logs.value.length > 1000) logs.value.splice(0, logs.value.length - 1000)
}

onMounted(async () => {
  // The stream opens before the first load, so leaving early closes it.
  events = openEvents(id.value, { log: onLog, scan: onScan, graph: onGraph })
  await load()
  if (gone) return
  // A link from an analysis arrives with the entity to show.
  const want = route.query.focus
  if (typeof want === 'string' && nodes.value.some((n) => n.id === want)) {
    focusTimer = window.setTimeout(() => focus(want), 700)
    void router.replace({ query: {} })
  }
})

onMounted(() => window.addEventListener('keydown', onKey))

onBeforeUnmount(() => {
  gone = true
  events?.close()
  window.clearTimeout(liveTimer)
  window.clearTimeout(reloadTimer)
  window.clearTimeout(focusTimer)
  window.removeEventListener('keydown', onKey)
})
</script>

<template>
  <LoadingBlock v-if="!sketch" />
  <div v-else class="sketch">
    <header class="bar">
      <nav class="crumbs" aria-label="Breadcrumb">
        <router-link
          class="crumb"
          :to="`/investigations/${sketch.investigation_id}`"
          :title="`Back to ${investigationName || 'the investigation'}`"
          >{{ investigationName || 'Investigation' }}</router-link
        >
        <span class="slash">/</span>
        <div class="switcher">
          <button class="crumb current" @click="switching = !switching">
            {{ sketch.title }}
            <ChevronDown v-if="siblings.length > 1" :size="14" />
          </button>
          <div
            v-if="switching && siblings.length > 1"
            class="switch-menu"
            @mouseleave="switching = false"
          >
            <button
              v-for="s in siblings"
              :key="s.id"
              class="menu-item"
              :class="{ on: s.id === sketch.id }"
              @click="goSketch(s.id)"
            >
              {{ s.title }}
            </button>
          </div>
        </div>
      </nav>
      <span class="text-xs text-muted counts"
        >{{ nodes.length }} entities, {{ edges.length }} relationships</span
      >
      <span v-if="live" class="text-xs live" :title="live.kind">
        <span class="pulse"></span>{{ live.name }} just edited
      </span>
      <div class="find">
        <input
          v-model="search"
          class="form-input"
          placeholder="Find an entity"
          @keydown.enter.prevent="matches.length && focus(matches[0].id)"
        />
        <!-- With the entities panel open the panel's list shows the hits. -->
        <div v-if="matches.length && !showLeft" class="hits">
          <button v-for="m in matches" :key="m.id" class="hit" @click="focus(m.id)">
            <span class="dot" :style="{ background: catalog.color(m.type) }"></span>{{ m.label }}
          </button>
        </div>
      </div>
      <div class="flex gap-2 ml-auto">
        <button v-if="canEdit" class="btn btn-sm btn-primary" @click="openAdd()">Add</button>
        <button v-if="canEdit" class="btn btn-sm btn-secondary" @click="dialog = 'import'">
          Import
        </button>
        <MenuButton title="Settings, export, clear or delete">
          <button v-if="canEdit" class="menu-item" @click="dialog = 'settings'">
            Sketch settings
          </button>
          <button class="menu-item" @click="dialog = 'shortcuts'">Shortcuts</button>
          <div class="menu-sep"></div>
          <button class="menu-item" @click="download">Export</button>
          <button v-if="view === 'graph'" class="menu-item" @click="downloadPNG">
            Export the graph as PNG
          </button>
          <template v-if="canEdit && nodes.length">
            <div class="menu-sep"></div>
            <button class="menu-item danger" @click="clearAll">Clear the sketch</button>
          </template>
          <template v-if="isOwner">
            <div v-if="!(canEdit && nodes.length)" class="menu-sep"></div>
            <button class="menu-item danger" @click="removeSketch">Delete the sketch</button>
          </template>
        </MenuButton>
        <button
          class="btn btn-sm"
          :class="showNotes ? 'btn-primary' : 'btn-secondary'"
          :title="`Analyses and notes (${modKey} Shift L)`"
          @click="toggleNotes"
        >
          Notes
        </button>
        <button
          class="btn btn-sm"
          :class="showChat ? 'btn-primary' : 'btn-secondary'"
          title="Ask the assistant about the sketch"
          @click="toggleChat"
        >
          Chat
        </button>
        <div class="segmented">
          <button :class="{ active: view === 'graph' }" title="Graph" @click="view = 'graph'">
            <Share2 :size="14" /> Graph
          </button>
          <button :class="{ active: view === 'table' }" title="Table" @click="view = 'table'">
            <Table2 :size="14" /> Table
          </button>
          <button
            :class="{ active: view === 'relationships' }"
            title="Relationships"
            @click="view = 'relationships'"
          >
            <ArrowLeftRight :size="14" /> Relationships
          </button>
          <button :class="{ active: view === 'map' }" title="Map" @click="view = 'map'">
            <MapPin :size="14" /> Map
          </button>
        </div>
        <button
          v-if="view !== 'graph'"
          class="btn btn-sm"
          :class="rules.length ? 'btn-primary' : 'btn-secondary'"
          title="Hide entities by a rule"
          data-filters-toggle
          @click="showFilters = !showFilters"
        >
          Filters<span v-if="hiddenIds.size">&nbsp;{{ hiddenIds.size }}</span>
        </button>
        <button v-if="view === 'map'" class="btn btn-sm btn-secondary" @click="fit">Fit</button>
      </div>
    </header>

    <div class="body">
      <aside v-if="showLeft" class="leftbar">
        <LeftPanel
          :query="search"
          :nodes="nodes"
          :selected="selected"
          :can-edit="canEdit"
          @focus="focusFromList"
          @toggle="toggle"
          @select-all="(ids) => selectMany(ids, false)"
          @unselect="unselectMany"
          @clear="clearSelection"
          @add="openAdd"
          @import="dialog = 'import'"
          @transform="dialog = 'run'"
          @delete="removeSelected"
          @ask-ai="askAi"
        />
      </aside>
      <div class="graph">
        <button
          class="left-tab"
          :title="`${showLeft ? 'Hide' : 'Show'} the entities panel (${modKey} B)`"
          @click="toggleLeft"
        >
          <ChevronLeft v-if="showLeft" :size="14" />
          <ChevronRight v-else :size="14" />
        </button>
        <FilterPopover
          v-if="showFilters"
          v-model="rules"
          :fields="ruleFields"
          :hidden="hiddenIds.size"
          @close="showFilters = false"
        />
        <NodesTable
          v-if="view === 'table'"
          :nodes="shownNodes"
          :selected="selected"
          @focus="focusFromList"
          @toggle="toggle"
          @select-all="(ids) => selectMany(ids, false)"
          @unselect="unselectMany"
        />
        <EdgesTable
          v-else-if="view === 'relationships'"
          :nodes="nodes"
          :edges="edges"
          :selected-edge="selectedEdge"
          @pick="selectEdge"
          @focus="focusFromList"
        />
        <MapCanvas
          v-if="view === 'map'"
          ref="mapView"
          :nodes="nodes"
          :selected="selected"
          @select="select"
          @clear="selected = new Set()"
        />
        <GraphCanvas
          v-show="view === 'graph'"
          ref="canvas"
          v-model:box-select="boxSelect"
          :nodes="nodes"
          :edges="edges"
          :selected="selected"
          :selected-edge="selectedEdge"
          :can-edit="canEdit"
          :hidden-ids="hiddenIds"
          :lit-ids="litIds"
          :grouping="grouping"
          :remote-images="remoteImages"
          @drop-type="dropType"
          @select="select"
          @select-many="selectMany"
          @select-edge="selectEdge"
          @clear="clearSelection"
          @moved="moved"
          @node-menu="nodeMenu"
          @group-menu="groupMenu"
          @edge-menu="edgeMenu"
          @background-menu="backgroundMenu"
          @quick-add="quickAdd"
          @link-drag="linkDrag"
        />
        <div v-if="view === 'graph'" class="tools">
          <button title="Zoom in" @click="canvas?.zoomBy(1.4)"><ZoomIn :size="16" /></button>
          <button title="Zoom out" @click="canvas?.zoomBy(1 / 1.4)">
            <ZoomOut :size="16" />
          </button>
          <button title="Fit everything" @click="fit"><Maximize :size="16" /></button>
          <button
            title="Zoom to the selection"
            :disabled="selected.size < 1"
            @click="canvas?.zoomToSelection(selected)"
          >
            <Crosshair :size="16" />
          </button>
          <span class="sep"></span>
          <button
            title="Select with a box (hold S)"
            :class="{ on: boxSelect }"
            @click="boxSelect = !boxSelect"
          >
            <SquareDashedMousePointer :size="16" />
          </button>
          <button
            title="Hide entities by a rule"
            data-filters-toggle
            :class="{ on: rules.length > 0 }"
            @click="showFilters = !showFilters"
          >
            <Funnel :size="16" />
            <span v-if="hiddenIds.size" class="badge-count">{{ hiddenIds.size }}</span>
          </button>
          <button
            title="Fold 12 or more entities of one kind under one parent into a collection"
            :class="{ on: grouping }"
            @click="grouping = !grouping"
          >
            <Layers :size="16" />
          </button>
          <button
            :title="
              remoteImages
                ? 'Pictures from other sites are loaded, click to stop'
                : 'Load remote images: pictures hosted on other sites'
            "
            :class="{ on: remoteImages }"
            @click="remoteImages = !remoteImages"
          >
            <ImageIcon :size="16" />
          </button>
          <template v-if="canEdit">
            <span class="sep"></span>
            <button
              :title="history.undo ? 'Undo: ' + history.undo : 'Nothing to undo'"
              :disabled="!history.undo"
              @click="revert(true)"
            >
              <Undo2 :size="16" />
            </button>
            <button
              :title="history.redo ? 'Redo: ' + history.redo : 'Nothing to redo'"
              :disabled="!history.redo"
              @click="revert(false)"
            >
              <Redo2 :size="16" />
            </button>
          </template>
          <template v-if="canEdit && nodes.length > 1">
            <span class="sep"></span>
            <button title="Force layout" @click="relayout('force')"><Network :size="16" /></button>
            <button title="Hierarchy layout" @click="relayout('hierarchy')">
              <Workflow :size="16" />
            </button>
          </template>
        </div>
        <div v-if="picked.length && view === 'graph'" class="selection">
          <span class="text-sm">{{ picked.length }} selected</span>
          <button v-if="canEdit && sameType" class="btn btn-sm btn-primary" @click="dialog = 'run'">
            Transform
          </button>
          <button
            v-if="canEdit && picked.length === 2"
            class="btn btn-sm btn-secondary"
            @click="link(picked[0], picked[1])"
          >
            Link
          </button>
          <button
            v-if="canEdit && picked.length > 1 && sameType"
            class="btn btn-sm btn-secondary"
            @click="mergeSelected"
          >
            Merge
          </button>
          <button v-if="canEdit" class="btn btn-sm btn-danger" @click="removeSelected">
            Delete
          </button>
        </div>
        <p v-if="view === 'map' && nodes.length && mapView?.count === 0" class="hint text-muted">
          No entity here has coordinates. ip_to_infos places IP addresses and
          location_to_coordinates places postal addresses.
        </p>
        <p v-if="!nodes.length" class="hint text-muted">
          Add an entity or import a list to start. Double-click the canvas to add one where you
          click, right-click an entity for its transforms.
        </p>
      </div>

      <aside
        v-if="one || showChat || showNotes || edgeSelected"
        class="side"
        :class="{ wide: (showChat && sideTab === 'chat') || (showNotes && sideTab === 'notes') }"
      >
        <!-- With chat or notes open the close button has a row of its own
             beside the tabs, their toolbars sit in the top right corner. -->
        <button
          v-if="!(showChat || showNotes)"
          class="side-close"
          title="Close the panel (Escape)"
          @click="closeSide"
        >
          &times;
        </button>
        <div v-else class="side-head">
          <div v-if="one" class="tabs side-tabs">
            <button
              class="tab"
              :class="{ active: sideTab === 'entity' }"
              @click="sideTab = 'entity'"
            >
              Entity
            </button>
            <button
              v-if="showChat"
              class="tab"
              :class="{ active: sideTab === 'chat' }"
              @click="sideTab = 'chat'"
            >
              Chat
            </button>
            <button
              v-if="showNotes"
              class="tab"
              :class="{ active: sideTab === 'notes' }"
              @click="sideTab = 'notes'"
            >
              Notes
            </button>
          </div>
          <button class="side-close inline" title="Close the panel (Escape)" @click="closeSide">
            &times;
          </button>
        </div>
        <ChatPanel
          v-if="showChat && (!one || sideTab === 'chat')"
          :sketch-id="id"
          :selected="picked"
          :nodes="nodes"
          :can-edit="canEdit"
          @applied="loadGraph"
        />
        <NotesPanel
          v-else-if="showNotes && (!one || sideTab === 'notes')"
          :investigation-id="sketch.investigation_id"
          :sketch-id="id"
          :sketch-title="sketch.title"
          :nodes="nodes"
          :can-edit="canEdit"
          @focus="focus"
        />
        <EdgePanel
          v-else-if="edgeSelected && !one"
          :sketch-id="id"
          :edge="edgeSelected"
          :nodes="nodes"
          :can-edit="canEdit"
          @changed="loadGraph"
          @focus="focus"
          @scan="showScan"
        />
        <NodePanel
          v-else-if="one"
          :sketch-id="id"
          :node="one"
          :nodes="nodes"
          :edges="edges"
          :can-edit="canEdit"
          @changed="loadGraph"
          @focus="focus"
          @select="(ids) => selectMany(ids, false)"
          @link="link"
          @clone="openClone"
          @transform="dialog = 'run'"
          @edges-changed="loadGraph"
          @scan="showScan"
        />
      </aside>
    </div>

    <section class="drawer" :class="{ open: drawer }">
      <div class="console-strip">
        <button
          class="toggle"
          :title="`${drawer ? 'Close' : 'Open'} the console (${modKey} D)`"
          @click="drawer = drawer ? '' : 'logs'"
        >
          <ChevronDown v-if="drawer" :size="14" />
          <ChevronUp v-else :size="14" />
          <SquareTerminal :size="14" />
          Console
        </button>
        <div class="tabs drawer-tabs">
          <button
            class="tab"
            :class="{ active: drawer === 'logs' }"
            @click="drawer = drawer === 'logs' ? '' : 'logs'"
          >
            Log
          </button>
          <button
            class="tab"
            :class="{ active: drawer === 'scans' }"
            @click="drawer = drawer === 'scans' ? '' : 'scans'"
          >
            Scans<span v-if="running" class="badge badge-info ml-2">{{ running }}</span>
          </button>
          <button
            class="tab"
            :class="{ active: drawer === 'schedules' }"
            @click="drawer = drawer === 'schedules' ? '' : 'schedules'"
          >
            Schedules
          </button>
        </div>
        <button
          v-if="!drawer && lastLog"
          class="last code-font"
          title="Open the log"
          @click="drawer = 'logs'"
        >
          <span class="text-muted">{{ formatTimeParts(lastLog.created_at, clock) }}</span>
          <StatusBadge :status="lastLog.level" scope="log" />
          <span class="msg">{{ lastLog.message }}</span>
        </button>
        <span v-if="running" class="running text-xs">
          <span class="pulse"></span>{{ running }} running
        </span>
      </div>
      <div v-if="drawer === 'logs'" class="log code-font">
        <div v-if="logs.length" class="log-filters">
          <select v-model="logLevel" class="form-select">
            <option value="">All levels</option>
            <option v-for="lv in logLevels" :key="lv" :value="lv">{{ lv }}</option>
          </select>
          <button v-if="logScan" class="btn btn-sm btn-secondary" @click="logScan = ''">
            {{ scanName(logScan) }} only &times;
          </button>
          <span class="text-muted text-xs">Click a line to see its scan.</span>
        </div>
        <div
          v-for="l in shownLogs"
          :key="l.id"
          class="line"
          :class="{ clickable: l.scan_id }"
          :title="l.scan_id ? 'Show the scan' : ''"
          @click="toScan(l)"
        >
          <span class="text-muted">{{ formatTimeParts(l.created_at, clock) }}</span>
          <StatusBadge :status="l.level" scope="log" />
          <span>{{ l.message }}</span>
        </div>
        <p v-if="!logs.length" class="text-muted">Nothing has run on this sketch yet.</p>
        <p v-else-if="!shownLogs.length" class="text-muted">No line matches.</p>
        <p v-else-if="matchingLogs.length > LOG_SHOWN" class="text-muted">
          The newest {{ LOG_SHOWN }} of {{ matchingLogs.length }} lines, narrow by level or scan.
        </p>
      </div>
      <div v-else-if="drawer === 'schedules'" class="log">
        <p v-if="!schedules.length && !newSchedule" class="text-muted">
          No flow runs on a schedule here yet. One runs again every so often on a selection or on
          every entity of its input type, and the run says what is new and what has gone.
        </p>
        <div v-for="s in schedules" :key="s.id" class="line">
          <StatusBadge :status="s.enabled ? 'running' : 'cancelled'" scope="scan" />
          <span class="fw-medium">{{ s.flow_name }}</span>
          <span class="text-muted text-sm"
            >{{ everyLabel(s.every_seconds) }}, {{ scopeOf(s) }}</span
          >
          <span v-if="s.last_run_at" class="text-muted text-sm"
            >last {{ formatTimeParts(s.last_run_at, clock) }}</span
          >
          <template v-if="s.summary">
            <span class="text-sm" :class="{ 'text-danger': s.summary.status === 'failed' }">
              {{ s.summary.status }}, {{ s.summary.new }} new, {{ s.summary.seen }} seen again,
              {{ s.summary.missing }} missing
            </span>
            <button
              class="steps-toggle text-sm"
              title="Select what the last run found"
              @click="showScan(s.summary.scan_id)"
            >
              scan
            </button>
          </template>
          <span v-if="s.last_error" class="text-danger text-sm">{{ s.last_error }}</span>
          <template v-if="canEdit">
            <button class="btn btn-sm btn-secondary ml-auto" @click="runNow(s)">Run now</button>
            <button class="btn btn-sm btn-secondary" @click="toggleSchedule(s)">
              {{ s.enabled ? 'Pause' : 'Resume' }}
            </button>
            <button class="btn btn-sm btn-danger" @click="removeSchedule(s)">Delete</button>
          </template>
        </div>
        <div v-if="newSchedule" class="line">
          <select v-model="newSchedule.flow_id" class="form-select">
            <option v-for="f in flowsForSchedule" :key="f.id" :value="f.id">
              {{ f.name }} ({{ f.definition.input }})
            </option>
          </select>
          <select v-model.number="newSchedule.every" class="form-select">
            <option v-for="o in everyOptions" :key="o.value" :value="o.value">{{ o.label }}</option>
          </select>
          <select v-model="newSchedule.scope" class="form-select">
            <option value="all">on every entity of the input type</option>
            <option value="selection" :disabled="!selected.size">
              on the selection ({{ selected.size }})
            </option>
          </select>
          <button
            class="btn btn-sm btn-primary"
            :disabled="!newSchedule.flow_id"
            @click="saveSchedule"
          >
            Save
          </button>
          <button class="btn btn-sm btn-secondary" @click="newSchedule = null">Cancel</button>
        </div>
        <button v-else-if="canEdit" class="btn btn-sm btn-secondary mt-2" @click="startSchedule">
          Schedule a flow
        </button>
      </div>
      <div v-else-if="drawer === 'scans'" class="log">
        <p v-if="!scans.length" class="text-muted">Nothing has run on this sketch yet.</p>
        <div
          v-for="s in scans"
          :id="'scan-' + s.id"
          :key="s.id"
          class="line"
          :class="{ lit: litScan === s.id }"
        >
          <span class="text-muted">{{ formatTimeParts(s.started_at ?? s.created_at, clock) }}</span>
          <StatusBadge :status="s.status" scope="scan" />
          <span class="fw-medium">{{ s.name }}</span>
          <button class="steps-toggle text-sm" title="Only this scan's log" @click="logOf(s)">
            log
          </button>
          <button
            v-if="s.status === 'completed'"
            class="steps-toggle text-sm"
            title="Select what this scan found"
            @click="selectFoundBy(s)"
          >
            found
          </button>
          <button
            v-if="litScan === s.id"
            class="steps-toggle text-sm"
            title="Drop the highlight of what this scan found"
            @click="litScan = ''"
          >
            unmark
          </button>
          <span class="text-muted text-sm">{{ s.kind }}</span>
          <span class="text-muted text-sm">{{ outcome(s) }}</span>
          <button
            v-if="s.kind === 'flow' && s.details?.steps"
            class="steps-toggle text-sm"
            @click="toggleSteps(s.id)"
          >
            {{ openSteps.has(s.id) ? 'hide steps' : 'steps' }}
          </button>
          <span v-if="s.error" class="text-danger text-sm">{{ s.error }}</span>
          <button
            v-if="canEdit && (s.status === 'pending' || s.status === 'running')"
            class="btn btn-sm btn-secondary ml-auto"
            @click="cancel(s)"
          >
            Cancel
          </button>
          <ul v-if="openSteps.has(s.id)" class="steps text-sm">
            <li v-for="st in stepsOf(s)" :key="st.id">
              <span class="code-font">{{ st.id }}</span>
              <span :class="{ 'text-danger': st.failed }">{{ st.text }}</span>
            </li>
          </ul>
        </div>
      </div>
    </section>

    <AddNodeDialog
      v-if="dialog === 'add'"
      :sketch-id="id"
      :link-to="cloneOf ? undefined : (one ?? undefined)"
      :edges="edges"
      :initial-type="addType || undefined"
      :clone-of="cloneOf ?? undefined"
      @close="dialog = ''"
      @added="added"
    />
    <RunDialog
      v-if="dialog === 'run'"
      :sketch-id="id"
      :nodes="picked"
      :preselect="preselect || undefined"
      @close="launched"
      @launched="launched"
    />
    <GraphMenu
      v-if="menu"
      :kind="menu.kind"
      :x="menu.x"
      :y="menu.y"
      :sketch-id="id"
      :nodes="menuNodes"
      :edge="menuEdge"
      :all-nodes="nodes"
      :can-edit="canEdit"
      @close="menu = null"
      @launched="launched"
      @configure="configure"
      @add-relation="addRelation"
      @link-to="(n) => ((menu = null), link(n))"
      @clone="openClone"
      @ask-ai="askAi"
      @flag="flag"
      @delete="menuDelete"
      @open="focus($event.id)"
      @add="menuAddHere"
      @import="((menu = null), (dialog = 'import'))"
      @edge-changed="loadGraph"
      @select-relatives="selectRelatives"
      @select-type="selectType"
      @select-all="((menu = null), selectAll())"
      @invert="invertSelection"
      @path="pathBetween"
    >
      <template #export>
        <button class="menu-item" @click="exportSelection">
          Export {{ menu.ids.length }} entities as JSON
        </button>
      </template>
    </GraphMenu>
    <QuickAdd
      v-if="quick"
      :sketch-id="id"
      :x="quick.x"
      :y="quick.y"
      @close="quick = null"
      @added="quickAdded"
    />
    <DeleteNodesDialog
      v-if="dialog === 'delete'"
      :sketch-id="id"
      :ids="[...selected]"
      :nodes="nodes"
      @close="dialog = ''"
      @deleted="removed"
    />
    <LinkDialog
      v-if="dialog === 'link' && linkFrom"
      :sketch-id="id"
      :from="linkFrom"
      :to="linkTo ?? undefined"
      :nodes="nodes"
      :edges="edges"
      @close="dialog = ''"
      @linked="linked"
    />
    <SketchSettingsDialog
      v-if="dialog === 'settings'"
      :sketch="sketch"
      @close="dialog = ''"
      @saved="((sketch = $event), (dialog = ''))"
    />
    <ShortcutsDialog v-if="dialog === 'shortcuts'" @close="dialog = ''" />
    <ImportDialog
      v-if="dialog === 'import'"
      :sketch-id="id"
      @close="dialog = ''"
      @imported="loadGraph"
    />
    <ExportDialog
      v-if="dialog === 'export'"
      title="Export sketch"
      what="The sketch with its entities, relationships and files"
      :exporter="(p) => exportSketch(id, p)"
      @close="dialog = ''"
    />
  </div>
</template>

<style scoped>
.sketch {
  display: flex;
  flex-direction: column;
  height: calc(100vh - var(--topbar));
}

.bar {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 10px 16px;
  border-bottom: 1px solid var(--border-primary);
  background: var(--bg-primary);
}

.crumbs {
  display: flex;
  align-items: center;
  gap: 6px;
  min-width: 0;
}

.crumb {
  max-width: 220px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  color: var(--text-muted);
  font-size: 14px;
  text-decoration: none;
}

.crumb:hover {
  color: var(--text-primary);
}

.crumb.current {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  border: 0;
  background: none;
  padding: 0;
  color: var(--text-primary);
  font-weight: 600;
  cursor: pointer;
}

.slash {
  color: var(--text-placeholder);
}

.switcher {
  position: relative;
}

.switch-menu {
  position: absolute;
  top: calc(100% + 6px);
  left: 0;
  z-index: var(--z-topbar);
  min-width: 200px;
  max-height: 320px;
  overflow: auto;
  padding: 4px;
  background: var(--bg-popover);
  border: 1px solid var(--border-primary);
  border-radius: var(--radius-md);
  box-shadow: var(--shadow-md);
}

.switch-menu .on {
  font-weight: 600;
}

.counts {
  white-space: nowrap;
}

.segmented button {
  display: inline-flex;
  align-items: center;
  gap: 5px;
}

.find {
  position: relative;
  width: 240px;
}

.hits {
  position: absolute;
  top: 100%;
  left: 0;
  right: 0;
  z-index: var(--z-raised);
  background: var(--bg-popover);
  border: 1px solid var(--border-primary);
  border-radius: var(--radius-md);
  box-shadow: var(--shadow-md);
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
  width: 8px;
  height: 8px;
  border-radius: 50%;
  flex-shrink: 0;
}

.body {
  flex: 1;
  display: flex;
  min-height: 0;
}

.graph {
  flex: 1;
  position: relative;
  min-width: 0;
}

.leftbar {
  width: 280px;
  flex-shrink: 0;
  border-right: 1px solid var(--border-primary);
  background: var(--bg-primary);
  min-height: 0;
}

/* A tab on the panel's edge that hides and shows it. */
.left-tab {
  position: absolute;
  left: 0;
  top: 50%;
  z-index: 6;
  transform: translateY(-50%);
  display: flex;
  align-items: center;
  justify-content: center;
  width: 16px;
  height: 48px;
  padding: 0;
  border: none;
  border-radius: 0 6px 6px 0;
  background: color-mix(in srgb, var(--text-primary) 40%, transparent);
  color: var(--bg-primary);
  cursor: pointer;
  transition: background 0.15s;
}

.left-tab:hover {
  background: color-mix(in srgb, var(--text-primary) 75%, transparent);
}

/* The canvas tools, a strip along the top left of the graph. */
.live {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  color: var(--graph-accent);
}

.tools {
  position: absolute;
  left: 12px;
  top: 12px;
  display: flex;
  align-items: center;
  gap: 2px;
  padding: 4px;
  background: var(--bg-popover);
  border: 1px solid var(--border-primary);
  border-radius: var(--radius-md);
  box-shadow: var(--shadow-sm);
}

.tools button {
  position: relative;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 30px;
  height: 30px;
  border: 0;
  border-radius: var(--radius-sm);
  background: none;
  color: var(--text-secondary);
  font-size: 15px;
  line-height: 1;
  cursor: pointer;
}

.tools button:hover:not(:disabled) {
  background: var(--bg-hover);
  color: var(--text-primary);
}

.tools button:disabled {
  opacity: 0.4;
  cursor: default;
}

.tools button.on {
  background: var(--bg-active);
  color: var(--text-primary);
}

.tools .sep {
  width: 1px;
  height: 18px;
  margin: 0 4px;
  background: var(--border-primary);
}

.badge-count {
  position: absolute;
  top: -4px;
  right: -4px;
  min-width: 15px;
  padding: 0 3px;
  border-radius: 8px;
  background: var(--graph-accent);
  color: var(--text-on-status);
  font-size: 10px;
  line-height: 15px;
}

.selection {
  position: absolute;
  top: 12px;
  left: 50%;
  transform: translateX(-50%);
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 6px 10px;
  background: var(--bg-popover);
  border: 1px solid var(--border-primary);
  border-radius: var(--radius-md);
  box-shadow: var(--shadow-sm);
}

.hint {
  position: absolute;
  pointer-events: none;
  top: 45%;
  width: 100%;
  text-align: center;
}

.body {
  position: relative;
}

/* Laid over the graph, so opening it never moves the canvas under the
   cursor. */
.side {
  position: absolute;
  top: 0;
  right: 0;
  bottom: 0;
  z-index: var(--z-panel);
  width: 340px;
  overflow: auto;
  border-left: 1px solid var(--border-primary);
  background: var(--bg-primary);
  box-shadow: var(--shadow-lg);
  display: flex;
  flex-direction: column;
}

.side.wide {
  width: 420px;
}

.side-close {
  position: absolute;
  top: 8px;
  right: 8px;
  z-index: 2;
  width: 26px;
  height: 26px;
  border: 0;
  border-radius: var(--radius-sm);
  background: var(--bg-popover);
  color: var(--text-muted);
  font-size: 18px;
  line-height: 1;
  cursor: pointer;
}

.side-close:hover {
  background: var(--bg-hover);
  color: var(--text-primary);
}

.side-head {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 8px 8px 0 16px;
}

.side-tabs {
  margin: 0;
}

.side-close.inline {
  position: static;
  flex: none;
  margin-left: auto;
}

.drawer {
  border-top: 1px solid var(--border-primary);
  background: var(--bg-primary);
}

/* The console's strip: always one line, the newest entry when closed. */
.console-strip {
  display: flex;
  align-items: center;
  gap: 12px;
  min-width: 0;
  padding: 6px 16px;
}

.console-strip .toggle {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  flex: none;
  border: 0;
  background: none;
  color: var(--text-secondary);
  font-size: 13px;
  cursor: pointer;
}

.console-strip .toggle:hover {
  color: var(--text-primary);
}

.drawer-tabs {
  flex: none;
  margin: 0;
}

.console-strip .last {
  display: flex;
  align-items: center;
  gap: 8px;
  flex: 1;
  min-width: 0;
  border: 0;
  background: none;
  color: var(--text-primary);
  font-size: 12px;
  text-align: left;
  cursor: pointer;
}

.console-strip .last .msg {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.console-strip .running {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  flex: none;
  margin-left: auto;
  color: var(--text-secondary);
}

.pulse {
  width: 7px;
  height: 7px;
  border-radius: 50%;
  background: var(--graph-accent);
  animation: pulse 1.2s ease-in-out infinite;
}

@keyframes pulse {
  50% {
    opacity: 0.3;
  }
}

.drawer.open .log {
  height: 200px;
}

.log {
  overflow: auto;
  padding: 8px 16px;
  font-size: 12px;
}

.line {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 8px;
  padding: 2px 4px;
  border-radius: var(--radius-sm);
}

.line.clickable {
  cursor: pointer;
}

.line.clickable:hover,
.line.lit {
  background: var(--bg-hover);
}

.log-filters {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 6px;
}

.log-filters .form-select {
  width: auto;
  font-size: 12px;
  padding: 3px 28px 3px 8px;
}

.steps-toggle {
  border: 0;
  background: none;
  color: var(--text-secondary);
  text-decoration: underline;
  cursor: pointer;
}

.steps {
  flex-basis: 100%;
  margin: 0 0 4px 24px;
  padding: 0;
  list-style: none;
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.steps li {
  display: flex;
  gap: 8px;
}
</style>
