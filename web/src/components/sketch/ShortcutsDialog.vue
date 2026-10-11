<script setup lang="ts">
// What the keyboard and the mouse do on a sketch.
import BaseModal from '@/components/BaseModal.vue'

const emit = defineEmits<{ (e: 'close'): void }>()
import { modKey as mod } from '@/composables/modKey'

const groups = [
  {
    title: 'Mouse',
    rows: [
      ['Click', 'Select an entity, opening its details'],
      ['Shift or ' + mod + ' click', 'Add to the selection'],
      ['Right click', 'Menu of the entity, the selection, the relationship or the canvas'],
      ['Double click the canvas', 'Add an entity where you click'],
      ['Alt drag from one entity to another', 'Link them'],
      ['Drag an entity', 'Move it and keep it there'],
      ['Drag a type card onto the canvas', 'Add one of that type there'],
    ],
  },
  {
    title: 'Keys',
    rows: [
      ['Hold S and drag', 'Select with a box'],
      [mod + ' A', 'Select every shown entity'],
      ['Delete or Backspace', 'Delete the selection'],
      [mod + ' Z', 'Undo your last edit of the graph'],
      [mod + ' Shift Z', 'Redo it'],
      ['Escape', 'Clear the selection, close a menu'],
      [mod + ' B', 'Show or hide the entities panel'],
      [mod + ' D', 'Open or close the console (log and scans)'],
      [mod + ' Shift L', 'Show or hide the notes'],
      [mod + ' G', 'Sketch settings'],
      [mod + ' K', 'This list'],
      [mod + ' J', 'Jump to an investigation, a sketch or a page'],
    ],
  },
]
</script>

<template>
  <BaseModal title="Shortcuts" size="modal-w480" @close="emit('close')">
    <template v-for="g in groups" :key="g.title">
      <h4 class="mb-2 mt-2">{{ g.title }}</h4>
      <dl class="keys">
        <template v-for="[k, what] in g.rows" :key="k">
          <dt>
            <kbd>{{ k }}</kbd>
          </dt>
          <dd>{{ what }}</dd>
        </template>
      </dl>
    </template>
    <template #footer>
      <button class="btn btn-secondary" @click="emit('close')">Close</button>
    </template>
  </BaseModal>
</template>

<style scoped>
.keys {
  display: grid;
  grid-template-columns: auto 1fr;
  gap: 6px 14px;
  margin: 0 0 12px;
  font-size: 13px;
}

.keys dd {
  margin: 0;
  color: var(--text-secondary);
}

kbd {
  padding: 1px 6px;
  border: 1px solid var(--border-primary);
  border-radius: var(--radius-sm);
  background: var(--bg-tertiary);
  font-family: inherit;
  font-size: 12px;
  white-space: nowrap;
}
</style>
