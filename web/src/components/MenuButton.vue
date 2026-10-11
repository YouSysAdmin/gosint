<script setup lang="ts">
// A button that opens a small menu of actions, for the ones a page
// needs but a person does not reach for every minute. The slot holds
// buttons with the menu-item class. Closes on a pick, a click outside
// or Escape.
import { onBeforeUnmount, onMounted, ref } from 'vue'

withDefaults(defineProps<{ label?: string; title?: string }>(), { label: '...', title: 'More' })

const open = ref(false)
const host = ref<HTMLElement | null>(null)

function away(ev: MouseEvent) {
  if (host.value && !host.value.contains(ev.target as Node)) open.value = false
}

function key(ev: KeyboardEvent) {
  if (ev.key === 'Escape') open.value = false
}

onMounted(() => {
  document.addEventListener('mousedown', away, true)
  document.addEventListener('keydown', key)
})

onBeforeUnmount(() => {
  document.removeEventListener('mousedown', away, true)
  document.removeEventListener('keydown', key)
})
</script>

<template>
  <div ref="host" class="menu-host">
    <button
      class="btn btn-sm btn-secondary"
      :title="title"
      :aria-expanded="open"
      @click="open = !open"
    >
      {{ label }}
    </button>
    <div v-if="open" class="menu" @click="open = false">
      <slot />
    </div>
  </div>
</template>

<style scoped>
.menu-host {
  position: relative;
}

.menu {
  position: absolute;
  right: 0;
  top: calc(100% + 4px);
  z-index: 20;
  min-width: 180px;
  padding: 4px;
  display: flex;
  flex-direction: column;
  background: var(--bg-popover);
  border: 1px solid var(--border-primary);
  border-radius: var(--radius-md);
  box-shadow: var(--shadow-md);
}

.menu :deep(.menu-item) {
  padding: 7px 10px;
  text-align: left;
  font-size: 13px;
  border: 0;
  border-radius: var(--radius-sm);
  background: none;
  color: var(--text-primary);
  cursor: pointer;
  font-family: inherit;
}

.menu :deep(.menu-item:hover) {
  background: var(--bg-hover);
}

.menu :deep(.menu-item.danger) {
  color: var(--danger-fg);
}

.menu :deep(.menu-sep) {
  height: 1px;
  margin: 4px 0;
  background: var(--border-primary);
}
</style>
