<script setup lang="ts">
// The sketch graph, drawn by force-graph on a canvas.
//
// A node is a small shape coloured by its own colour or its type's,
// sized by how many relationships it has, its type's icon inside once
// zoomed in enough. Names are drawn in one pass after the nodes
// (drawLabels): the selection, the hovered entity and
// the hubs as chips at any zoom, everyone else once zoomed in, never
// two on top of each other. Dragging a node pins it where it was dropped and reports the
// position, so the layout survives a reload. Hovering lights the node
// and its neighbours up and dims the rest.
//
// Beyond clicks the canvas reports right clicks (for the menus), a
// double click on the background (quick add), an Alt drag from one
// node to another (a new relationship) and a box selection (hold S).
import { forceCollide, forceX, forceY } from 'd3-force'
import ForceGraph from 'force-graph'
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import type { GraphEdge, GraphNode } from '@/api/types'
import { iconImage, inkOn } from '@/composables/iconImage'
import { treeLayout } from '@/composables/treeLayout'
import { useCatalogStore } from '@/stores/catalog'

interface Node {
  id: string
  label: string
  type: string
  color: string

  // fill is the colour drawn: the entity's own, else its type's,
  // resolved once per reload rather than per frame.
  fill: string

  // icon is the type's Lucide icon name, drawn inside once big enough.
  icon: string
  shape: string
  flag: string
  size: number | null
  image: string
  degree: number

  // A collection: one node standing for many leaves of one type hanging
  // from one parent by one relationship.
  group?: { members: string[]; parent: string }
  x?: number
  y?: number
  fx?: number
  fy?: number
}

interface Link {
  id: string
  source: string | Node
  target: string | Node
  label: string
}

export interface Point {
  x: number
  y: number
}

export type Position = { node_id: string; x: number; y: number }

const props = defineProps<{
  nodes: GraphNode[]
  edges: GraphEdge[]
  selected: Set<string>
  selectedEdge?: string
  canEdit: boolean

  // Entities the filter rules hide, on top of the legend's types.
  hiddenIds?: Set<string>

  // Entities a lit scan found, drawn with a dashed ring.
  litIds?: Set<string>

  // Grouping folds GROUP_MIN or more leaves of one type under one
  // parent and relationship into a collection node.
  grouping?: boolean

  // remoteImages lets a picture from another site be fetched. Off, only
  // files served by this install are drawn, so opening a sketch does
  // not tell every avatar host who is looking.
  remoteImages?: boolean
}>()

const emit = defineEmits<{
  (e: 'select', id: string, additive: boolean): void
  (e: 'selectMany', ids: string[], additive: boolean): void
  (e: 'selectEdge', id: string): void
  (e: 'clear'): void
  (e: 'moved', positions: Position[]): void
  (e: 'nodeMenu', id: string, at: Point): void
  (e: 'groupMenu', ids: string[], at: Point): void
  (e: 'edgeMenu', id: string, at: Point): void
  (e: 'backgroundMenu', at: Point, graph: Point): void
  (e: 'quickAdd', at: Point, graph: Point): void
  (e: 'linkDrag', from: string, to: string): void
  (e: 'dropType', type: string, graph: Point): void
}>()

// Box selection, switched on by the toolbar or held with S.
const boxSelect = defineModel<boolean>('boxSelect', { default: false })

const FLAG_COLORS: Record<string, string> = {
  red: '#ef4444',
  orange: '#f97316',
  yellow: '#eab308',
  green: '#22c55e',
  blue: '#3b82f6',
}

const catalog = useCatalogStore()
const host = ref<HTMLDivElement | null>(null)
const minimap = ref<HTMLCanvasElement | null>(null)
let graph: ForceGraph<Node, Link> | null = null
let resize: ResizeObserver | null = null

// Hover state for the highlight. Plain variables, the canvas redraws
// every frame anyway.
let hovered: string | null = null
let hoveredLink: string | null = null
const neighbours = new Set<string>()
const adjacency = new Map<string, Set<string>>()

// Types hidden through the legend. Hiding keeps the layout, the nodes
// just stop being drawn.
const hidden = ref(new Set<string>())
const typeOf = new Map<string, string>()

// Alt drag and box selection.
const linking = ref(false)
const dragging = ref<{ from: Node; to: Point } | null>(null)
const box = ref<{ a: Point; b: Point } | null>(null)
let boxHeld = false

const legend = computed(() => {
  const counts = new Map<string, number>()
  for (const n of props.nodes) counts.set(n.type, (counts.get(n.type) ?? 0) + 1)

  return [...counts.entries()]
    .map(([type, count]) => ({ type, count, title: catalog.type(type)?.title ?? type }))
    .sort((a, b) => b.count - a.count || a.title.localeCompare(b.title))
})

function cssVar(name: string, fallback: string): string {
  return getComputedStyle(document.body).getPropertyValue(name).trim() || fallback
}

// The console's own font, so names on the canvas match the panels.
const cssFont = "'Geist Variable', system-ui, sans-serif"

// The theme's colours, read once per frame instead of once per node.
const pal = {
  bg: '#fff',
  chip: '#fff',
  border: '#d4d4d4',
  text: '#0a0a0a',
  muted: '#737373',
  accent: '#ea580c',
}

function readPalette() {
  pal.bg = cssVar('--bg-primary', '#fff')
  pal.chip = cssVar('--bg-popover', pal.bg)
  pal.border = cssVar('--border-strong', '#d4d4d4')
  pal.text = cssVar('--text-primary', '#0a0a0a')
  pal.muted = cssVar('--text-muted', '#737373')
  pal.accent = cssVar('--graph-accent', '#ea580c')
}

function setHidden(next: Set<string>) {
  hidden.value = next
  graph?.nodeVisibility(visibleNode).linkVisibility(visibleLink)
}

// A plain click toggles the type, a click with a modifier keeps only
// it, and the same again brings every type back.
function toggleType(type: string, e?: MouseEvent) {
  if (e && (e.altKey || e.metaKey || e.ctrlKey)) {
    onlyType(type)

    return
  }

  const next = new Set(hidden.value)
  if (next.has(type)) next.delete(type)
  else next.add(type)
  setHidden(next)
}

function onlyType(type: string) {
  const others = legend.value.map((l) => l.type).filter((t) => t !== type)
  const alone = !hidden.value.has(type) && others.every((t) => hidden.value.has(t))
  setHidden(alone ? new Set() : new Set(others))
}

function showAllTypes() {
  setHidden(new Set())
}

function visibleNode(n: Node): boolean {
  return !hidden.value.has(n.type) && !props.hiddenIds?.has(n.id)
}

function endId(end: string | Node): string {
  return typeof end === 'object' ? end.id : end
}

function endType(end: string | Node): string {
  return typeof end === 'object' ? end.type : (typeOf.get(end) ?? '')
}

function visibleLink(l: Link): boolean {
  if (hidden.value.has(endType(l.source)) || hidden.value.has(endType(l.target))) return false
  return !props.hiddenIds?.has(endId(l.source)) && !props.hiddenIds?.has(endId(l.target))
}

// besideNeighbour is a spot a little away from a placed neighbour of id,
// at a random angle so siblings added together spread into a ring.
function besideNeighbour(id: string, known: Map<string, Node>): Point | null {
  for (const other of adjacency.get(id) ?? []) {
    const p = known.get(other)
    if (p?.x === undefined || p.y === undefined) continue
    const angle = Math.random() * 2 * Math.PI
    const dist = 40 + Math.random() * 30
    return { x: p.x + Math.cos(angle) * dist, y: p.y + Math.sin(angle) * dist }
  }
  return null
}

// freeSpot is a place near the middle of the view that no entity
// covers, for a new entity with nothing placed to hang from. Without it
// such an entity started at the origin, wherever that was on screen, and
// the pinned graph pushed it away without bound.
function freeSpot(known: Map<string, Node>): Point {
  const c = graph?.centerAt() ?? { x: 0, y: 0 }
  const taken = [...known.values()].filter((n) => n.x !== undefined && n.y !== undefined)
  const clear = (x: number, y: number) =>
    taken.every((n) => Math.hypot(n.x! - x, n.y! - y) > radius(n) + 24)
  for (let ring = 0; ring < 40; ring++) {
    const dist = ring * 30
    const steps = Math.max(1, ring * 6)
    const turn = Math.random() * 2 * Math.PI
    for (let i = 0; i < steps; i++) {
      const a = turn + (i / steps) * 2 * Math.PI
      const x = c.x + Math.cos(a) * dist
      const y = c.y + Math.sin(a) * dist
      if (clear(x, y)) return { x, y }
    }
  }
  return c
}

// The most connected entities are named at any zoom, they are what a
// reader finds the way by.
const HUBS = 12
const HUB_DEGREE = 4

// GROUP_MIN is how many leaves of one kind under one parent fold into
// a collection. expanded holds the collections opened by a double
// click, until the grouping is switched or the page reloads.
const GROUP_MIN = 12
const expanded = new Set<string>()

type Group = {
  id: string
  parent: string
  label: string
  out: boolean
  type: string
  members: string[]
}

// groupsOf finds the collections: leaves (one relationship) of one
// type hanging from one parent by one relationship, GROUP_MIN or more,
// not hidden by a rule and not expanded.
function groupsOf(degree: Map<string, number>): Group[] {
  if (!props.grouping) return []
  const typeById = new Map(props.nodes.map((n) => [n.id, n.type]))
  const byKey = new Map<string, Group>()
  for (const e of props.edges) {
    for (const [parent, child, out] of [
      [e.source, e.target, true],
      [e.target, e.source, false],
    ] as const) {
      const type = typeById.get(child)
      if (!type || degree.get(child) !== 1 || props.hiddenIds?.has(child)) continue
      const id = `group:${parent}:${out ? 'out' : 'in'}:${e.label}:${type}`
      const g = byKey.get(id) ?? { id, parent, label: e.label, out, type, members: [] }
      g.members.push(child)
      byKey.set(id, g)
    }
  }
  return [...byKey.values()].filter((g) => g.members.length >= GROUP_MIN && !expanded.has(g.id))
}
const hubs = new Set<string>()

// fresh counts the entities the last toData placed for the first time.
let fresh = 0

// toData keeps the simulation's own positions for nodes already drawn,
// so a refresh does not throw the whole layout back into motion.
function toData() {
  const known = new Map<string, Node>()
  for (const n of graph?.graphData().nodes ?? []) known.set(n.id, n)

  typeOf.clear()
  adjacency.clear()
  const degree = new Map<string, number>()
  for (const e of props.edges) {
    degree.set(e.source, (degree.get(e.source) ?? 0) + 1)
    degree.set(e.target, (degree.get(e.target) ?? 0) + 1)
    if (!adjacency.has(e.source)) adjacency.set(e.source, new Set())
    if (!adjacency.has(e.target)) adjacency.set(e.target, new Set())
    adjacency.get(e.source)!.add(e.target)
    adjacency.get(e.target)!.add(e.source)
  }

  const groups = groupsOf(degree)
  const grouped = new Map<string, Group>()
  for (const g of groups) {
    for (const m of g.members) grouped.set(m, g)
    typeOf.set(g.id, g.type)
    adjacency.set(g.id, new Set([g.parent]))
    adjacency.get(g.parent)?.add(g.id)
  }

  const lone: Node[] = []
  const nodes: Node[] = props.nodes
    .filter((n) => !grouped.has(n.id))
    .map((n) => {
      typeOf.set(n.id, n.type)
      const prev = known.get(n.id)
      const node: Node = {
        id: n.id,
        label: n.label,
        type: n.type,
        color: n.color,
        fill: n.color || catalog.color(n.type),
        icon: catalog.type(n.type)?.icon ?? '',
        shape: n.shape,
        flag: n.flag,
        size: n.size,
        image: n.image || pictureOf(n.properties),
        degree: degree.get(n.id) ?? 0,
      }
      if (prev?.x !== undefined) {
        node.x = prev.x
        node.y = prev.y
        node.fx = prev.fx
        node.fy = prev.fy
      } else if (n.x !== null && n.y !== null) {
        node.x = node.fx = n.x
        node.y = node.fy = n.y
      } else {
        // A new entity starts beside the entity it hangs from, so the
        // simulation only has to nudge it out, instead of flinging it in
        // from wherever the origin happens to be.
        fresh++
        const beside = besideNeighbour(n.id, known)
        if (beside) {
          node.x = beside.x
          node.y = beside.y
        } else {
          // Nothing placed to hang from: placed below, as a batch.
          lone.push(node)
        }
      }
      known.set(n.id, node)
      return node
    })

  // Newcomers with nothing to hang from start in view, as a grid from
  // one free spot (a scan per entity was quadratic on a big import).
  // One with no relationship at all stays where it lands.
  if (lone.length) {
    const spot = freeSpot(known)
    const cols = Math.ceil(Math.sqrt(lone.length))
    lone.forEach((node, i) => {
      node.x = spot.x + (i % cols) * 36
      node.y = spot.y + Math.floor(i / cols) * 36
      if (!adjacency.has(node.id)) {
        node.fx = node.x
        node.fy = node.y
      }
    })
  }

  // A collection sits at the middle of its placed members, else beside
  // its parent like any newcomer.
  const byId = new Map(props.nodes.map((n) => [n.id, n]))
  for (const g of groups) {
    const prev = known.get(g.id)
    const title = catalog.type(g.type)?.title ?? g.type
    const node: Node = {
      id: g.id,
      label: `${g.members.length} ${title}`,
      type: g.type,
      color: '',
      fill: catalog.color(g.type),
      icon: '',
      shape: '',
      flag: '',
      size: null,
      image: '',
      degree: g.members.length,
      group: { members: g.members, parent: g.parent },
    }
    const placed = g.members
      .map((m) => byId.get(m))
      .filter((m) => m && m.x !== null && m.y !== null)
    if (prev?.x !== undefined) {
      node.x = prev.x
      node.y = prev.y
      node.fx = prev.fx
      node.fy = prev.fy
    } else if (placed.length) {
      node.x = node.fx = placed.reduce((s, m) => s + (m!.x ?? 0), 0) / placed.length
      node.y = node.fy = placed.reduce((s, m) => s + (m!.y ?? 0), 0) / placed.length
    } else {
      fresh++
      const beside = besideNeighbour(g.id, known)
      if (beside) {
        node.x = beside.x
        node.y = beside.y
      } else lone.push(node)
    }
    known.set(g.id, node)
    nodes.push(node)
  }

  hubs.clear()
  for (const g of groups) hubs.add(g.id)
  for (const n of [...nodes]
    .filter((n) => n.degree >= HUB_DEGREE)
    .sort((a, b) => b.degree - a.degree)
    .slice(0, HUBS))
    hubs.add(n.id)

  const ids = new Set(nodes.map((n) => n.id))
  const links: Link[] = props.edges
    .filter((e) => ids.has(e.source) && ids.has(e.target))
    .map((e) => ({ id: e.id, source: e.source, target: e.target, label: e.label }))
  for (const g of groups) {
    links.push({
      id: g.id + ':link',
      source: g.out ? g.parent : g.id,
      target: g.out ? g.id : g.parent,
      label: g.label,
    })
  }

  return { nodes, links }
}

// radius grows gently with the relationships, unless the node has a
// size of its own.
function radius(n: Node): number {
  if (n.group) return Math.min(30, 8 + 1.6 * Math.log2(n.group.members.length))
  if (n.size) return Math.max(3, Math.min(30, n.size))
  return 3 + 1.4 * Math.log2(1 + n.degree)
}

function shapePath(ctx: CanvasRenderingContext2D, shape: string, x: number, y: number, r: number) {
  ctx.beginPath()
  switch (shape) {
    case 'square':
      ctx.rect(x - r, y - r, 2 * r, 2 * r)
      break
    case 'triangle':
      ctx.moveTo(x, y - r * 1.15)
      ctx.lineTo(x + r * 1.1, y + r * 0.8)
      ctx.lineTo(x - r * 1.1, y + r * 0.8)
      ctx.closePath()
      break
    case 'hexagon':
      for (let i = 0; i < 6; i++) {
        const a = (Math.PI / 3) * i
        const px = x + r * 1.1 * Math.cos(a)
        const py = y + r * 1.1 * Math.sin(a)
        if (i === 0) ctx.moveTo(px, py)
        else ctx.lineTo(px, py)
      }
      ctx.closePath()
      break
    default:
      ctx.arc(x, y, r, 0, 2 * Math.PI)
  }
}

// pictureOf is the image an entity's own properties name, when the
// style sets none: an avatar, a logo, a gravatar.
const pictureKeys = ['profile_picture_url', 'image_url', 'avatar_url', 'logo_url', 'picture', 'src']

function pictureOf(props: Record<string, unknown>): string {
  for (const k of pictureKeys) {
    const v = props[k]
    if (typeof v === 'string' && /^https?:\/\//.test(v)) return v
  }
  return ''
}

// Images drawn inside nodes, loaded once. Asked for with CORS first,
// which keeps the canvas exportable, then without, since most sites
// send no CORS headers. A failed one is remembered so it is not asked
// for every frame.
const images = new Map<string, HTMLImageElement | null>()
let tainted = false

// sameOrigin is a picture this install serves, an attached file.
function sameOrigin(url: string): boolean {
  try {
    return new URL(url, window.location.href).origin === window.location.origin
  } catch {
    return false
  }
}

function picture(url: string): HTMLImageElement | null {
  if (!props.remoteImages && !sameOrigin(url)) return null
  if (images.has(url)) return images.get(url) ?? null
  const img = new Image()
  img.crossOrigin = 'anonymous'
  img.onerror = () => {
    const plain = new Image()
    plain.onload = () => {
      tainted = true
    }
    plain.onerror = () => images.set(url, null)
    plain.src = url
    images.set(url, plain)
  }
  img.src = url
  images.set(url, img)
  return img
}

// toPNG renders the canvas as it is to a PNG data URL, or "" when an
// image drawn without CORS has made the canvas unreadable.
function toPNG(): string {
  const c = host.value?.querySelector('canvas')
  if (!c || tainted) return ''
  try {
    return c.toDataURL('image/png')
  } catch {
    return ''
  }
}

function dimmed(id: string): boolean {
  return hovered !== null && hovered !== id && !neighbours.has(id)
}

// ICON_MIN_PX is the radius on screen from which a node shows its
// type's icon, below it the icon is a smudge.
const ICON_MIN_PX = 8

function draw(node: Node, ctx: CanvasRenderingContext2D, scale: number) {
  // Never under two pixels on screen, or a zoomed out graph is dust.
  const r = Math.max(radius(node), 2 / scale)
  const x = node.x ?? 0
  const y = node.y ?? 0
  const picked = node.group
    ? node.group.members.every((m) => props.selected.has(m))
    : props.selected.has(node.id)

  ctx.globalAlpha = dimmed(node.id) ? 0.15 : 1
  if (node.group) {
    // A stack: two discs peeking out behind, so a collection reads as
    // many at a glance.
    for (const off of [0.36, 0.18]) {
      shapePath(ctx, node.shape, x + off * r, y + off * r, r)
      ctx.fillStyle = node.fill
      ctx.globalAlpha = dimmed(node.id) ? 0.08 : 0.45
      ctx.fill()
      ctx.lineWidth = 1 / scale
      ctx.strokeStyle = pal.bg
      ctx.stroke()
    }
    ctx.globalAlpha = dimmed(node.id) ? 0.15 : 1
  }

  shapePath(ctx, node.shape, x, y, r)
  ctx.fillStyle = node.fill
  ctx.fill()
  // A rim in the background colour keeps touching nodes apart.
  ctx.lineWidth = 1 / scale
  ctx.strokeStyle = pal.bg
  ctx.stroke()
  const img = node.image ? picture(node.image) : null
  if (img && img.complete && img.naturalWidth) {
    ctx.save()
    shapePath(ctx, node.shape, x, y, r)
    ctx.clip()
    ctx.drawImage(img, x - r, y - r, 2 * r, 2 * r)
    ctx.restore()
  } else if (node.icon && r * scale >= ICON_MIN_PX) {
    const icon = iconImage(node.icon, inkOn(node.fill), redraw)
    if (icon) {
      const s = r * 1.1
      ctx.drawImage(icon, x - s / 2, y - s / 2, s, s)
    }
  }

  if (picked || hovered === node.id) {
    // A halo outside the shape, so the type colour stays readable.
    shapePath(ctx, node.shape, x, y, r + 2.5 / scale)
    ctx.lineWidth = 2 / scale
    ctx.strokeStyle = picked ? pal.accent : pal.text
    ctx.stroke()
  }

  if (
    props.litIds &&
    (props.litIds.has(node.id) || node.group?.members.some((m) => props.litIds!.has(m)))
  ) {
    // What the lit scan found, a dashed ring outside the halo.
    shapePath(ctx, node.shape, x, y, r + 5.5 / scale)
    ctx.lineWidth = 1.5 / scale
    ctx.setLineDash([3 / scale, 3 / scale])
    ctx.strokeStyle = pal.accent
    ctx.stroke()
    ctx.setLineDash([])
  }

  if (node.flag && FLAG_COLORS[node.flag]) {
    ctx.beginPath()
    ctx.arc(x + r * 0.8, y - r * 0.8, Math.max(2, r * 0.4), 0, 2 * Math.PI)
    ctx.fillStyle = FLAG_COLORS[node.flag]
    ctx.fill()
    ctx.lineWidth = 1 / scale
    ctx.strokeStyle = pal.bg
    ctx.stroke()
  }

  ctx.globalAlpha = 1
}

// --- Names ---------------------------------------------------------------

// LABEL_ZOOM is the zoom past which every entity in view is named,
// MOCK_ZOOM the one below which the unnamed ones get no placeholder bar.
const LABEL_ZOOM = 2
const MOCK_ZOOM = 0.6
const MAX_LABEL = 36

type Rect = { x: number; y: number; w: number; h: number }

function overlaps(a: Rect, placed: Rect[]): boolean {
  return placed.some(
    (b) => a.x < b.x + b.w && a.x + a.w > b.x && a.y < b.y + b.h && a.y + a.h > b.y,
  )
}

function shortLabel(s: string): string {
  return s.length > MAX_LABEL ? s.slice(0, MAX_LABEL - 1) + '...' : s
}

// widths caches measureText by font and text, the costly call of the
// label pass.
const widths = new Map<string, number>()

function textWidth(ctx: CanvasRenderingContext2D, font: string, text: string): number {
  const k = font + '|' + text
  let w = widths.get(k)
  if (w === undefined) {
    if (widths.size > 5000) widths.clear()
    w = ctx.measureText(text).width
    widths.set(k, w)
  }
  return w
}

// CELL is the bucket size of the shape grid a plain name is checked
// against, in graph units.
const CELL = 120

// drawLabels names the entities in view. Chips (a card with the type's
// colour) for the selection, the hovered entity and the hubs, plain text
// for the hovered entity's neighbours at any zoom and for the rest once
// zoomed in, a grey bar where a
// name would be below that. Higher ranks are placed first and a name
// that would cover a placed one is left out.
function drawLabels(ctx: CanvasRenderingContext2D, scale: number) {
  if (!graph || !host.value) return
  const tl = graph.screen2GraphCoords(0, 0)
  const br = graph.screen2GraphCoords(host.value.clientWidth, host.value.clientHeight)
  const items: { n: Node; rank: number }[] = []
  // The shapes in view, which a plain name must not cover either,
  // bucketed so a name is checked against its own cell only.
  const shapes = new Map<string, (Rect & { id: string })[]>()
  const cell = (x: number, y: number) => Math.floor(x / CELL) + ':' + Math.floor(y / CELL)
  const covers = (rect: Rect, id: string) => {
    for (let x = rect.x; x <= rect.x + rect.w + CELL; x += CELL)
      for (let y = rect.y; y <= rect.y + rect.h + CELL; y += CELL)
        for (const o of shapes.get(cell(x, y)) ?? [])
          if (o.id !== id && overlaps(rect, [o])) return true
    return false
  }
  for (const n of graph.graphData().nodes) {
    if (n.x === undefined || n.y === undefined || !visibleNode(n)) continue
    if (n.x < tl.x - 50 || n.x > br.x + 50 || n.y < tl.y - 50 || n.y > br.y + 50) continue
    const r = radius(n)
    const key = cell(n.x, n.y)
    const bucket = shapes.get(key) ?? []
    bucket.push({ id: n.id, x: n.x - r, y: n.y - r, w: 2 * r, h: 2 * r })
    shapes.set(key, bucket)
    const rank =
      props.selected.has(n.id) || hovered === n.id
        ? 0
        : neighbours.has(n.id)
          ? 1
          : hubs.has(n.id)
            ? 2
            : 3
    if (rank === 3 && dimmed(n.id)) continue
    if (rank === 2 && dimmed(n.id)) continue
    if (rank === 3 && scale < LABEL_ZOOM) {
      if (scale >= MOCK_ZOOM) {
        const w = 14 / scale
        const h = 2.5 / scale
        ctx.fillStyle = pal.muted
        ctx.globalAlpha = 0.3
        ctx.beginPath()
        ctx.roundRect(n.x - w / 2, n.y + radius(n) + 2.5 / scale, w, h, h / 2)
        ctx.fill()
        ctx.globalAlpha = 1
      }
      continue
    }
    items.push({ n, rank })
  }

  items.sort((a, b) => a.rank - b.rank || b.n.degree - a.n.degree)
  const placed: Rect[] = []
  for (const { n, rank } of items.slice(0, 400)) {
    const chip = rank === 0 || rank === 2
    const size = (chip ? 11 : 10) / scale
    const font = `${chip && rank === 0 ? '600 ' : ''}${size}px ${cssFont}`
    ctx.font = font
    const text = shortLabel(n.label)
    const tw = textWidth(ctx, font, text)
    const padX = chip ? 6 / scale : 2 / scale
    const dot = chip ? 7 / scale : 0
    const w = tw + padX * 2 + dot
    const h = size + (chip ? 7 : 3) / scale
    const rect = { x: n.x! - w / 2, y: n.y! + radius(n) + 4 / scale, w, h }
    if (n.id !== hovered && overlaps(rect, placed)) continue
    if (!chip && covers(rect, n.id)) continue
    placed.push(rect)

    if (chip) {
      ctx.beginPath()
      ctx.roundRect(rect.x, rect.y, w, h, 4 / scale)
      ctx.fillStyle = pal.chip
      ctx.globalAlpha = 0.92
      ctx.fill()
      ctx.globalAlpha = 1
      ctx.lineWidth = 1 / scale
      ctx.strokeStyle = rank === 0 && props.selected.has(n.id) ? pal.accent : pal.border
      ctx.stroke()
      ctx.beginPath()
      ctx.arc(rect.x + padX + 2.5 / scale, rect.y + h / 2, 2.5 / scale, 0, 2 * Math.PI)
      ctx.fillStyle = n.fill
      ctx.fill()
    }

    ctx.textAlign = 'left'
    ctx.textBaseline = 'middle'
    ctx.fillStyle = chip ? pal.text : pal.muted
    ctx.fillText(text, rect.x + padX + dot, rect.y + h / 2 + 0.5 / scale)
  }
}

function pointerArea(n: Node, color: string, ctx: CanvasRenderingContext2D, scale: number) {
  ctx.fillStyle = color
  ctx.beginPath()
  ctx.arc(n.x ?? 0, n.y ?? 0, Math.max(radius(n), 2 / scale) + 3 / scale, 0, 2 * Math.PI)
  ctx.fill()
}

// litLink is a wire under the cursor, the selected one, or one touching
// the entity under the cursor.
function litLink(l: Link): boolean {
  if (hoveredLink === l.id || props.selectedEdge === l.id || withinSelection(l)) return true
  return hovered !== null && (endId(l.source) === hovered || endId(l.target) === hovered)
}

// withinSelection is a wire whose both ends are selected, which is how
// a selected path shows its steps.
function withinSelection(l: Link): boolean {
  return (
    props.selected.size > 1 &&
    props.selected.has(endId(l.source)) &&
    props.selected.has(endId(l.target))
  )
}

// drawLinkLabel writes the relationship on a lit wire only. Drawn on
// every wire, a hub with thirty relationships of one kind floods the
// canvas with the same word.
// labelledWires is how many relationships a hovered entity may have
// before its wires stop being labelled, the words only pile up past it.
const labelledWires = 8

function drawLinkLabel(l: Link, ctx: CanvasRenderingContext2D, scale: number) {
  if (typeof l.source !== 'object' || typeof l.target !== 'object') return
  const direct = hoveredLink === l.id || props.selectedEdge === l.id || withinSelection(l)
  if (!direct) {
    if (!litLink(l)) return
    const hub = l.source.id === hovered ? l.source : l.target
    if (hub.degree > labelledWires) return
  }
  const lit = true
  const x = ((l.source.x ?? 0) + (l.target.x ?? 0)) / 2
  const y = ((l.source.y ?? 0) + (l.target.y ?? 0)) / 2
  ctx.font = `${9 / scale}px ${cssFont}`
  ctx.textAlign = 'center'
  ctx.textBaseline = 'middle'
  ctx.fillStyle = lit ? pal.accent : pal.muted
  ctx.fillText(l.label, x, y - 4 / scale)
}

function linkColor(l: Link): string {
  if (hoveredLink === l.id || props.selectedEdge === l.id || withinSelection(l)) return pal.accent
  if (hovered !== null) return litLink(l) ? pal.accent : 'rgba(140,140,140,0.08)'
  return 'rgba(140,140,140,0.35)'
}

function linkWidth(l: Link): number {
  if (hoveredLink === l.id || props.selectedEdge === l.id || withinSelection(l)) return 2
  return hovered !== null && litLink(l) ? 1.2 : 0.7
}

function setHover(n: Node | null) {
  hovered = n?.id ?? null
  neighbours.clear()
  if (n) for (const id of adjacency.get(n.id) ?? []) neighbours.add(id)
  if (host.value) host.value.style.cursor = n ? 'pointer' : ''
  redraw()
}

// redraw asks force-graph for one more frame. Rendering pauses while
// the simulation is still (autoPauseRedraw, or the page burns a core
// doing nothing), so a change of our own state (selection, hover,
// the link being dragged, the theme) has to ask. Setting the
// visibility accessor is the lib's cheapest "something changed".
function redraw() {
  graph?.nodeVisibility(visibleNode)
}

function tooltip(n: Node): string {
  const title = catalog.type(n.type)?.title ?? n.type
  const esc = (s: string) =>
    s.replace(/[&<>]/g, (c) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;' })[c] ?? c)
  if (n.group) return `<b>${esc(n.label)}</b><br>one relationship each, double click to open`
  return `<b>${esc(n.label)}</b><br>${esc(title)}, ${n.degree} ${n.degree === 1 ? 'relationship' : 'relationships'}`
}

// screen turns a mouse event into a point on the canvas and on the graph.
function screen(ev: MouseEvent): { at: Point; graph: Point } {
  const rect = host.value!.getBoundingClientRect()
  const x = ev.clientX - rect.left
  const y = ev.clientY - rect.top
  const g = graph!.screen2GraphCoords(x, y)
  return { at: { x: ev.clientX, y: ev.clientY }, graph: { x: g.x, y: g.y } }
}

// nodeAt is the node under a canvas point, within reach.
function nodeAt(p: Point, reach = 14): Node | null {
  if (!graph) return null
  let best: Node | null = null
  let bestD = reach
  for (const n of graph.graphData().nodes) {
    if (!visibleNode(n) || n.x === undefined || n.y === undefined) continue
    const s = graph.graph2ScreenCoords(n.x, n.y)
    const d = Math.hypot(s.x - p.x, s.y - p.y)
    if (d < bestD) {
      bestD = d
      best = n
    }
  }
  return best
}

let lastBackgroundClick = 0
let lastGroupClick = { id: '', at: 0 }

onMounted(() => {
  if (!host.value) return
  graph = new ForceGraph<Node, Link>(host.value)
    .nodeId('id')
    .nodeLabel(tooltip)
    .nodeCanvasObject(draw)
    .nodePointerAreaPaint(pointerArea)
    .linkLabel(() => '')
    .linkCanvasObjectMode(() => 'after')
    .linkCanvasObject(drawLinkLabel)
    .nodeVisibility(visibleNode)
    .linkVisibility(visibleLink)
    // force-graph places an arrow between the node SURFACES using the
    // radius it derives from nodeVal, so it must know the radius drawn.
    .nodeRelSize(1)
    .nodeVal((n) => radius(n) ** 2)
    .linkDirectionalArrowLength(3.5)
    .linkDirectionalArrowRelPos(1)
    .linkColor(linkColor)
    .linkWidth(linkWidth)
    .onNodeHover(setHover)
    .onLinkHover((l) => {
      hoveredLink = l?.id ?? null
      redraw()
    })
    .onNodeClick((n, ev) => {
      const additive = ev.shiftKey || ev.metaKey || ev.ctrlKey
      if (!n.group) {
        emit('select', n.id, additive)
        return
      }
      // A collection selects its members, a double click opens it.
      const now = Date.now()
      if (lastGroupClick.id === n.id && now - lastGroupClick.at < 400) {
        lastGroupClick = { id: '', at: 0 }
        expand(n.id)
        return
      }
      lastGroupClick = { id: n.id, at: now }
      emit('selectMany', n.group.members, additive)
    })
    .onNodeRightClick((n, ev) => {
      ev.preventDefault()
      if (n.group) emit('groupMenu', n.group.members, screen(ev).at)
      else emit('nodeMenu', n.id, screen(ev).at)
    })
    .onLinkClick((l) => emit('selectEdge', l.id))
    .onLinkRightClick((l, ev) => {
      ev.preventDefault()
      emit('edgeMenu', l.id, screen(ev).at)
    })
    .onBackgroundClick((ev) => {
      const now = Date.now()
      if (now - lastBackgroundClick < 400) {
        lastBackgroundClick = 0
        const s = screen(ev)
        emit('quickAdd', s.at, s.graph)
        return
      }
      lastBackgroundClick = now
      emit('clear')
    })
    .onBackgroundRightClick((ev) => {
      ev.preventDefault()
      const s = screen(ev)
      emit('backgroundMenu', s.at, s.graph)
    })
    .onNodeDragEnd((n) => {
      n.fx = n.x
      n.fy = n.y
      // A collection is the view's, its place is not stored.
      if (!n.group) emit('moved', [{ node_id: n.id, x: n.x ?? 0, y: n.y ?? 0 }])
    })
    .onRenderFramePre(() => {
      if (frame % 60 === 0) readPalette()
    })
    .onRenderFramePost(afterFrame)
    .onEngineStop(settled)

  fresh = 0
  graph.graphData(toData())

  // Nodes keep clear of each other, by their own radius plus a margin.
  graph.d3Force(
    'collide',
    forceCollide<Node>().radius((n) => radius(n) + 4),
  )

  // No centering force: it moves every node toward the origin each tick,
  // the pinned ones snap back, and the few free ones carry the whole
  // shift, which flung newcomers off across the canvas. Positions are
  // stored, the view is fitted instead.
  graph.d3Force('center', null)
  graph.d3Force('gravityX', forceX<Node>(0).strength(0))
  graph.d3Force('gravityY', forceY<Node>(0).strength(0))
  settleFresh()

  resize = new ResizeObserver(() => {
    if (host.value && graph) graph.width(host.value.clientWidth).height(host.value.clientHeight)
  })
  resize.observe(host.value)
  fitOnce()

  window.addEventListener('keydown', onKey)
  window.addEventListener('keyup', onKeyUp)
})

// New entities arrive unpinned beside their parent. The simulation is
// cold by then, so they would sit in a clump until a drag reheated it
// and flung them apart. Instead it is reheated at once with everything
// else pinned, and they are pinned where they come to rest.
watch(
  () => [props.nodes, props.edges],
  () => {
    if (!graph) return
    fresh = 0
    graph.graphData(toData())
    settleFresh()
    fitOnce()
  },
)

// fitOnce fits the view the first time there is something to fit: the
// graph arrives after the canvas mounts, a timer fitted an empty one.
let fitted = false

function fitOnce() {
  if (fitted || !graph || !props.nodes.length) return
  fitted = true
  setTimeout(() => graph?.zoomToFit(400, 60), 50)
}

// settleFresh lets the entities toData placed for the first time find
// their spot and pins them there. With part of the graph already placed
// only what is close pushes the newcomers, the pinned rest would
// otherwise drive them off across the canvas. With nothing placed yet
// (a new sketch, an import) gravity keeps the pieces together.
function settleFresh() {
  if (!graph) return
  if (fresh === 0) {
    // Everything is pinned, the engine has nothing to do: stop it at
    // once instead of the lib's fifteen seconds of cooling down, which
    // kept the page drawing frames after every open.
    graph.cooldownTicks(0)

    return
  }

  const all = fresh === graph.graphData().nodes.length
  charge()?.distanceMax(all ? Infinity : FRESH_REACH)
  gravity(all)
  pinAfterSettle = all ? 'all' : 'fresh'
  graph.cooldownTicks(Infinity).d3ReheatSimulation()
}

// A type's colour edited in the catalog repaints the canvas in place.
watch(
  () => catalog.types,
  () => {
    if (!graph) return
    const own = new Map(props.nodes.map((n) => [n.id, n.color]))
    for (const n of graph.graphData().nodes as Node[]) {
      n.fill = own.get(n.id) || catalog.color(n.type)
      if (!n.group) n.icon = catalog.type(n.type)?.icon ?? ''
    }
    redraw()
  },
)

watch(() => props.remoteImages, redraw)

watch(
  () => props.hiddenIds,
  () => graph?.nodeVisibility(visibleNode).linkVisibility(visibleLink),
)

// rebuild redraws the graph from the props, as a change of the data
// does, keeping every placed position.
function rebuild() {
  if (!graph) return
  fresh = 0
  graph.graphData(toData())
  settleFresh()
}

watch(
  () => props.grouping,
  () => {
    expanded.clear()
    rebuild()
  },
)

// expand opens a collection: its members are drawn on their own, the
// unplaced ones settling beside the parent.
function expand(groupId: string) {
  expanded.add(groupId)
  rebuild()
}

// groupOf is the collection holding id, when it is folded away.
function groupOf(id: string): Node | undefined {
  return graph?.graphData().nodes.find((n) => n.group?.members.includes(id))
}

// The selection and the lit relationship are drawn by us, so a change
// asks for a frame.
watch(() => [props.selected, props.selectedEdge, props.litIds], redraw)

// The theme switches without a frame of its own: read the palette
// again and draw once.
const theme = new MutationObserver(() => {
  readPalette()
  redraw()
})
theme.observe(document.documentElement, { attributes: true, attributeFilter: ['data-theme'] })
const scheme = window.matchMedia('(prefers-color-scheme: dark)')
function onScheme() {
  readPalette()
  redraw()
}
scheme.addEventListener('change', onScheme)

// A type card dragged from the Add panel lands where it is dropped.
function onDrop(ev: DragEvent) {
  const type = ev.dataTransfer?.getData('application/x-gosint-type')
  if (!type || !graph) return
  ev.preventDefault()
  emit('dropType', type, screen(ev).graph)
}

watch(
  () => props.canEdit,
  (can) => graph?.enableNodeDrag(can),
  { immediate: true },
)

// --- Alt drag to link, S for box selection ----------------------------

function typing(ev: KeyboardEvent): boolean {
  const t = ev.target as HTMLElement | null
  return !!t?.closest('input, textarea, select, [contenteditable]')
}

function onKey(ev: KeyboardEvent) {
  if (typing(ev)) return
  if (ev.key === 'Alt' && props.canEdit) linking.value = true
  if ((ev.key === 's' || ev.key === 'S') && !ev.metaKey && !ev.ctrlKey && !boxSelect.value) {
    boxHeld = true
    boxSelect.value = true
  }
}

function onKeyUp(ev: KeyboardEvent) {
  if (ev.key === 'Alt') {
    linking.value = false
    dragging.value = null
  }
  if ((ev.key === 's' || ev.key === 'S') && boxHeld) {
    boxHeld = false
    if (!box.value) boxSelect.value = false
  }
}

// A drag that starts on the overlay is followed on the window: the
// side panel sits over part of the graph, and a drag ending on a node
// under it must still land there instead of dying at the panel's edge.
function follow() {
  const move = (e: MouseEvent) => overlayMove(e)
  const up = (e: MouseEvent) => {
    window.removeEventListener('mousemove', move)
    window.removeEventListener('mouseup', up)
    overlayUp(e)
  }
  window.addEventListener('mousemove', move)
  window.addEventListener('mouseup', up)
}

function overlayDown(ev: MouseEvent) {
  if (ev.button !== 0) return
  const rect = host.value!.getBoundingClientRect()
  const p = { x: ev.clientX - rect.left, y: ev.clientY - rect.top }
  if (linking.value) {
    const n = nodeAt(p)
    if (n) {
      dragging.value = { from: n, to: p }
      follow()
    }
    return
  }
  if (boxSelect.value) {
    box.value = { a: p, b: p }
    follow()
  }
}

function overlayMove(ev: MouseEvent) {
  const rect = host.value!.getBoundingClientRect()
  const p = { x: ev.clientX - rect.left, y: ev.clientY - rect.top }
  if (dragging.value) {
    dragging.value = { ...dragging.value, to: p }
    redraw()
  } else if (box.value) box.value = { a: box.value.a, b: p }
}

function overlayUp(ev: MouseEvent) {
  const rect = host.value!.getBoundingClientRect()
  const p = { x: ev.clientX - rect.left, y: ev.clientY - rect.top }
  if (dragging.value) {
    const to = nodeAt(p, 20)
    const from = dragging.value.from
    dragging.value = null
    redraw()
    if (to && to.id !== from.id) emit('linkDrag', from.id, to.id)
    return
  }
  if (box.value && graph) {
    const { a, b } = box.value
    const x1 = Math.min(a.x, b.x)
    const x2 = Math.max(a.x, b.x)
    const y1 = Math.min(a.y, b.y)
    const y2 = Math.max(a.y, b.y)
    box.value = null
    if (x2 - x1 > 3 && y2 - y1 > 3) {
      const ids: string[] = []
      for (const n of graph.graphData().nodes) {
        if (!visibleNode(n) || n.x === undefined || n.y === undefined) continue
        const s = graph.graph2ScreenCoords(n.x, n.y)
        if (s.x >= x1 && s.x <= x2 && s.y >= y1 && s.y <= y2) ids.push(n.id)
      }
      emit('selectMany', ids, ev.shiftKey || ev.metaKey || ev.ctrlKey)
    }
    if (!boxHeld) boxSelect.value = false
  }
}

const boxStyle = computed(() => {
  if (!box.value) return {}
  const { a, b } = box.value
  return {
    left: Math.min(a.x, b.x) + 'px',
    top: Math.min(a.y, b.y) + 'px',
    width: Math.abs(a.x - b.x) + 'px',
    height: Math.abs(a.y - b.y) + 'px',
  }
})

// --- Frame hooks: the link being dragged and the minimap --------------

let frame = 0

function afterFrame(ctx: CanvasRenderingContext2D, scale: number) {
  drawLabels(ctx, scale)
  if (dragging.value && graph) {
    const { from, to } = dragging.value
    const g = graph.screen2GraphCoords(to.x, to.y)
    ctx.save()
    ctx.setLineDash([4 / scale, 3 / scale])
    ctx.lineWidth = 1.5 / scale
    ctx.strokeStyle = cssVar('--text-primary', '#000')
    ctx.beginPath()
    ctx.moveTo(from.x ?? 0, from.y ?? 0)
    ctx.lineTo(g.x, g.y)
    ctx.stroke()
    ctx.restore()
  }

  if (++frame % 6 === 0) drawMinimap()
}

// minimapFrame is how the last drawn minimap maps graph coordinates
// to its pixels, kept so a click on it maps back.
let minimapFrame: { minX: number; minY: number; k: number; ox: number; oy: number } | null = null

// A click or a drag on the minimap puts that spot in the middle of
// the view.
function minimapJump(ev: MouseEvent) {
  const f = minimapFrame
  if (!f || !graph) return
  graph.centerAt(f.minX + (ev.offsetX - f.ox) / f.k, f.minY + (ev.offsetY - f.oy) / f.k, 0)
}

function minimapDown(ev: MouseEvent) {
  if (ev.button !== 0) return
  ev.preventDefault()
  minimapJump(ev)
  const move = (e: MouseEvent) => {
    if (e.target === minimap.value) minimapJump(e)
  }
  const up = () => {
    window.removeEventListener('mousemove', move)
    window.removeEventListener('mouseup', up)
  }
  window.addEventListener('mousemove', move)
  window.addEventListener('mouseup', up)
}

function drawMinimap() {
  const c = minimap.value
  if (!c || !graph || !host.value) return
  const ctx = c.getContext('2d')
  if (!ctx) return
  const nodes = graph.graphData().nodes.filter((n) => n.x !== undefined && visibleNode(n))
  ctx.clearRect(0, 0, c.width, c.height)
  minimapFrame = null
  if (nodes.length < 2) return

  let minX = Infinity
  let maxX = -Infinity
  let minY = Infinity
  let maxY = -Infinity
  for (const n of nodes) {
    minX = Math.min(minX, n.x!)
    maxX = Math.max(maxX, n.x!)
    minY = Math.min(minY, n.y!)
    maxY = Math.max(maxY, n.y!)
  }
  // The viewport may lie outside the nodes, keep it in the picture.
  const tl = graph.screen2GraphCoords(0, 0)
  const br = graph.screen2GraphCoords(host.value.clientWidth, host.value.clientHeight)
  minX = Math.min(minX, tl.x)
  maxX = Math.max(maxX, br.x)
  minY = Math.min(minY, tl.y)
  maxY = Math.max(maxY, br.y)

  const pad = 8
  const k = Math.min(
    (c.width - 2 * pad) / (maxX - minX || 1),
    (c.height - 2 * pad) / (maxY - minY || 1),
  )
  const ox = pad + (c.width - 2 * pad - (maxX - minX) * k) / 2
  const oy = pad + (c.height - 2 * pad - (maxY - minY) * k) / 2
  const sx = (x: number) => ox + (x - minX) * k
  const sy = (y: number) => oy + (y - minY) * k
  minimapFrame = { minX, minY, k, ox, oy }

  for (const n of nodes) {
    ctx.fillStyle = n.fill
    ctx.beginPath()
    ctx.arc(sx(n.x!), sy(n.y!), 1.5, 0, 2 * Math.PI)
    ctx.fill()
  }

  ctx.strokeStyle = cssVar('--text-secondary', '#555')
  ctx.lineWidth = 1
  ctx.strokeRect(sx(tl.x), sy(tl.y), (br.x - tl.x) * k, (br.y - tl.y) * k)
}

// --- Layouts -----------------------------------------------------------

// FRESH_REACH is how far repulsion reaches while new entities settle.
const FRESH_REACH = 150

function charge(): { distanceMax(d: number): unknown } | undefined {
  return graph?.d3Force('charge') as { distanceMax(d: number): unknown } | undefined
}

// gravity pulls free nodes gently toward the middle of the graph, so a
// force layout keeps separate pieces together. A pull, unlike the
// centering force, never pushes a newcomer. Off outside a force layout.
const GRAVITY = 0.04

function gravity(on: boolean) {
  if (!graph) return
  const nodes = graph.graphData().nodes.filter((n) => n.x !== undefined && n.y !== undefined)
  const cx = nodes.reduce((a, n) => a + n.x!, 0) / Math.max(1, nodes.length)
  const cy = nodes.reduce((a, n) => a + n.y!, 0) / Math.max(1, nodes.length)
  const fx = graph.d3Force('gravityX') as ReturnType<typeof forceX<Node>> | undefined
  const fy = graph.d3Force('gravityY') as ReturnType<typeof forceY<Node>> | undefined
  fx?.x(cx).strength(on ? GRAVITY : 0)
  fy?.y(cy).strength(on ? GRAVITY : 0)
}

// pinAfterSettle says what the next engine stop pins: everything after
// a force layout, or only the entities still free after new ones came.
let pinAfterSettle: 'no' | 'all' | 'fresh' = 'no'

function allPositions(): Position[] {
  return (graph?.graphData().nodes ?? [])
    .filter((n) => n.x !== undefined && n.y !== undefined)
    .map((n) => ({ node_id: n.id, x: n.x!, y: n.y! }))
}

function settled() {
  if (pinAfterSettle === 'no' || !graph) return
  const mode = pinAfterSettle
  pinAfterSettle = 'no'
  charge()?.distanceMax(Infinity)
  gravity(false)
  const pinned: Position[] = []
  for (const n of graph.graphData().nodes) {
    if (mode === 'all' || n.fx === undefined) {
      n.fx = n.x
      n.fy = n.y
      if (n.x !== undefined && n.y !== undefined && !n.group)
        pinned.push({ node_id: n.id, x: n.x, y: n.y })
    }
  }
  emit('moved', pinned)
  graph.cooldownTicks(0)
  if (mode === 'all') graph.zoomToFit(400, 60)
}

// layoutForce lets the simulation place everything again, then pins
// the result.
function layoutForce() {
  if (!graph) return
  for (const n of graph.graphData().nodes) {
    n.fx = undefined
    n.fy = undefined
  }
  charge()?.distanceMax(Infinity)
  gravity(true)
  pinAfterSettle = 'all'
  graph.cooldownTicks(Infinity).d3ReheatSimulation()
}

// layoutHierarchy draws the graph as a compact tree, top down, a
// parent's leaves packed in a grid under it (treeLayout).
function layoutHierarchy() {
  if (!graph) return
  const data = graph.graphData()
  const spots = treeLayout(
    data.nodes.map((n) => ({ id: n.id, degree: n.degree, type: n.type, label: n.label })),
    data.links.map((l) => ({ source: endId(l.source), target: endId(l.target) })),
  )
  for (const n of data.nodes) {
    const p = spots.get(n.id)
    if (!p) continue
    n.x = n.fx = p.x
    n.y = n.fy = p.y
  }
  graph.d3ReheatSimulation()
  emit('moved', allPositions())
  setTimeout(() => graph?.zoomToFit(400, 60), 50)
}

// --- Viewport ---------------------------------------------------------

function fit() {
  graph?.zoomToFit(400, 60)
}

function zoomBy(factor: number) {
  if (graph) graph.zoom(graph.zoom() * factor, 250)
}

// MAX_FIT_ZOOM keeps a fit on one entity from filling the view with it.
const MAX_FIT_ZOOM = 4

function zoomToSelection(ids: Set<string>) {
  if (!graph || !ids.size) return
  graph.zoomToFit(400, 80, (n) => ids.has(n.id) || !!n.group?.members.some((m) => ids.has(m)))
  window.setTimeout(() => {
    if (graph && graph.zoom() > MAX_FIT_ZOOM) graph.zoom(MAX_FIT_ZOOM, 200)
  }, 420)
}

function center(id: string) {
  const n = graph?.graphData().nodes.find((x) => x.id === id)
  if (n && n.x !== undefined && n.y !== undefined) {
    graph?.centerAt(n.x, n.y, 400)
    graph?.zoom(3, 400)
    return
  }
  // Folded into a collection: open it first.
  const g = groupOf(id)
  if (g) {
    expand(g.id)
    window.setTimeout(() => center(id), 50)
  }
}

// nodeIdsVisible lists what the legend has not hidden, for select all.
function nodeIdsVisible(): string[] {
  return props.nodes
    .filter((n) => !hidden.value.has(n.type) && !props.hiddenIds?.has(n.id))
    .map((n) => n.id)
}

defineExpose({
  fit,
  center,
  expand,
  zoomBy,
  zoomToSelection,
  layoutForce,
  layoutHierarchy,
  nodeIdsVisible,
  toPNG,
})

onBeforeUnmount(() => {
  theme.disconnect()
  scheme.removeEventListener('change', onScheme)
  window.removeEventListener('keydown', onKey)
  window.removeEventListener('keyup', onKeyUp)
  resize?.disconnect()
  graph?._destructor()
  graph = null
})
</script>

<template>
  <div class="wrap" @dragover.prevent @drop="onDrop">
    <div ref="host" class="canvas"></div>

    <div
      v-if="linking || boxSelect"
      class="overlay"
      :class="{ crosshair: boxSelect, linking }"
      @mousedown="overlayDown"
    >
      <div v-if="box" class="box" :style="boxStyle"></div>
    </div>

    <div v-if="linking || boxSelect" class="mode text-xs">
      {{ linking ? 'Drag from one entity to another to link them' : 'Drag a box to select' }}
    </div>

    <canvas
      ref="minimap"
      class="minimap"
      width="160"
      height="100"
      title="Click or drag to move the view"
      @mousedown.stop="minimapDown"
      @contextmenu.prevent.stop
    ></canvas>

    <div v-if="legend.length > 1" class="legend">
      <button
        v-for="l in legend"
        :key="l.type"
        class="legend-item"
        :class="{ off: hidden.has(l.type) }"
        :title="
          (hidden.has(l.type) ? 'Show ' : 'Hide ') + l.title + '. Alt-click to show only ' + l.title
        "
        @click="toggleType(l.type, $event)"
      >
        <span class="dot" :style="{ background: catalog.color(l.type) }"></span>
        {{ l.title }}
        <span class="count">{{ l.count }}</span>
      </button>
      <button
        v-if="hidden.size > 0"
        class="legend-item all"
        title="Show every type"
        @click="showAllTypes"
      >
        Show all
      </button>
    </div>
  </div>
</template>

<style scoped>
.wrap {
  position: relative;
  width: 100%;
  height: 100%;
}

.canvas {
  width: 100%;
  height: 100%;
  background: var(--bg-primary);
}

.overlay {
  position: absolute;
  inset: 0;
  z-index: 5;
}

.overlay.crosshair {
  cursor: crosshair;
}

.overlay.linking {
  cursor: alias;
}

.box {
  position: absolute;
  border: 1px dashed var(--text-secondary);
  background: rgba(120, 120, 120, 0.12);
  pointer-events: none;
}

.mode {
  position: absolute;
  top: 12px;
  left: 50%;
  transform: translateX(-50%);
  padding: 4px 10px;
  border-radius: 999px;
  background: var(--bg-popover);
  border: 1px solid var(--border-primary);
  color: var(--text-secondary);
  pointer-events: none;
}

.minimap {
  position: absolute;
  right: 12px;
  bottom: 12px;
  border: 1px solid var(--border-primary);
  border-radius: var(--radius-sm);
  background: var(--bg-popover);
  opacity: 0.9;
  cursor: crosshair;
}

.legend {
  position: absolute;
  left: 12px;
  bottom: 12px;
  display: flex;
  flex-wrap: wrap;
  gap: 4px;
  max-width: 60%;
}

.legend-item {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 3px 8px;
  font-size: 11px;
  border: 1px solid var(--border-primary);
  border-radius: 999px;
  background: var(--bg-popover);
  color: var(--text-secondary);
  cursor: pointer;
}

.legend-item.off {
  opacity: 0.45;
  text-decoration: line-through;
}

.legend-item.all {
  color: var(--text-primary);
}

.legend-item .dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
}

.legend-item .count {
  color: var(--text-muted);
}
</style>
