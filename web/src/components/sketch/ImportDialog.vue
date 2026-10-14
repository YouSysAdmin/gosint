<script setup lang="ts">
// Imports entities: lines of text, each recognized as a type, a sketch
// export, an nmap XML report, a nuclei report or a table mapped to
// entities.
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
  type TableMapping,
  type TablePreview,
} from '@/api/sketches'
import type { Detection, ImportResult } from '@/api/types'
import BaseModal from '@/components/BaseModal.vue'
import ImportTableMapper from '@/components/sketch/ImportTableMapper.vue'
import { useNotificationStore } from '@/stores/notification'

const props = defineProps<{ sketchId: string }>()
const emit = defineEmits<{ (e: 'close'): void; (e: 'imported'): void }>()

const toast = useNotificationStore()
const text = ref('')
const rows = ref<(Detection & { include: boolean })[]>([])
const result = ref<ImportResult | null>(null)
const busy = ref(false)
const passphrase = ref('')

// MAX_LINES is what one recognition sends, the rest of the text waits.
const MAX_LINES = 10000

// A table: the file, its preview, and the mapping the mapper drew over
// it, ready once every entity has its main column.
const tableFile = ref<File | null>(null)
const preview = ref<TablePreview | null>(null)
const mapping = ref<TableMapping | null>(null)
const mappingReady = ref(false)

// pickedFile takes the file out of a file input and clears the input,
// so the same file picked again fires a change event.
function pickedFile(ev: Event): File | undefined {
  const el = ev.target as HTMLInputElement
  const file = el.files?.[0]
  el.value = ''
  return file
}

// pickTable previews the file, the mapper drafts the mapping from it.
async function pickTable(ev: Event) {
  const file = pickedFile(ev)
  if (!file) return
  busy.value = true
  try {
    tableFile.value = file
    preview.value = await previewTable(props.sketchId, file)
  } catch (err) {
    toast.error(apiErrorMessage(err))
    tableFile.value = null
  } finally {
    busy.value = false
  }
}

async function importMapped() {
  if (!tableFile.value || !mapping.value) return
  const file = tableFile.value
  const mapped = mapping.value
  await run(() => importTable(props.sketchId, file, mapped))
}

function leaveTable() {
  preview.value = null
  tableFile.value = null
  mapping.value = null
  mappingReady.value = false
}

const lines = computed(() =>
  text.value
    .split('\n')
    .map((l) => l.trim())
    .filter(Boolean),
)

const sent = computed(() => Math.min(lines.value.length, MAX_LINES))

async function analyze() {
  busy.value = true
  try {
    rows.value = (await detect(lines.value.slice(0, MAX_LINES))).map((d) => ({
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
  const file = pickedFile(ev)
  if (!file) return
  await run(() => importSketchFile(props.sketchId, file, passphrase.value))
}

async function importReport(ev: Event) {
  const file = pickedFile(ev)
  if (!file) return
  await run(async () => importNmap(props.sketchId, await file.text()))
}

async function importNucleiReport(ev: Event) {
  const file = pickedFile(ev)
  if (!file) return
  await run(async () => importNuclei(props.sketchId, await file.text()))
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
      <ImportTableMapper
        v-model:mapping="mapping"
        v-model:ready="mappingReady"
        :preview="preview"
      />
    </template>
    <template v-else-if="!rows.length">
      <p class="text-secondary mb-2">One value per line, each type is recognized for you.</p>
      <textarea
        v-model="text"
        class="form-textarea code-font"
        rows="10"
        placeholder="example.com&#10;8.8.8.8&#10;jane@example.com"
      />
      <p v-if="lines.length > MAX_LINES" class="notice notice-warning text-sm mt-2">
        Only the first {{ MAX_LINES }} of {{ lines.length }} values are recognized at once, the rest
        waits for another round.
      </p>
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
      <input
        type="file"
        accept=".json,.gosint,application/json"
        :disabled="busy"
        @change="importFile"
      />
      <p class="text-secondary mt-4 mb-2">
        Or an nmap XML report (<code>nmap -oX</code>): its hosts, open ports and names.
      </p>
      <input
        type="file"
        accept=".xml,text/xml,application/xml"
        :disabled="busy"
        @change="importReport"
      />
      <p class="text-secondary mt-4 mb-2">
        Or a nuclei report (<code>nuclei -jsonl</code> or <code>-json-export</code>): each result an
        unverified finding under the site or host it was found on, coloured by severity, with the
        CVEs it names.
      </p>
      <input
        type="file"
        accept=".jsonl,.json,application/json"
        :disabled="busy"
        @change="importNucleiReport"
      />
      <p class="text-secondary mt-4 mb-2">
        Or a table (CSV or XLSX, the first sheet): you map its columns to entities and draw the
        relationships between them, one set per row.
      </p>
      <input
        type="file"
        accept=".csv,.xlsx,text/csv,application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
        :disabled="busy"
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
        Recognize {{ sent }} values
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
</style>
