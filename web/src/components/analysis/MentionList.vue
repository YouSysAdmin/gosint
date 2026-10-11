<script setup lang="ts">
// The entities offered while typing @ in an analysis.
import { ref, watch } from 'vue'
import EntityIcon from '@/components/sketch/EntityIcon.vue'

export interface MentionItem {
  id: string
  label: string
  type: string
  sketch: string
  sketchTitle?: string
}

const props = defineProps<{
  items: MentionItem[]
  command: (item: MentionItem) => void
}>()

const index = ref(0)

watch(
  () => props.items,
  () => (index.value = 0),
)

function pick(i: number) {
  const item = props.items[i]
  if (item) props.command(item)
}

// onKeyDown is called by the suggestion plugin, true when handled.
function onKeyDown({ event }: { event: KeyboardEvent }): boolean {
  if (event.key === 'ArrowUp') {
    index.value = (index.value + props.items.length - 1) % Math.max(props.items.length, 1)
    return true
  }
  if (event.key === 'ArrowDown') {
    index.value = (index.value + 1) % Math.max(props.items.length, 1)
    return true
  }
  if (event.key === 'Enter' || event.key === 'Tab') {
    pick(index.value)
    return true
  }
  return false
}

defineExpose({ onKeyDown })
</script>

<template>
  <div class="mentions">
    <button
      v-for="(it, i) in items"
      :key="it.id"
      class="item"
      :class="{ on: i === index }"
      @mousedown.prevent="pick(i)"
      @mouseenter="index = i"
    >
      <EntityIcon :type="it.type" :size="12" />
      <span class="label">{{ it.label }}</span>
      <span v-if="it.sketchTitle" class="text-xs text-muted">{{ it.sketchTitle }}</span>
    </button>
    <div v-if="!items.length" class="text-xs text-muted none">No entity matches</div>
  </div>
</template>

<style scoped>
.mentions {
  display: flex;
  flex-direction: column;
  min-width: 220px;
  max-width: 360px;
  max-height: 240px;
  overflow-y: auto;
  padding: 4px;
  background: var(--bg-popover);
  border: 1px solid var(--border-primary);
  border-radius: var(--radius-md);
  box-shadow: var(--shadow-lg);
}

.item {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 5px 8px;
  border: 0;
  border-radius: var(--radius-sm);
  background: none;
  color: var(--text-primary);
  font-size: 13px;
  text-align: left;
  cursor: pointer;
}

.item.on {
  background: var(--bg-hover);
}

.item .label {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  min-width: 0;
}

.none {
  padding: 6px 8px;
}
</style>
