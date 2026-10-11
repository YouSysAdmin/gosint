<script setup lang="ts">
// Imports entities: lines of text, each recognized as a type, a sketch
// export, an nmap XML report or a nuclei report.
import { computed, ref } from 'vue'
import { apiErrorMessage } from '@/api/client'
import { detect } from '@/api/catalog'
import {
  importGraph,
  importNmap,
  importNuclei,
  importSketchFile,
  importTable,
  previewTable,
  type TablePreview,
} from '@/api/sketches'
import type { Detection, ImportResult } from '@/api/types'
import BaseModal from '@/components/BaseModal.vue'
import { useCatalogStore } from '@/stores/catalog'
import { useNotificationStore } from '@/stores/notification'

const props = defineProps<{ sketchId: string }>()
const emit = defineEmits<{ (e: 'close'): void; (e: 'imported'): void }>()

const toast = useNotificationStore()
const catalog = useCatalogStore()
const text = ref('')

// A table: the file, its preview, and the mapping drawn over it. Each
// block is one entity a row makes, with a column per field, and the
// links join two blocks under a label.
// extra lists the fields opened for mapping beyond the primary one, so
// a type with thirty fields shows the few that matter.
type Block = { type: string; fields: Record<string, string>; extra: string[] }
type Link = { from: number; to: number; label: string }
const tableFile = ref<File | null>(null)
const preview = ref<TablePreview | null>(null)
const blocks = ref<Block[]>([])
const links = ref<Link[]>([])

const typeNames = computed(() => catalog.types.map((t) => t.name).sort())

function fieldsOf(type: string) {
  return catalog.type(type)?.fields.filter((f) => f.kind !== 'object') ?? []
}

function primaryOf(type: string): string {
  return catalog.type(type)?.fields.find((f) => f.primary)?.name ?? ''
}

// pickTable previews the file and drafts a block per column the server
// recognized, the column on the type's main field.
async function pickTable(ev: Event) {
  const file = (ev.target as HTMLInputElement).files?.[0]
  if (!file) return
  busy.value = true
  try {
    tableFile.value = file
    preview.value = await previewTable(props.sketchId, file)
    blocks.value = []
    links.value = []
    for (const g of preview.value.guesses) {
      const primary = primaryOf(g.type)
      if (primary && !blocks.value.some((b) => b.type === g.type)) {
        blocks.value.push({ type: g.type, fields: { [primary]: g.column }, extra: [] })
      }
    }
    if (!blocks.value.length) addBlock()
  } catch (err) {
    toast.error(apiErrorMessage(err))
    tableFile.value = null
  } finally {
    busy.value = false
  }
}

function addBlock() {
  blocks.value.push({ type: 'domain', fields: {}, extra: [] })
}

// shownFields is the primary field and the ones opened or mapped.
function shownFields(b: Block) {
  return fieldsOf(b.type).filter((f) => f.primary || b.extra.includes(f.name) || b.fields[f.name])
}

function moreFields(b: Block) {
  const shown = new Set(shownFields(b).map((f) => f.name))
  return fieldsOf(b.type).filter((f) => !shown.has(f.name))
}

function pickField(b: Block, ev: Event) {
  const el = ev.target as HTMLSelectElement
  if (el.value && !b.extra.includes(el.value)) b.extra.push(el.value)
  el.value = ''
}

function removeBlock(i: number) {
  blocks.value.splice(i, 1)
  links.value = links.value
    .filter((l) => l.from !== i && l.to !== i)
    .map((l) => ({ ...l, from: l.from > i ? l.from - 1 : l.from, to: l.to > i ? l.to - 1 : l.to }))
}

// retype keeps a column on the new type's main field when the old one
// had it there, the rest of the mapping does not carry over.
function retype(b: Block, type: string) {
  const was = b.fields[primaryOf(b.type)]
  b.type = type
  b.fields = was && primaryOf(type) ? { [primaryOf(type)]: was } : {}
  b.extra = []
}

function setField(b: Block, field: string, column: string) {
  if (column) b.fields[field] = column
  else delete b.fields[field]
}

function addLink() {
  links.value.push({ from: 0, to: Math.min(1, blocks.value.length - 1), label: 'RELATED_TO' })
}

const mappingReady = computed(
  () =>
    blocks.value.length > 0 &&
    blocks.value.every((b) => !primaryOf(b.type) || b.fields[primaryOf(b.type)]) &&
    links.value.every((l) => l.from !== l.to && l.label.trim()),
)

async function importMapped() {
  if (!tableFile.value) return
  const file = tableFile.value
  await run(() =>
    importTable(props.sketchId, file, {
      entities: blocks.value.map((b) => ({ type: b.type, fields: { ...b.fields } })),
      links: links.value.map((l) => ({
        ...l,
        label: l.label.trim().toUpperCase().replace(/\s+/g, '_'),
      })),
    }),
  )
}

function leaveTable() {
  preview.value = null
  tableFile.value = null
  blocks.value = []
  links.value = []
}
const rows = ref<(Detection & { include: boolean })[]>([])
const result = ref<ImportResult | null>(null)
const busy = ref(false)
const passphrase = ref('')

const lines = computed(() =>
  text.value
    .split('\n')
    .map((l) => l.trim())
    .filter(Boolean),
)

async function analyze() {
  busy.value = true
  try {
    rows.value = (await detect(lines.value.slice(0, 10000))).map((d) => ({
      ...d,
      include: !d.error,
    }))
  } catch (err) {
    toast.error(apiErrorMessage(err))
  } finally {
    busy.value = false
  }
}

async function importRows() {
  const nodes = rows.value
    .filter((r) => r.include)
    .map((r) => ({ type: r.type, label: r.label, properties: r.properties }))
  await run(() => importGraph(props.sketchId, nodes))
}

// An export file goes to the server as it is, plain or encrypted with
// its passphrase, and is merged into this sketch.
async function importFile(ev: Event) {
  const file = (ev.target as HTMLInputElement).files?.[0]
  if (!file) return
  await run(() => importSketchFile(props.sketchId, file, passphrase.value))
}

async function importReport(ev: Event) {
  const file = (ev.target as HTMLInputElement).files?.[0]
  if (!file) return
  const xml = await file.text()
  await run(() => importNmap(props.sketchId, xml))
}

async function importNucleiReport(ev: Event) {
  const file = (ev.target as HTMLInputElement).files?.[0]
  if (!file) return
  const report = await file.text()
  await run(() => importNuclei(props.sketchId, report))
}

async function run(fn: () => Promise<ImportResult>) {
  busy.value = true
  try {
    result.value = await fn()
    emit('imported')
  } catch (err) {
    toast.error(apiErrorMessage(err))
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <BaseModal title="Import" size="modal-w760" @close="emit('close')">
    <div v-if="result">
      <p>
        {{ result.nodes_created }} entities and {{ result.edges_created }} relationships imported,
        {{ result.nodes_skipped }} skipped<template
          v-if="result.files_created || result.files_skipped"
          >, {{ result.files_created }} files attached</template
        >.
      </p>
      <ul v-if="result.errors.length" class="text-sm text-danger">
        <li v-for="e in result.errors" :key="e">{{ e }}</li>
      </ul>
    </div>
    <template v-else-if="preview">
      <p class="text-secondary mb-2">
        {{ preview.total }} rows, {{ preview.columns.length }} columns. Each row makes the entities
        below, a column per field. A list field splits on semicolons, pipes or line breaks.
      </p>
      <div class="table-wrapper sample mb-3">
        <table class="table">
          <thead>
            <tr>
              <th v-for="c in preview.columns" :key="c">{{ c }}</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="(r, i) in preview.rows.slice(0, 5)" :key="i">
              <td v-for="(v, j) in r" :key="j" class="text-sm">{{ v }}</td>
            </tr>
          </tbody>
        </table>
      </div>

      <div v-for="(b, i) in blocks" :key="i" class="block mb-3">
        <div class="block-head">
          <span class="text-sm fw-medium">Entity {{ i + 1 }}</span>
          <select
            class="form-select"
            :value="b.type"
            @change="retype(b, ($event.target as HTMLSelectElement).value)"
          >
            <option v-for="t in typeNames" :key="t" :value="t">
              {{ catalog.type(t)?.title ?? t }}
            </option>
          </select>
          <button class="btn btn-sm btn-secondary" @click="removeBlock(i)">Remove</button>
        </div>
        <div class="fields">
          <label v-for="f in shownFields(b)" :key="f.name" class="field">
            <span class="text-xs" :class="f.primary ? 'fw-medium' : 'text-muted'">
              {{ f.title }}<template v-if="f.primary"> *</template>
            </span>
            <select
              class="form-select"
              :value="b.fields[f.name] ?? ''"
              @change="setField(b, f.name, ($event.target as HTMLSelectElement).value)"
            >
              <option value="">-</option>
              <option v-for="c in preview.columns" :key="c" :value="c">{{ c }}</option>
            </select>
          </label>
          <label v-if="moreFields(b).length" class="field">
            <span class="text-xs text-muted">More</span>
            <select class="form-select" value="" @change="pickField(b, $event)">
              <option value="">Add a field...</option>
              <option v-for="f in moreFields(b)" :key="f.name" :value="f.name">
                {{ f.title }}
              </option>
            </select>
          </label>
        </div>
      </div>
      <button class="btn btn-sm btn-secondary mb-3" @click="addBlock">Add an entity</button>

      <div v-if="blocks.length > 1" class="mb-3">
        <div v-for="(l, i) in links" :key="i" class="link-row mb-2">
          <select v-model.number="l.from" class="form-select">
            <option v-for="(b, j) in blocks" :key="j" :value="j">{{ j + 1 }}. {{ b.type }}</option>
          </select>
          <input v-model="l.label" class="form-input code-font" placeholder="RELATED_TO" />
          <select v-model.number="l.to" class="form-select">
            <option v-for="(b, j) in blocks" :key="j" :value="j">{{ j + 1 }}. {{ b.type }}</option>
          </select>
          <button class="btn btn-sm btn-secondary" @click="links.splice(i, 1)">Remove</button>
        </div>
        <button class="btn btn-sm btn-secondary" @click="addLink">Add a relationship</button>
      </div>
    </template>
    <template v-else-if="!rows.length">
      <p class="text-secondary mb-2">One value per line, each type is recognized for you.</p>
      <textarea
        v-model="text"
        class="form-textarea code-font"
        rows="10"
        placeholder="example.com&#10;8.8.8.8&#10;jane@example.com"
      />
      <p class="text-secondary mt-4 mb-2">
        Or an export of a sketch, merged into this one. An encrypted export needs its passphrase
        first.
      </p>
      <input
        v-model="passphrase"
        type="password"
        class="form-input mb-2"
        placeholder="Passphrase of an encrypted export"
        autocomplete="off"
      />
      <input type="file" accept=".json,.gosint,application/json" @change="importFile" />
      <p class="text-secondary mt-4 mb-2">
        Or an nmap XML report (<code>nmap -oX</code>): its hosts, open ports and names.
      </p>
      <input type="file" accept=".xml,text/xml,application/xml" @change="importReport" />
      <p class="text-secondary mt-4 mb-2">
        Or a nuclei report (<code>nuclei -jsonl</code> or <code>-json-export</code>): each result an
        unverified finding under the site or host it was found on, coloured by severity, with the
        CVEs it names.
      </p>
      <input type="file" accept=".jsonl,.json,application/json" @change="importNucleiReport" />
      <p class="text-secondary mt-4 mb-2">
        Or a table (CSV or XLSX, the first sheet): you map its columns to entities and draw the
        relationships between them, one set per row.
      </p>
      <input
        type="file"
        accept=".csv,.xlsx,text/csv,application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
        @change="pickTable"
      />
    </template>
    <div v-else class="table-wrapper">
      <table class="table">
        <thead>
          <tr>
            <th></th>
            <th>Value</th>
            <th>Type</th>
            <th></th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="(r, i) in rows" :key="i">
            <td><input v-model="r.include" type="checkbox" :disabled="!!r.error" /></td>
            <td class="code-font">{{ r.value }}</td>
            <td>{{ r.type }}</td>
            <td class="text-sm text-danger">{{ r.error }}</td>
          </tr>
        </tbody>
      </table>
    </div>
    <template #footer>
      <button v-if="rows.length && !result" class="btn btn-secondary back" @click="rows = []">
        Back
      </button>
      <button v-else-if="preview && !result" class="btn btn-secondary back" @click="leaveTable">
        Back
      </button>
      <button class="btn btn-secondary" @click="emit('close')">
        {{ result ? 'Close' : 'Cancel' }}
      </button>
      <button
        v-if="preview && !result"
        class="btn btn-primary"
        :disabled="!mappingReady || busy"
        @click="importMapped"
      >
        Import {{ preview.total }} rows
      </button>
      <button
        v-else-if="!rows.length && !result"
        class="btn btn-primary"
        :disabled="!lines.length || busy"
        @click="analyze"
      >
        Recognize {{ lines.length }} values
      </button>
      <button v-else-if="!result" class="btn btn-primary" :disabled="busy" @click="importRows">
        Import {{ rows.filter((r) => r.include).length }}
      </button>
    </template>
  </BaseModal>
</template>

<style scoped>
.back {
  margin-right: auto;
}

.sample {
  max-height: 160px;
}

.block {
  padding: 10px;
  border: 1px solid var(--border-primary);
  border-radius: var(--radius-md);
}

.block-head {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 8px;
}

.block-head .form-select {
  max-width: 240px;
}

.fields {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(200px, 1fr));
  gap: 6px 10px;
}

.field {
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.link-row {
  display: flex;
  gap: 6px;
  align-items: center;
}
</style>
