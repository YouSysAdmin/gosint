<script setup lang="ts">
// Rules hiding entities from the graph: a property, a comparison and a
// value, every rule having to match.
import { onBeforeUnmount, onMounted, reactive, ref } from 'vue'
import { needsValue, OPERATORS, type Rule } from '@/composables/graphFilters'

defineProps<{ fields: string[]; hidden: number }>()
const rules = defineModel<Rule[]>({ required: true })
const emit = defineEmits<{ (e: 'close'): void }>()
const el = ref<HTMLElement | null>(null)

// A value being typed is a draft by rule index, committed into the
// rule 200ms after typing stops: every commit filters the whole graph.
const drafts = reactive(new Map<number, string>())
const timers = new Map<number, number>()

function typed(i: number, v: string) {
  drafts.set(i, v)
  window.clearTimeout(timers.get(i))
  timers.set(
    i,
    window.setTimeout(() => commit(i), 200),
  )
}

function commit(i: number) {
  window.clearTimeout(timers.get(i))
  timers.delete(i)
  const v = drafts.get(i)
  drafts.delete(i)
  if (v !== undefined && rules.value[i]) rules.value[i].value = v
}

// flush commits every draft before the rules are renumbered.
function flush() {
  for (const i of [...drafts.keys()]) commit(i)
}

function add() {
  flush()
  rules.value = [...rules.value, { field: 'label', op: 'contains', value: '' }]
}

function remove(i: number) {
  flush()
  rules.value = rules.value.filter((_, j) => j !== i)
}

function clear() {
  flush()
  rules.value = []
}

// The buttons that open the popover are not "outside": closing on
// their mousedown would let their click reopen it at once.
function away(ev: MouseEvent) {
  const t = ev.target as HTMLElement | null
  if (t?.closest('[data-filters-toggle]')) return
  if (el.value && !el.value.contains(ev.target as Node)) emit('close')
}

// Deferred, so the click that opened the popover does not close it,
// and cancelled when the popover is gone before the tick.
let armed = 0
onMounted(() => {
  armed = window.setTimeout(() => document.addEventListener('mousedown', away, true), 0)
})
onBeforeUnmount(() => {
  window.clearTimeout(armed)
  document.removeEventListener('mousedown', away, true)
  flush()
})
</script>

<template>
  <div ref="el" class="pop">
    <div class="flex items-center justify-between mb-2">
      <h4>Filters</h4>
      <span v-if="hidden" class="text-xs text-muted">{{ hidden }} hidden</span>
    </div>
    <p v-if="!rules.length" class="text-sm text-muted mb-2">
      No rule yet. The legend under the graph hides whole types.
    </p>
    <div v-for="(r, i) in rules" :key="i" class="rule">
      <select v-model="r.field" class="form-select">
        <option v-for="f in fields" :key="f" :value="f">{{ f }}</option>
      </select>
      <select v-model="r.op" class="form-select">
        <option v-for="o in OPERATORS" :key="o.value" :value="o.value">{{ o.label }}</option>
      </select>
      <input
        v-if="needsValue(r.op)"
        :value="drafts.get(i) ?? r.value"
        class="form-input"
        :placeholder="r.field === 'flag' ? 'red, orange, yellow, green, blue' : 'value'"
        @input="typed(i, ($event.target as HTMLInputElement).value)"
        @blur="commit(i)"
      />
      <span v-else class="form-input placeholder-cell"></span>
      <button class="x" title="Remove the rule" @click="remove(i)">&times;</button>
    </div>
    <div class="flex gap-2 mt-2">
      <button class="btn btn-sm btn-secondary" @click="add">Add a rule</button>
      <button v-if="rules.length" class="btn btn-sm btn-secondary" @click="clear">Clear</button>
    </div>
  </div>
</template>

<style scoped>
/* Keeps the grid of a rule whose operator takes no value. */
.placeholder-cell {
  visibility: hidden;
}

.pop {
  position: absolute;
  top: 56px;
  left: 12px;
  z-index: 20;
  width: 420px;
  padding: 12px;
  background: var(--bg-popover);
  border: 1px solid var(--border-primary);
  border-radius: var(--radius-md);
  box-shadow: var(--shadow-lg);
}

.rule {
  display: grid;
  grid-template-columns: 1.2fr 1fr 1.4fr auto;
  gap: 6px;
  margin-bottom: 6px;
}

.rule .form-select,
.rule .form-input {
  font-size: 12px;
  padding: 4px 8px;
  min-width: 0;
}

.x {
  border: 0;
  background: none;
  color: var(--text-muted);
  cursor: pointer;
  font-size: 16px;
}
</style>
