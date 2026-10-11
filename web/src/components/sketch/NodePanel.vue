<script setup lang="ts">
// The selected entity: its name to rename in place, its properties to
// read or edit through the type's own fields, notes, its look on the
// canvas and its relationships.
import { computed, nextTick, onBeforeUnmount, ref, watch } from 'vue'
import { apiErrorMessage } from '@/api/client'
import { deleteFile, getMail, listFiles, uploadFile } from '@/api/files'
import { searchEntities, type SearchHit } from '@/api/catalog'
import { deleteEdges, updateNode } from '@/api/sketches'
import type { AttachedFile, GraphEdge, GraphNode, MailMessage } from '@/api/types'
import MailViewer from '@/components/MailViewer.vue'
import CustomFields from '@/components/sketch/CustomFields.vue'
import EntityFields from '@/components/sketch/EntityFields.vue'
import EntityIcon from '@/components/sketch/EntityIcon.vue'
import NeighbourMap from '@/components/sketch/NeighbourMap.vue'
import { formatDate } from '@/composables/formatDate'
import { describeOrigin, originDate, originOf, seenOf } from '@/composables/provenance'
import { formFields, fromProperties, patch, type FormValues } from '@/composables/entityForm'
import { CUSTOM_KEY, customOf, typed, type CustomRow } from '@/composables/customFields'
import { useCatalogStore } from '@/stores/catalog'
import { useNotificationStore } from '@/stores/notification'

const props = defineProps<{
  sketchId: string
  node: GraphNode
  edges: GraphEdge[]
  nodes: GraphNode[]
  canEdit: boolean
}>()
const emit = defineEmits<{
  (e: 'changed', n: GraphNode): void
  (e: 'focus', id: string): void
  (e: 'select', ids: string[]): void
  (e: 'link', from: GraphNode): void
  (e: 'clone', from: GraphNode): void
  (e: 'transform'): void
  (e: 'edgesChanged'): void
  (e: 'scan', id: string): void
}>()

// Where else this very entity sits: the same type and key in another
// sketch the account may read, looked up when the panel opens on it.
const elsewhere = ref<SearchHit[]>([])
watch(
  () => props.node.id,
  async (id) => {
    elsewhere.value = []
    const key = props.node.key || props.node.label
    try {
      const found = await searchEntities({ type: props.node.type, key, exclude: props.sketchId })
      // The answer for an entity that is no longer shown is dropped.
      if (props.node.id === id) elsewhere.value = found
    } catch {
      // the panel stands without it
    }
  },
  { immediate: true },
)

// Where the entity came from, and the last run that found it again
// when that was another one.
const origin = computed(() => originOf(props.node.metadata))
const seen = computed(() => {
  const s = seenOf(props.node.metadata)
  return s && s.scan !== origin.value?.scan ? s : null
})

const SWATCHES = [
  '',
  '#ef4444',
  '#f97316',
  '#eab308',
  '#22c55e',
  '#3b82f6',
  '#8b5cf6',
  '#ec4899',
  '#64748b',
]
const SHAPES = ['circle', 'square', 'hexagon', 'triangle']

const catalog = useCatalogStore()
const toast = useNotificationStore()
const tab = ref<'properties' | 'style' | 'files'>('properties')

// Files attached to the entity: screenshots, documents. Loaded when the
// tab opens, since most entities have none.
const files = ref<AttachedFile[]>([])
const filesLoaded = ref(false)
const uploading = ref(false)
const dropping = ref(false)
const viewing = ref<AttachedFile | null>(null)
const viewText = ref('')
const viewMail = ref<MailMessage | null>(null)

// textViewMax bounds what the viewer prints as text.
const textViewMax = 2 << 20

// metaLine sums a file's metadata up: the camera, the moment, the
// place, the author. The full line is the title.
function metaLine(f: AttachedFile, full = false): string {
  const m = f.metadata ?? {}
  const parts: string[] = []
  const camera = [m.camera_make, m.camera_model].filter(Boolean).join(' ')
  if (camera) parts.push(camera)
  if (m.taken_at) parts.push('taken ' + formatDate(String(m.taken_at)))
  if (typeof m.latitude === 'number' && typeof m.longitude === 'number')
    parts.push(`${m.latitude.toFixed(4)}, ${m.longitude.toFixed(4)}`)
  if (m.author) parts.push('by ' + m.author)
  if (m.software) parts.push(String(m.software))
  if (m.created_at && !m.taken_at) parts.push('created ' + formatDate(String(m.created_at)))
  if (m.pages) parts.push(`${m.pages} pages`)
  if (m.width && m.height) parts.push(`${m.width}x${m.height}`)
  const line = parts.join(', ')
  return full || line.length <= 70 ? line : line.slice(0, 67) + '...'
}

// viewKind says how a file opens in the viewer: a picture, a PDF in a
// frame, an email as a message, text fetched and printed, sound or a
// film, or not at all and it downloads.
function viewKind(f: AttachedFile): 'image' | 'pdf' | 'mail' | 'text' | 'audio' | 'video' | '' {
  const ct = f.content_type
  if (f.is_image) return 'image'
  if (ct === 'application/pdf') return 'pdf'
  if (ct === 'message/rfc822' || /\.eml$/i.test(f.name)) return 'mail'
  if (ct.startsWith('audio/')) return 'audio'
  if (ct.startsWith('video/')) return 'video'
  if (
    ct.startsWith('text/') ||
    /\/(json|xml|x-?yaml|javascript|x-sh|x-python)$/.test(ct) ||
    /\.(txt|md|csv|log|json|ya?ml|xml|ini|conf|sh|py|js|ts|go)$/i.test(f.name)
  )
    return 'text'
  return ''
}

function ext(f: AttachedFile): string {
  return f.name.split('.').pop()?.toUpperCase().slice(0, 4) ?? ''
}

// view opens a file in the viewer. Text is read through fetch, which
// does not care that the server marks it as a download.
async function view(f: AttachedFile) {
  viewing.value = f
  viewText.value = ''
  viewMail.value = null
  if (viewKind(f) === 'mail') {
    try {
      const mail = await getMail(props.sketchId, f.id)
      if (viewing.value?.id === f.id) viewMail.value = mail
    } catch (err) {
      viewText.value = apiErrorMessage(err)
    }
    return
  }
  if (viewKind(f) !== 'text') return
  if (f.size > textViewMax) {
    viewText.value = 'The file is too large to show here, download it instead.'
    return
  }
  try {
    const res = await fetch(f.url, { credentials: 'same-origin' })
    const text = await res.text()
    if (viewing.value?.id === f.id) viewText.value = text
  } catch {
    viewText.value = 'The file could not be read.'
  }
}
const picker = ref<HTMLInputElement | null>(null)

async function loadFiles() {
  const id = props.node.id
  try {
    const list = await listFiles(props.sketchId, id)
    // A list that arrives after the panel moved to another entity is
    // dropped, or its files would show (and delete) under that one.
    if (props.node.id !== id) return
    files.value = list
    filesLoaded.value = true
  } catch (err) {
    toast.error(apiErrorMessage(err))
  }
}

// hashPatch is the file's sha256 for the entity's own hash field (a
// photo's hash, a file's hash_sha256), when the type has one and it is
// still empty: the entity is then known by its file.
function hashPatch(f: AttachedFile): Record<string, unknown> {
  const names = (type.value?.fields ?? []).map((x) => x.name)
  const key = names.includes('hash') ? 'hash' : names.includes('hash_sha256') ? 'hash_sha256' : ''
  if (!key || !f.hash || props.node.properties?.[key]) return {}
  return { [key]: f.hash }
}

async function addFiles(list: FileList | File[] | null) {
  if (!list?.length) return
  uploading.value = true
  try {
    let patch: Record<string, unknown> = {}
    for (const f of Array.from(list)) {
      const up = await uploadFile(props.sketchId, props.node.id, f)
      files.value.push(up)
      if (!Object.keys(patch).length) patch = hashPatch(up)
    }
    if (Object.keys(patch).length) {
      await save({ properties: patch })
    } else {
      emit('changed', props.node)
    }
  } catch (err) {
    toast.error(apiErrorMessage(err))
  } finally {
    uploading.value = false
  }
}

async function removeFile(f: AttachedFile) {
  try {
    await deleteFile(props.sketchId, f.id)
    files.value = files.value.filter((x) => x.id !== f.id)
    if (viewing.value?.id === f.id) viewing.value = null
    emit('changed', props.node)
  } catch (err) {
    toast.error(apiErrorMessage(err))
  }
}

function onDrop(ev: DragEvent) {
  dropping.value = false
  void addFiles(ev.dataTransfer?.files ?? null)
}

function fileSize(n: number): string {
  if (n < 1024) return `${n} B`
  if (n < 1024 * 1024) return `${(n / 1024).toFixed(0)} KB`
  return `${(n / 1024 / 1024).toFixed(1)} MB`
}

watch(
  () => [tab.value, props.node.id] as const,
  ([t, id], old) => {
    if (old && old[1] !== id) {
      files.value = []
      filesLoaded.value = false
    }
    if (t === 'files' && !filesLoaded.value) void loadFiles()
  },
)
const editing = ref(false)
const form = ref<FormValues>({})
const error = ref('')
const busy = ref(false)
const renaming = ref(false)
const title = ref('')
const notes = ref('')
const notesState = ref<'saved' | 'dirty' | 'saving'>('saved')
let notesTimer: ReturnType<typeof setTimeout> | null = null

// LONG is where a value is folded behind "Show more".
const LONG = 280
const unfolded = ref(new Set<string>())

// A secret field (a password, a found key) is masked until shown, and
// hidden again on the next entity.
const revealed = ref(new Set<string>())

// Relationship groups opened past their first chips.
const openGroups = ref(new Set<string>())

watch(
  () => props.node.id,
  () => {
    editing.value = false
    renaming.value = false
    unfolded.value = new Set()
    revealed.value = new Set()
    openGroups.value = new Set()
    notes.value = String(props.node.metadata?.notes ?? '')
    notesState.value = 'saved'
  },
  { immediate: true },
)

const type = computed(() => catalog.type(props.node.type))
const fields = computed(() => formFields(type.value))
const declared = computed(() => new Set(type.value?.fields.map((f) => f.name) ?? []))
const props_ = computed(() =>
  Object.entries(props.node.properties ?? {}).filter(
    ([, v]) => v !== null && v !== '' && !(Array.isArray(v) && !v.length),
  ),
)

const byId = computed(() => new Map(props.nodes.map((n) => [n.id, n])))
// colorOf is how the canvas draws an entity, its own colour or its type's.
function colorOf(n: GraphNode): string {
  return n.color || catalog.color(n.type)
}

// neighbours are the entities one relationship away, each once.
const neighbours = computed(() => {
  const seen = new Map<string, GraphNode>()
  for (const l of links.value) if (l.other) seen.set(l.other.id, l.other)
  return [...seen.values()].map((n) => ({
    id: n.id,
    label: n.label,
    type: n.type,
    color: colorOf(n),
    x: n.x,
    y: n.y,
  }))
})

// Relationships grouped by direction and label, so a hub's thirty
// HAS_SUBDOMAIN read as one heading over thirty chips. A long group is
// cut until asked, the map above shows them all.
const LINKS_SHOWN = 24
const linkGroups = computed(() => {
  const groups = new Map<
    string,
    { key: string; out: boolean; label: string; items: typeof links.value }
  >()
  for (const l of links.value) {
    const key = `${l.out ? '>' : '<'}${l.label}`
    if (!groups.has(key)) groups.set(key, { key, out: l.out, label: l.label, items: [] })
    groups.get(key)!.items.push(l)
  }
  return [...groups.values()].sort((a, b) => b.items.length - a.items.length)
})

function shown(g: { key: string; items: typeof links.value }) {
  return openGroups.value.has(g.key) ? g.items : g.items.slice(0, LINKS_SHOWN)
}

function openGroup(key: string) {
  openGroups.value = new Set([...openGroups.value, key])
}

const links = computed(() =>
  props.edges
    .filter((e) => e.source === props.node.id || e.target === props.node.id)
    .map((e) => {
      const out = e.source === props.node.id
      const other = byId.value.get(out ? e.target : e.source)
      return { id: e.id, out, label: e.label, other }
    }),
)

// show renders a value for reading: a list one item per line, an object
// as key: value lines, and a reference to another entity by its value.
function show(v: unknown): string {
  if (Array.isArray(v)) return v.map(show).join('\n')
  if (v && typeof v === 'object') {
    const entries = Object.entries(v as Record<string, unknown>)
    if (entries.length === 1 && typeof entries[0][1] !== 'object') return String(entries[0][1])
    return entries.map(([k, x]) => `${k}: ${show(x)}`).join('\n')
  }

  return String(v)
}

// code is a short value without spaces, an address or a hash, which
// reads better in the monospace font.
function code(text: string): boolean {
  return text.length <= 100 && !/\s/.test(text)
}

function isURL(text: string): boolean {
  return /^https?:\/\/\S+$/i.test(text)
}

function folded(key: string, text: string): boolean {
  return text.length > LONG && !unfolded.value.has(key)
}

function masked(key: string): boolean {
  return !!type.value?.fields.find((f) => f.name === key)?.secret && !revealed.value.has(key)
}

function reveal(key: string) {
  revealed.value = new Set([...revealed.value, key])
}

function unfold(key: string) {
  unfolded.value = new Set([...unfolded.value, key])
}

async function copy(text: string) {
  try {
    await navigator.clipboard.writeText(text)
    toast.success('Copied')
  } catch {
    toast.error('The clipboard is not available')
  }
}

async function save(p: Parameters<typeof updateNode>[2]) {
  busy.value = true
  try {
    emit('changed', await updateNode(props.sketchId, props.node.id, p))
    return true
  } catch (err) {
    error.value = apiErrorMessage(err)
    toast.error(error.value)
    return false
  } finally {
    busy.value = false
  }
}

const renameInput = ref<HTMLInputElement | null>(null)

function startRename() {
  if (!props.canEdit) return
  title.value = props.node.label
  renaming.value = true
  // autofocus fires once per document, so the field is focused by hand.
  void nextTick(() => renameInput.value?.focus())
}

// Enter and the blur the input's removal fires both land here, one
// save goes out.
let renameSent = false

async function rename() {
  if (renameSent) return
  renameSent = true
  const next = title.value.trim()
  renaming.value = false
  try {
    if (next && next !== props.node.label) await save({ label: next })
  } finally {
    renameSent = false
  }
}

const custom = ref<CustomRow[]>([])
const removedCustom = ref<string[]>([])
const customError = ref('')

function removeCustom(i: number) {
  const r = custom.value[i]
  if (r.was !== undefined) removedCustom.value = [...removedCustom.value, r.key]
  custom.value = custom.value.filter((_, j) => j !== i)
}

// customChanges is what the custom rows change, or a message naming a
// bad row.
function customChanges(): Record<string, unknown> | string {
  const out: Record<string, unknown> = {}
  for (const k of removedCustom.value) out[k] = null
  const seen = new Set<string>()
  for (const r of custom.value) r.error = undefined
  for (const r of custom.value) {
    const key = r.key.trim()
    if (r.was === undefined) {
      if (!key && !r.value.trim()) continue
      if (!CUSTOM_KEY.test(key))
        return `"${key}" is not a field name, use letters, digits, spaces, - and _`
      if (declared.value.has(key)) return `${key} is a field of the type, set it above`
      const removed = removedCustom.value.includes(key)
      if (seen.has(key) || (!removed && props.node.properties?.[key] !== undefined))
        return `${key} is there already`
      if (removed) delete out[key]
    }
    seen.add(key)
    if (r.was !== undefined && r.value === r.was) continue
    // Clearing a stored field removes it, a new one needs a value.
    if (r.was !== undefined && r.value.trim() === '') {
      out[key] = null
      continue
    }
    const t = typed(r)
    if ('error' in t) {
      r.error = t.error
      return `${key} is ${t.error}`
    }
    out[key] = t.value
  }
  return out
}

function startEdit() {
  form.value = fromProperties(fields.value, props.node.properties ?? {})
  custom.value = customOf(props.node.properties ?? {}, declared.value)
  removedCustom.value = []
  customError.value = ''
  error.value = ''
  editing.value = true
}

async function saveFields() {
  const before = fromProperties(fields.value, props.node.properties ?? {})
  const extra = customChanges()
  customError.value = ''
  if (typeof extra === 'string') {
    customError.value = extra
    return
  }
  const changes = { ...patch(fields.value, before, form.value), ...extra }
  if (!Object.keys(changes).length || (await save({ properties: changes }))) editing.value = false
}

// Notes live in the entity's metadata and save on their own a moment
// after typing stops.
function onNotes() {
  notesState.value = 'dirty'
  if (notesTimer) clearTimeout(notesTimer)
  notesTimer = setTimeout(saveNotes, 800)
}

async function saveNotes() {
  if (notesState.value !== 'dirty') return
  notesState.value = 'saving'
  try {
    emit(
      'changed',
      await updateNode(props.sketchId, props.node.id, { metadata: { notes: notes.value } }),
    )
    notesState.value = 'saved'
  } catch (err) {
    notesState.value = 'dirty'
    toast.error(apiErrorMessage(err))
  }
}

async function unlink(edgeId: string) {
  try {
    await deleteEdges(props.sketchId, [edgeId])
    emit('edgesChanged')
  } catch (err) {
    toast.error(apiErrorMessage(err))
  }
}

onBeforeUnmount(() => {
  if (notesTimer) clearTimeout(notesTimer)
})
</script>

<template>
  <div class="panel">
    <div class="hero">
      <EntityIcon :type="node.type" :size="28" />
      <div class="min-w-0 flex-1">
        <div class="text-xs text-muted">{{ type?.title ?? node.type }}</div>
        <input
          v-if="renaming"
          ref="renameInput"
          v-model="title"
          class="form-input rename"
          @keydown.enter.prevent="rename"
          @keydown.escape="renaming = false"
          @blur="rename"
        />
        <h3
          v-else
          class="label"
          :class="{ editable: canEdit }"
          :title="canEdit ? 'Click to rename' : ''"
          @click="startRename"
        >
          {{ node.label }}
        </h3>
      </div>
    </div>

    <div v-if="canEdit" class="flex gap-2 mb-3">
      <button class="btn btn-sm btn-primary" @click="emit('transform')">Transform</button>
      <button class="btn btn-sm btn-secondary" @click="emit('link', node)">Link to...</button>
      <button
        class="btn btn-sm btn-secondary"
        title="A new entity of this type with the same custom fields, values left empty"
        @click="emit('clone', node)"
      >
        Clone
      </button>
      <button
        v-if="!editing && fields.length"
        class="btn btn-sm btn-secondary ml-auto"
        @click="startEdit"
      >
        Edit
      </button>
    </div>

    <div class="tabs small mb-3">
      <button class="tab" :class="{ active: tab === 'properties' }" @click="tab = 'properties'">
        Properties
      </button>
      <button
        v-if="canEdit"
        class="tab"
        :class="{ active: tab === 'style' }"
        @click="tab = 'style'"
      >
        Style
      </button>
      <button class="tab" :class="{ active: tab === 'files' }" @click="tab = 'files'">
        {{ files.length ? `Files ${files.length}` : 'Files' }}
      </button>
    </div>

    <template v-if="tab === 'properties'">
      <form v-if="editing" class="mb-4" @submit.prevent="saveFields">
        <EntityFields v-model="form" :fields="fields" :error="error" />
        <CustomFields v-model="custom" :error="customError" @remove="removeCustom" />
        <div class="flex gap-2 justify-end">
          <button type="button" class="btn btn-sm btn-secondary" @click="editing = false">
            Cancel
          </button>
          <button class="btn btn-sm btn-primary" :disabled="busy">Save</button>
        </div>
      </form>

      <dl v-else class="facts stacked">
        <template v-for="[k, v] in props_" :key="k">
          <dt>{{ type?.fields.find((f) => f.name === k)?.title ?? k }}</dt>
          <dd class="row" :class="{ 'code-font': code(show(v)) }">
            <span class="value">
              <template v-if="masked(k)">
                ********
                <a href="#" class="more" @click.prevent="reveal(k)">Show</a>
              </template>
              <template v-else-if="folded(k, show(v))">
                {{ show(v).slice(0, LONG) }}...
                <a href="#" class="more" @click.prevent="unfold(k)">Show more</a>
              </template>
              <a
                v-else-if="isURL(show(v))"
                :href="show(v)"
                target="_blank"
                rel="noopener noreferrer"
                >{{ show(v) }}</a
              >
              <template v-else>{{ show(v) }}</template>
            </span>
            <button class="copy" title="Copy" @click="copy(show(v))">&#10697;</button>
          </dd>
        </template>
        <dt>Added</dt>
        <dd>
          {{ formatDate(node.created_at) }}
          <template v-if="origin">
            <br />
            <span class="text-secondary">{{ describeOrigin(origin) }}</span>
            <a v-if="origin.scan" href="#" class="more" @click.prevent="emit('scan', origin.scan)"
              >scan</a
            >
          </template>
        </dd>
        <template v-if="seen">
          <dt>Last seen</dt>
          <dd>
            {{ originDate(seen) }}
            <br />
            <span class="text-secondary">{{ describeOrigin(seen) }}</span>
            <a v-if="seen.scan" href="#" class="more" @click.prevent="emit('scan', seen.scan)"
              >scan</a
            >
          </dd>
        </template>
      </dl>

      <template v-if="links.length">
        <h4 class="mt-4 mb-2">Neighbours</h4>
        <NeighbourMap
          :center="{
            id: node.id,
            label: node.label,
            type: node.type,
            color: colorOf(node),
            x: node.x,
            y: node.y,
          }"
          :neighbours="neighbours"
          @focus="(id) => emit('focus', id)"
        />
      </template>

      <template v-if="elsewhere.length">
        <h4 class="mt-4 mb-2">Also in</h4>
        <ul class="elsewhere text-sm">
          <li v-for="h in elsewhere" :key="h.id">
            <router-link :to="`/sketches/${h.sketch_id}?focus=${h.id}`">{{
              h.sketch_title
            }}</router-link>
            <span class="text-muted">, {{ h.investigation_name }}</span>
          </li>
        </ul>
      </template>

      <h4 class="mt-4 mb-2">Relationships ({{ links.length }})</h4>
      <p v-if="!links.length" class="text-sm text-muted">
        Not linked to anything yet. Run a transform on it, or link it to another entity.
      </p>
      <div v-for="g in linkGroups" :key="g.key" class="group">
        <div class="group-head text-xs">
          <span>{{ g.out ? '&rarr;' : '&larr;' }} {{ g.label }}</span>
          <button
            class="select-group text-muted"
            :title="`Select these ${g.items.length}`"
            @click="
              emit(
                'select',
                g.items.filter((l) => l.other).map((l) => l.other!.id),
              )
            "
          >
            {{ g.items.length }} &middot; select all
          </button>
        </div>
        <div class="chips">
          <span
            v-for="l in shown(g)"
            :key="l.id"
            class="end"
            :style="{ '--chip': l.other ? colorOf(l.other) : 'var(--text-muted)' }"
            :title="l.other?.label"
          >
            <span class="name" @click="l.other && emit('focus', l.other.id)">{{
              l.other?.label ?? 'unknown'
            }}</span>
            <button
              v-if="canEdit"
              class="unlink"
              title="Remove this relationship"
              @click="unlink(l.id)"
            >
              &times;
            </button>
          </span>
          <button
            v-if="g.items.length > LINKS_SHOWN && !openGroups.has(g.key)"
            class="more-chips text-xs"
            @click="openGroup(g.key)"
          >
            {{ g.items.length - LINKS_SHOWN }} more
          </button>
        </div>
      </div>

      <div class="flex items-center justify-between mt-4 mb-1">
        <h4>Notes</h4>
        <span class="text-xs text-muted">{{
          notesState === 'saved' ? '' : notesState === 'saving' ? 'Saving...' : 'Unsaved'
        }}</span>
      </div>
      <textarea
        v-model="notes"
        class="form-textarea notes"
        rows="3"
        :readonly="!canEdit"
        placeholder="Write something about this entity"
        @input="onNotes"
        @blur="saveNotes"
      />
    </template>

    <template v-else-if="tab === 'style'">
      <div class="style-row">
        <span class="text-sm">Size</span>
        <input
          type="range"
          min="3"
          max="30"
          :value="node.size ?? 6"
          :disabled="busy"
          @change="save({ size: Number(($event.target as HTMLInputElement).value) })"
        />
        <button
          class="btn btn-sm btn-secondary"
          :disabled="!node.size"
          @click="save({ size: null })"
        >
          Auto
        </button>
      </div>
      <div class="style-row">
        <span class="text-sm">Colour</span>
        <span class="swatches">
          <button
            v-for="c in SWATCHES"
            :key="c"
            class="swatch"
            :class="{ on: (node.color || '') === c }"
            :style="{ background: c || catalog.color(node.type) }"
            :title="c ? c : 'The type colour'"
            @click="save({ color: c })"
          ></button>
        </span>
      </div>
      <div class="style-row">
        <span class="text-sm">Shape</span>
        <span class="segmented">
          <button
            v-for="s in SHAPES"
            :key="s"
            :class="{ active: (node.shape || 'circle') === s }"
            @click="save({ shape: s })"
          >
            {{ s }}
          </button>
        </span>
      </div>
      <div class="style-row column">
        <span class="text-sm">Image URL</span>
        <input
          class="form-input"
          :value="node.image"
          placeholder="https://... drawn inside the node"
          @change="save({ image: ($event.target as HTMLInputElement).value.trim() })"
        />
      </div>
    </template>

    <template v-else-if="tab === 'files'">
      <div
        v-if="canEdit"
        class="drop"
        :class="{ over: dropping }"
        @dragover.prevent="dropping = true"
        @dragleave="dropping = false"
        @drop.prevent="onDrop"
      >
        <span class="text-sm text-secondary">
          {{ uploading ? 'Uploading...' : 'Drop files here, a screenshot or a document' }}
        </span>
        <button class="btn btn-sm btn-secondary" :disabled="uploading" @click="picker?.click()">
          Choose files
        </button>
        <input
          ref="picker"
          type="file"
          multiple
          hidden
          @change="addFiles(($event.target as HTMLInputElement).files)"
        />
      </div>

      <p v-if="filesLoaded && !files.length" class="text-sm text-muted">No files attached.</p>

      <ul class="files">
        <li v-for="f in files" :key="f.id" class="file">
          <button v-if="f.is_image" class="thumb" :title="f.name" @click="view(f)">
            <img :src="f.url" :alt="f.name" />
          </button>
          <button
            v-else-if="viewKind(f)"
            class="thumb doc"
            :title="'Open ' + f.name"
            @click="view(f)"
          >
            {{ ext(f) }}
          </button>
          <a v-else class="thumb doc" :href="f.url + '?download=1'" :title="f.name">
            {{ ext(f) }}
          </a>
          <div class="meta">
            <button v-if="viewKind(f)" class="name as-link" @click="view(f)">{{ f.name }}</button>
            <a v-else :href="f.url + '?download=1'" class="name">{{ f.name }}</a>
            <span class="text-xs text-muted">
              {{ fileSize(f.size) }}, {{ formatDate(f.created_at) }}
            </span>
            <span v-if="metaLine(f)" class="text-xs text-secondary" :title="metaLine(f, true)">
              {{ metaLine(f) }}
            </span>
            <button
              v-if="canEdit && f.is_image && node.image !== f.url"
              class="as-picture"
              @click="save({ image: f.url, properties: hashPatch(f) })"
            >
              Use as the picture
            </button>
            <span v-else-if="f.is_image && node.image === f.url" class="text-xs text-muted">
              The entity's picture
            </span>
          </div>
          <button v-if="canEdit" class="remove" title="Remove the file" @click="removeFile(f)">
            &times;
          </button>
        </li>
      </ul>

      <div v-if="viewing" class="lightbox" @click.self="viewing = null">
        <img v-if="viewKind(viewing) === 'image'" :src="viewing.url" :alt="viewing.name" />
        <iframe
          v-else-if="viewKind(viewing) === 'pdf'"
          :src="viewing.url"
          :title="viewing.name"
          class="frame"
        ></iframe>
        <video v-else-if="viewKind(viewing) === 'video'" :src="viewing.url" controls></video>
        <audio v-else-if="viewKind(viewing) === 'audio'" :src="viewing.url" controls></audio>
        <MailViewer
          v-else-if="viewKind(viewing) === 'mail' && viewMail"
          :mail="viewMail"
          :title="viewing.name"
          :raw-url="viewing.url"
        />
        <pre v-else class="sheet">{{ viewText || 'Loading...' }}</pre>
        <div class="lightbox-bar">
          <span>{{ viewing.name }}</span>
          <a :href="viewing.url + '?download=1'" class="btn btn-sm btn-secondary">Download</a>
          <button class="btn btn-sm btn-secondary" @click="viewing = null">Close</button>
        </div>
      </div>
    </template>
  </div>
</template>

<style scoped>
.drop {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 8px;
  padding: 16px;
  margin-bottom: 12px;
  border: 1px dashed var(--border-primary);
  border-radius: var(--radius-md);
  text-align: center;
}

.drop.over {
  border-color: var(--text-primary);
  background: var(--bg-hover);
}

.files {
  list-style: none;
  margin: 0;
  padding: 0;
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.file {
  display: flex;
  align-items: center;
  gap: 10px;
}

.thumb {
  flex: none;
  width: 56px;
  height: 56px;
  padding: 0;
  border: 1px solid var(--border-primary);
  border-radius: var(--radius-sm);
  background: var(--bg-tertiary);
  overflow: hidden;
  cursor: pointer;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  text-decoration: none;
  color: var(--text-secondary);
  font-size: 11px;
  font-weight: 600;
}

.thumb img {
  width: 100%;
  height: 100%;
  object-fit: cover;
}

.meta {
  display: flex;
  flex-direction: column;
  min-width: 0;
  flex: 1;
}

.meta .name {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  color: var(--text-primary);
}

.as-picture {
  align-self: flex-start;
  padding: 0;
  border: 0;
  background: none;
  color: var(--text-link, var(--text-secondary));
  font-size: 12px;
  cursor: pointer;
  text-decoration: underline;
}

.remove {
  flex: none;
  padding: 0 6px;
  border: 0;
  background: none;
  color: var(--text-muted);
  font-size: 18px;
  line-height: 1;
  cursor: pointer;
}

.remove:hover {
  color: var(--danger-fg);
}

.lightbox {
  position: fixed;
  inset: 0;
  z-index: 100;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 12px;
  padding: 24px;
  background: rgba(0, 0, 0, 0.8);
}

.lightbox img,
.lightbox video {
  max-width: 100%;
  max-height: calc(100vh - 100px);
  border-radius: var(--radius-sm);
  background: var(--bg-primary);
}

.lightbox .frame {
  width: min(100%, 1000px);
  height: calc(100vh - 100px);
  border: 0;
  border-radius: var(--radius-sm);
  background: #fff;
}

.lightbox .sheet {
  width: min(100%, 900px);
  max-height: calc(100vh - 100px);
  overflow: auto;
  margin: 0;
  padding: 12px 16px;
  border-radius: var(--radius-sm);
  background: var(--bg-primary);
  color: var(--text-primary);
  font-size: 12px;
  line-height: 1.5;
  white-space: pre-wrap;
  overflow-wrap: anywhere;
}

.as-link {
  padding: 0;
  border: 0;
  background: none;
  color: inherit;
  font: inherit;
  text-align: left;
  cursor: pointer;
}

.as-link:hover {
  text-decoration: underline;
}

.lightbox-bar {
  display: flex;
  align-items: center;
  gap: 8px;
  color: #fff;
}

.panel {
  padding: 16px;
}

.hero {
  display: flex;
  align-items: center;
  gap: 10px;
  margin-bottom: 12px;
}

.min-w-0 {
  min-width: 0;
}

.flex-1 {
  flex: 1;
}

.label {
  margin: 0;
  word-break: break-all;
}

.label.editable {
  cursor: text;
}

.rename {
  font-size: 15px;
  font-weight: 600;
}

.tabs.small {
  padding: 2px;
  width: fit-content;
}

.tabs.small .tab {
  padding: 3px 10px;
  font-size: 12px;
}

/* Properties read as a table: the field on the left, its value on the
   right, a hairline between rows. */
.facts.stacked {
  grid-template-columns: minmax(84px, 36%) 1fr;
  gap: 0;
  font-size: 13px;
  border-top: 1px solid var(--border-secondary);
}

.facts.stacked dt,
.facts.stacked dd {
  margin: 0;
  padding: 7px 0;
  border-bottom: 1px solid var(--border-secondary);
}

.facts.stacked dt {
  padding-right: 10px;
  font-size: 12px;
  color: var(--text-muted);
  overflow-wrap: anywhere;
}

.facts.stacked dd {
  white-space: pre-line;
  overflow-wrap: anywhere;
}

.facts .row {
  display: flex;
  gap: 6px;
  align-items: flex-start;
}

.facts .value {
  min-width: 0;
  flex: 1;
}

.copy {
  border: 0;
  background: none;
  color: var(--text-muted);
  cursor: pointer;
  opacity: 0;
  padding: 0 2px;
}

.facts .row:hover .copy {
  opacity: 1;
}

.more {
  margin-left: 6px;
  white-space: nowrap;
}

.elsewhere {
  margin: 0;
  padding-left: 18px;
}

.notes {
  font-size: 13px;
}

.group {
  margin-bottom: 10px;
}

.select-group {
  border: 0;
  background: none;
  padding: 0;
  font: inherit;
  color: var(--text-muted);
  cursor: pointer;
}

.select-group:hover {
  color: var(--text-primary);
}

.group-head {
  display: flex;
  justify-content: space-between;
  margin-bottom: 5px;
  color: var(--text-secondary);
  font-weight: 500;
  overflow-wrap: anywhere;
}

.chips {
  display: flex;
  flex-wrap: wrap;
  gap: 4px;
}

/* An entity at the other end, tinted with its colour. */
.end {
  display: inline-flex;
  align-items: center;
  max-width: 100%;
  min-width: 0;
  padding: 2px 4px 2px 7px;
  border-radius: 5px;
  border: 1px solid color-mix(in srgb, var(--chip) 45%, transparent);
  background: color-mix(in srgb, var(--chip) 16%, transparent);
  font-size: 12px;
}

.end:hover {
  background: color-mix(in srgb, var(--chip) 30%, transparent);
}

.end .name {
  min-width: 0;
  cursor: pointer;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.more-chips {
  padding: 2px 8px;
  border: 1px dashed var(--border-strong);
  border-radius: 5px;
  background: none;
  color: var(--text-muted);
  cursor: pointer;
}

.unlink {
  padding: 0 4px;
  border: 0;
  background: none;
  color: var(--text-muted);
  font-size: 14px;
  line-height: 1;
  cursor: pointer;
  opacity: 0;
}

.end:hover .unlink {
  opacity: 1;
}

.unlink:hover {
  color: var(--danger-fg);
}

.style-row {
  display: flex;
  align-items: center;
  gap: 10px;
  margin-bottom: 14px;
}

.style-row > .text-sm {
  width: 56px;
  flex: none;
  color: var(--text-secondary);
}

.style-row.column {
  flex-direction: column;
  align-items: stretch;
  gap: 4px;
}

.style-row input[type='range'] {
  flex: 1;
}

.swatches {
  display: flex;
  gap: 6px;
  flex-wrap: wrap;
}

.swatch {
  width: 20px;
  height: 20px;
  border-radius: 50%;
  border: 2px solid transparent;
  cursor: pointer;
}

.swatch.on {
  border-color: var(--text-primary);
}

.segmented {
  display: inline-flex;
  border: 1px solid var(--border-button);
  border-radius: var(--radius-sm);
  overflow: hidden;
}

.segmented button {
  padding: 3px 8px;
  font-size: 12px;
  background: var(--bg-primary);
  color: var(--text-secondary);
  border: 0;
  cursor: pointer;
}

.segmented button + button {
  border-left: 1px solid var(--border-button);
}

.segmented button.active {
  background: var(--bg-active);
  color: var(--text-primary);
}
</style>
