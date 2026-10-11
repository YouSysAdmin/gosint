<script setup lang="ts">
// The sketch's left panel: the entities as a list to search (the query
// is the header's "Find an entity" field), filter and
// tick, with the actions on what is ticked, and an Add tab of type
// cards to click or drag onto the canvas.
import { computed, ref, watch } from 'vue'
import type { GraphNode } from '@/api/types'
import EntityIcon from '@/components/sketch/EntityIcon.vue'
import { useCatalogStore } from '@/stores/catalog'

const props = defineProps<{
  nodes: GraphNode[]
  selected: Set<string>
  canEdit: boolean
  // query is the text typed in the header's search field.
  query: string
}>()

const emit = defineEmits<{
  (e: 'focus', id: string): void
  (e: 'toggle', id: string): void
  (e: 'selectAll', ids: string[]): void
  (e: 'unselect', ids: string[]): void
  (e: 'clear'): void
  (e: 'add', type?: string): void
  (e: 'import'): void
  (e: 'transform'): void
  (e: 'delete'): void
  (e: 'askAi'): void
}>()

const catalog = useCatalogStore()
const tab = ref<'entities' | 'add'>('entities')
const typeFilter = ref('')
const typeSearch = ref('')
// The flag colour the list is narrowed to, "" for any.
const flagFilter = ref('')
const FLAG_ORDER = ['red', 'orange', 'yellow', 'green', 'blue']

const flags = computed(() => {
  const counts = new Map<string, number>()
  for (const n of props.nodes) if (n.flag) counts.set(n.flag, (counts.get(n.flag) ?? 0) + 1)
  return FLAG_ORDER.filter((f) => counts.has(f)).map((f) => ({ flag: f, count: counts.get(f)! }))
})

const types = computed(() => {
  const counts = new Map<string, number>()
  for (const n of props.nodes) counts.set(n.type, (counts.get(n.type) ?? 0) + 1)
  return [...counts.entries()].sort((a, b) => b[1] - a[1])
})

// A filter on a flag or a type the sketch no longer has would leave an
// empty list with nothing to click to undo it, so it lets go.
watch(flags, (now) => {
  if (flagFilter.value && !now.some((f) => f.flag === flagFilter.value)) flagFilter.value = ''
})
watch(types, (now) => {
  if (typeFilter.value && !now.some(([t]) => t === typeFilter.value)) typeFilter.value = ''
})

const shown = computed(() => {
  const q = props.query.trim().toLowerCase()
  return props.nodes
    .filter((n) => !typeFilter.value || n.type === typeFilter.value)
    .filter((n) => !flagFilter.value || n.flag === flagFilter.value)
    .filter((n) => !q || n.label.toLowerCase().includes(q) || n.type.includes(q))
    .sort((a, b) => a.label.localeCompare(b.label))
})

const allShown = computed(
  () => shown.value.length > 0 && shown.value.every((n) => props.selected.has(n.id)),
)
const picked = computed(() => props.nodes.filter((n) => props.selected.has(n.id)))
// The first few ticked entities are listed above their actions.
const PICKED_SHOWN = 5

function typeTitle(type: string): string {
  return catalog.type(type)?.title ?? type
}

const sameType = computed(
  () => picked.value.length > 0 && picked.value.every((n) => n.type === picked.value[0].type),
)

function toggleAll() {
  if (allShown.value)
    emit(
      'unselect',
      shown.value.map((n) => n.id),
    )
  else
    emit(
      'selectAll',
      shown.value.map((n) => n.id),
    )
}

const grouped = computed(() => {
  const q = typeSearch.value.trim().toLowerCase()
  return catalog.categories
    .map((c) => ({
      ...c,
      types: catalog.types.filter(
        (t) =>
          t.category === c.key &&
          (!q ||
            t.title.toLowerCase().includes(q) ||
            t.name.includes(q) ||
            t.fields.some((f) => f.name.includes(q))),
      ),
    }))
    .filter((c) => c.types.length)
})

function dragType(ev: DragEvent, type: string) {
  ev.dataTransfer?.setData('application/x-gosint-type', type)
  if (ev.dataTransfer) ev.dataTransfer.effectAllowed = 'copy'
}
</script>

<template>
  <div class="left">
    <div class="tabs small">
      <button class="tab" :class="{ active: tab === 'entities' }" @click="tab = 'entities'">
        Entities <span class="text-muted">{{ nodes.length }}</span>
      </button>
      <button v-if="canEdit" class="tab" :class="{ active: tab === 'add' }" @click="tab = 'add'">
        Add
      </button>
    </div>

    <template v-if="tab === 'entities'">
      <div v-if="picked.length" class="picked">
        <div class="flex items-center gap-2 mb-2">
          <span class="text-sm fw-medium">{{ picked.length }} selected</span>
          <button class="icon-btn ml-auto" title="Clear the selection" @click="emit('clear')">
            &times;
          </button>
        </div>
        <ul class="picked-list">
          <li
            v-for="n in picked.slice(0, PICKED_SHOWN)"
            :key="n.id"
            class="picked-row"
            @click="emit('focus', n.id)"
          >
            <span class="label">{{ n.label }}</span>
            <span class="type-badge" :style="{ '--chip': catalog.color(n.type) }">{{
              typeTitle(n.type)
            }}</span>
          </li>
          <li v-if="picked.length > PICKED_SHOWN" class="text-xs text-muted more">
            and {{ picked.length - PICKED_SHOWN }} more
          </li>
        </ul>
        <div v-if="canEdit" class="flex gap-1 flex-wrap">
          <button
            class="btn btn-sm btn-primary"
            :disabled="!sameType"
            :title="sameType ? 'Run a transform on the selection' : 'The selection mixes types'"
            @click="emit('transform')"
          >
            Transform {{ picked.length }}
          </button>
          <button class="btn btn-sm btn-secondary" @click="emit('askAi')">Ask the assistant</button>
          <button class="btn btn-sm btn-danger" @click="emit('delete')">
            Delete {{ picked.length }}
          </button>
        </div>
      </div>

      <div class="controls">
        <select v-model="typeFilter" class="form-select" title="Only one type">
          <option value="">All types</option>
          <option v-for="[t, c] in types" :key="t" :value="t">
            {{ catalog.type(t)?.title ?? t }} ({{ c }})
          </option>
        </select>
      </div>
      <div v-if="flags.length" class="flags">
        <span class="text-xs text-muted">Flag</span>
        <button
          v-for="f in flags"
          :key="f.flag"
          class="flag-chip text-xs"
          :class="{ on: flagFilter === f.flag }"
          :title="flagFilter === f.flag ? 'Show every flag' : `Only the ${f.flag} flag`"
          @click="flagFilter = flagFilter === f.flag ? '' : f.flag"
        >
          <span class="flag" :class="f.flag"></span>{{ f.count }}
        </button>
      </div>
      <label class="checkbox-label all text-xs">
        <input type="checkbox" :checked="allShown" :disabled="!shown.length" @change="toggleAll" />
        Select the {{ shown.length }} shown
      </label>

      <ul class="list">
        <li
          v-for="n in shown"
          :key="n.id"
          class="row"
          :class="{ on: selected.has(n.id) }"
          @click="emit('focus', n.id)"
        >
          <input
            type="checkbox"
            :checked="selected.has(n.id)"
            @click.stop
            @change="emit('toggle', n.id)"
          />
          <EntityIcon :type="n.type" />
          <span class="label">{{ n.label }}</span>
          <span v-if="n.flag" class="flag" :class="n.flag"></span>
          <span class="type-badge" :style="{ '--chip': catalog.color(n.type) }">{{
            typeTitle(n.type)
          }}</span>
        </li>
        <li v-if="!shown.length" class="text-sm text-muted pad">
          {{ nodes.length ? 'No entity matches.' : 'No entity yet.' }}
        </li>
      </ul>
    </template>

    <template v-else>
      <div class="controls">
        <input v-model="typeSearch" class="form-input" placeholder="Search a type" />
        <button
          class="btn btn-sm btn-secondary"
          title="Import a list or a JSON export"
          @click="emit('import')"
        >
          Import
        </button>
      </div>
      <p class="text-xs text-muted pad">
        Click a type to add one, or drag it onto the canvas to place it there.
      </p>
      <div class="cards">
        <template v-for="c in grouped" :key="c.key">
          <div class="group text-xs text-muted">{{ c.title }}</div>
          <button
            v-for="t in c.types"
            :key="t.name"
            class="type-card"
            draggable="true"
            :title="t.description"
            @click="emit('add', t.name)"
            @dragstart="dragType($event, t.name)"
          >
            <EntityIcon :type="t.name" :size="15" />
            <span class="type-name">{{ t.title }}</span>
            <span class="type-plus text-muted">+</span>
          </button>
        </template>
      </div>
    </template>
  </div>
</template>

<style scoped>
.left {
  display: flex;
  flex-direction: column;
  height: 100%;
  min-height: 0;
}

.tabs.small {
  margin: 8px 12px 0;
  padding: 2px;
}

.tabs.small .tab {
  padding: 3px 10px;
  font-size: 12px;
}

.picked {
  margin: 8px 12px 0;
  padding: 8px 10px;
  border: 1px solid var(--border-primary);
  border-radius: var(--radius-md);
  background: var(--bg-tertiary);
}

.icon-btn {
  border: 0;
  background: none;
  color: var(--text-muted);
  cursor: pointer;
  font-size: 16px;
  line-height: 1;
}

.controls {
  display: flex;
  gap: 6px;
  padding: 8px 12px 4px;
}

.controls .form-input,
.controls .form-select {
  min-width: 0;
  font-size: 12px;
  padding: 4px 8px;
}

.controls .form-select {
  flex: 1;
}

.all {
  margin: 0 12px 4px;
}

.flags {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 4px;
  margin: 0 12px 6px;
}

.flag-chip {
  display: inline-flex;
  align-items: center;
  gap: 5px;
  padding: 1px 7px;
  border: 1px solid var(--border-primary);
  border-radius: 10px;
  background: none;
  color: var(--text-secondary);
  cursor: pointer;
}

.flag-chip.on {
  border-color: var(--text-primary);
  background: var(--bg-active);
  color: var(--text-primary);
}

.flag-chip .flag {
  margin-left: 0;
}

.list {
  flex: 1;
  min-height: 0;
  overflow-y: auto;
  margin: 0;
  padding: 0 8px 8px;
  list-style: none;
}

.row {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 5px 6px;
  border-radius: var(--radius-sm);
  font-size: 13px;
  cursor: pointer;
}

.row:hover {
  background: var(--bg-hover);
}

.row.on {
  background: var(--bg-active);
}

/* A flex child, the box would shrink beside a long label. */
.row input[type='checkbox'] {
  flex: none;
  width: 16px;
  height: 16px;
  margin: 0;
}

.row .label {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  min-width: 0;
}

.flag {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  flex: none;
}

/* The type, tinted with its colour, at the end of a row. */
.type-badge {
  flex: none;
  margin-left: auto;
  max-width: 45%;
  padding: 1px 6px;
  border-radius: 4px;
  border: 1px solid color-mix(in srgb, var(--chip) 40%, transparent);
  background: color-mix(in srgb, var(--chip) 14%, transparent);
  color: var(--text-secondary);
  font-size: 11px;
  line-height: 16px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.row .flag + .type-badge {
  margin-left: 0;
}

.row .flag {
  margin-left: auto;
}

.picked-list {
  list-style: none;
  margin: 0 0 8px;
  padding: 0;
  display: flex;
  flex-direction: column;
  gap: 3px;
}

.picked-row {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 4px 6px;
  border-radius: var(--radius-sm);
  background: var(--bg-primary);
  font-size: 12px;
  cursor: pointer;
}

.picked-row .label {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.picked-list .more {
  padding: 0 6px;
}

.flag.red {
  background: #ef4444;
}
.flag.orange {
  background: #f97316;
}
.flag.yellow {
  background: #eab308;
}
.flag.green {
  background: #22c55e;
}
.flag.blue {
  background: #3b82f6;
}

.pad {
  padding: 4px 12px;
  margin: 0;
}

.cards {
  flex: 1;
  min-height: 0;
  overflow-y: auto;
  padding: 4px 8px 12px;
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.group {
  margin: 10px 4px 2px;
}

.type-card {
  display: flex;
  align-items: center;
  gap: 10px;
  width: 100%;
  padding: 6px 8px;
  border: 1px solid transparent;
  border-radius: var(--radius-sm);
  background: none;
  color: var(--text-primary);
  font-size: 13px;
  text-align: left;
  cursor: grab;
}

.type-card:hover {
  background: var(--bg-hover);
  border-color: var(--border-primary);
}

.type-card:hover .type-plus {
  opacity: 1;
}

.type-card:active {
  cursor: grabbing;
}

.type-name {
  flex: 1;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.type-plus {
  opacity: 0;
}
</style>
