<script setup lang="ts">
// A button that opens a small menu of actions, for the ones a page
// needs but a person does not reach for every minute. The slot holds
// buttons with the menu-item class (styled in styles.css). Closes on
// a pick, a click outside or Escape. Opened from the keyboard, the
// first item takes the focus, and closing hands it back to the button.
import { nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'

withDefaults(defineProps<{ label?: string; title?: string }>(), { label: '...', title: 'More' })

const open = ref(false)
const host = ref<HTMLElement | null>(null)
const trigger = ref<HTMLButtonElement | null>(null)
const panel = ref<HTMLElement | null>(null)
let byKeyboard = false

function away(ev: MouseEvent) {
  if (host.value && !host.value.contains(ev.target as Node)) open.value = false
}

function key(ev: KeyboardEvent) {
  if (ev.key === 'Escape' && open.value) {
    // The page's own Escape (a cleared selection) is not meant.
    ev.stopPropagation()
    open.value = false
    trigger.value?.focus()
  }
}

function toggle(ev: MouseEvent) {
  // A click raised by Enter or Space on the button has no position.
  byKeyboard = ev.detail === 0
  open.value = !open.value
}

// The slotted buttons are the menu's items, told so for a reader.
watch(open, async (on) => {
  if (!on) return
  await nextTick()
  const items = panel.value?.querySelectorAll<HTMLElement>('.menu-item') ?? []
  items.forEach((el) => el.setAttribute('role', 'menuitem'))
  if (byKeyboard) items[0]?.focus()
})

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
      ref="trigger"
      class="btn btn-sm btn-secondary"
      :title="title"
      aria-haspopup="menu"
      :aria-expanded="open"
      @click="toggle"
    >
      {{ label }}
    </button>
    <div v-if="open" ref="panel" class="menu" role="menu" @click="open = false">
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
  z-index: var(--z-popover);
  min-width: 180px;
  padding: 4px;
  display: flex;
  flex-direction: column;
  background: var(--bg-popover);
  border: 1px solid var(--border-primary);
  border-radius: var(--radius-md);
  box-shadow: var(--shadow-md);
}
</style>
