<script setup lang="ts">
// The mapping drawn over a table preview: each block is one entity a
// row makes, with a column per field, and the links join two blocks
// under a label. The finished mapping (the shape the server takes) and
// whether it is complete are kept in the parent's v-models.
import { computed, ref, watch, watchEffect } from 'vue'
import type { TableMapping, TablePreview } from '@/api/sketches'
import { useCatalogStore } from '@/stores/catalog'

// extra lists the fields opened for mapping beyond the primary one, so
// a type with thirty fields shows the few that matter.
type Block = { type: string; fields: Record<string, string>; extra: string[] }
type Link = { from: number; to: number; label: string }

const props = defineProps<{ preview: TablePreview }>()
const mapping = defineModel<TableMapping | null>('mapping', { required: true })
const ready = defineModel<boolean>('ready', { required: true })

const catalog = useCatalogStore()
const blocks = ref<Block[]>([])
const links = ref<Link[]>([])

const typeNames = computed(() => catalog.types.map((t) => t.name).sort())

function fieldsOf(type: string) {
  return catalog.type(type)?.fields.filter((f) => f.kind !== 'object') ?? []
}

function primaryOf(type: string): string {
  return catalog.type(type)?.fields.find((f) => f.primary)?.name ?? ''
}

// draft starts a block per column the server recognized, the column
// on the type's main field.
function draft(preview: TablePreview) {
  blocks.value = []
  links.value = []
  for (const g of preview.guesses) {
    const primary = primaryOf(g.type)
    if (primary && !blocks.value.some((b) => b.type === g.type)) {
      blocks.value.push({ type: g.type, fields: { [primary]: g.column }, extra: [] })
    }
  }
  if (!blocks.value.length) addBlock()
}

watch(() => props.preview, draft, { immediate: true })

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

// removeBlock drops the links touching the block and renumbers the
// rest. A link left joining a block to itself goes too, and one block
// alone has no link to draw.
function removeBlock(i: number) {
  blocks.value.splice(i, 1)
  links.value =
    blocks.value.length < 2
      ? []
      : links.value
          .filter((l) => l.from !== i && l.to !== i)
          .map((l) => ({
            ...l,
            from: l.from > i ? l.from - 1 : l.from,
            to: l.to > i ? l.to - 1 : l.to,
          }))
          .filter((l) => l.from !== l.to)
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

// The models follow every edit: ready once each block maps its main
// field and each link joins two blocks under a label, the mapping with
// the labels in the server's shape.
watchEffect(() => {
  ready.value =
    blocks.value.length > 0 &&
    blocks.value.every((b) => !primaryOf(b.type) || b.fields[primaryOf(b.type)]) &&
    links.value.every((l) => l.from !== l.to && l.label.trim())
  mapping.value = {
    entities: blocks.value.map((b) => ({ type: b.type, fields: { ...b.fields } })),
    links: links.value.map((l) => ({
      ...l,
      label: l.label.trim().toUpperCase().replace(/\s+/g, '_'),
    })),
  }
})
</script>

<template>
  <div>
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
  </div>
</template>

<style scoped>
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
