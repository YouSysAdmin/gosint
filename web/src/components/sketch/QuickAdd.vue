<script setup lang="ts">
// Adding an entity where the canvas was double-clicked: type a value,
// its type is recognized as you type, Enter adds it at that spot. Typing
// a type's name offers that type, picking it keeps it as a chip and the
// text that follows is the value.
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { detect } from '@/api/catalog'
import { apiErrorMessage } from '@/api/client'
import { addNode } from '@/api/sketches'
import type { Detection, EntityType, GraphNode } from '@/api/types'
import EntityIcon from '@/components/sketch/EntityIcon.vue'
import { useCatalogStore } from '@/stores/catalog'
import { useNotificationStore } from '@/stores/notification'

const props = defineProps<{ sketchId: string; x: number; y: number }>()

// place keeps the box inside the window.
const place = computed(() => ({
  left: Math.max(8, Math.min(props.x, window.innerWidth - 300)) + 'px',
  top: Math.max(8, Math.min(props.y, window.innerHeight - 260)) + 'px',
}))
const emit = defineEmits<{ (e: 'close'): void; (e: 'added', node: GraphNode): void }>()

const catalog = useCatalogStore()
const toast = useNotificationStore()
const value = ref('')
const found = ref<Detection | null>(null)
const chosen = ref<EntityType | null>(null)
const active = ref(0)
const busy = ref(false)
const input = ref<HTMLInputElement | null>(null)
let timer: ReturnType<typeof setTimeout> | null = null

const maxTypes = 6

// matches are the types whose name or title holds the text, exact
// and prefix matches first.
const matches = computed<EntityType[]>(() => {
  if (chosen.value) return []
  const q = value.value.trim().toLowerCase()
  if (!q) return []
  const rank = (t: EntityType) => {
    const names = [t.name.toLowerCase(), t.title.toLowerCase()]
    if (names.includes(q)) return 0
    if (names.some((n) => n.startsWith(q))) return 1
    if (names.some((n) => n.includes(q))) return 2
    return -1
  }
  return catalog.types
    .map((t) => ({ t, r: rank(t) }))
    .filter((m) => m.r >= 0)
    .sort((a, b) => a.r - b.r || a.t.title.localeCompare(b.t.title))
    .slice(0, maxTypes)
    .map((m) => m.t)
})

type Option = { kind: 'value' } | { kind: 'type'; type: EntityType }

// options lists what Enter may do: an exact type name first, then the
// value as detected, then the other matching types.
const options = computed<Option[]>(() => {
  if (chosen.value || !value.value.trim()) return []
  const q = value.value.trim().toLowerCase()
  const types = matches.value.map((t) => ({ kind: 'type', type: t }) as Option)
  const exact = matches.value.findIndex(
    (t) => t.name.toLowerCase() === q || t.title.toLowerCase() === q,
  )
  const val: Option = { kind: 'value' }
  if (exact < 0) return [val, ...types]
  const [first] = types.splice(exact, 1)

  return [first, val, ...types]
})

watch(options, () => (active.value = 0))

watch(value, (v) => {
  if (timer) clearTimeout(timer)
  found.value = null
  const text = v.trim()
  if (!text || chosen.value) return
  timer = setTimeout(async () => {
    try {
      const [d] = await detect([text])
      if (value.value.trim() === text) found.value = d ?? null
    } catch {
      found.value = null
    }
  }, 250)
})

function detectedTitle(): string {
  if (!found.value) return '...'
  if (found.value.error) return 'Phrase'

  return catalog.type(found.value.type)?.title ?? found.value.type
}

function choose(t: EntityType) {
  chosen.value = t
  value.value = ''
  found.value = null
  input.value?.focus()
}

function pick(o: Option | undefined) {
  if (o?.kind === 'type') choose(o.type)
  else void submit()
}

async function submit() {
  const text = value.value.trim()
  if (!text || busy.value) return
  busy.value = true
  try {
    let n: GraphNode
    if (chosen.value) {
      n = await addNode(props.sketchId, { type: chosen.value.name, label: text })
    } else {
      const d = found.value ?? (await detect([text]))[0]
      n =
        d && !d.error
          ? await addNode(props.sketchId, {
              type: d.type,
              label: d.label,
              properties: d.properties,
            })
          : await addNode(props.sketchId, { type: 'phrase', label: text })
    }

    emit('added', n)
  } catch (err) {
    toast.error(apiErrorMessage(err))
  } finally {
    busy.value = false
  }
}

function inputKey(ev: KeyboardEvent) {
  const n = options.value.length
  if (ev.key === 'ArrowDown' && n) {
    ev.preventDefault()
    active.value = (active.value + 1) % n
  } else if (ev.key === 'ArrowUp' && n) {
    ev.preventDefault()
    active.value = (active.value - 1 + n) % n
  } else if (ev.key === 'Backspace' && chosen.value && !value.value) {
    chosen.value = null
  }
}

function key(ev: KeyboardEvent) {
  if (ev.key === 'Escape') emit('close')
}

onMounted(() => {
  void catalog.load()
  document.addEventListener('keydown', key)
  setTimeout(() => input.value?.focus(), 0)
})

onBeforeUnmount(() => {
  document.removeEventListener('keydown', key)
  if (timer) clearTimeout(timer)
})
</script>

<template>
  <form class="quick" :style="place" @submit.prevent="pick(options[active])">
    <div class="row">
      <span v-if="chosen" class="chip text-xs" title="Backspace drops the type">
        <EntityIcon :type="chosen.name" :size="12" />
        {{ chosen.title }}
      </span>
      <input
        ref="input"
        v-model="value"
        class="form-input"
        :placeholder="chosen ? `${chosen.title}...` : 'Type a value or a type...'"
        :disabled="busy"
        @keydown="inputKey"
        @blur="!value.trim() && !chosen && emit('close')"
      />
    </div>
    <ul v-if="options.length" class="options">
      <li
        v-for="(o, i) in options"
        :key="o.kind === 'type' ? o.type.name : '-value'"
        class="option text-xs"
        :class="{ active: i === active }"
        @mousedown.prevent="pick(o)"
        @mouseenter="active = i"
      >
        <template v-if="o.kind === 'value'">
          <span
            class="dot"
            :style="{
              background:
                found && !found.error ? catalog.color(found.type) : 'var(--text-tertiary)',
            }"
          ></span>
          <span class="what">Add "{{ value.trim() }}"</span>
          <span class="hint">as {{ detectedTitle() }}</span>
        </template>
        <template v-else>
          <EntityIcon :type="o.type.name" :size="12" />
          <span class="what">{{ o.type.title }}</span>
          <span class="hint">type</span>
        </template>
      </li>
    </ul>
  </form>
</template>

<style scoped>
.quick {
  position: fixed;
  z-index: 40;
  width: 290px;
  padding: 6px;
  background: var(--bg-popover);
  border: 1px solid var(--border-primary);
  border-radius: var(--radius-md);
  box-shadow: var(--shadow-lg);
}

.row {
  display: flex;
  align-items: center;
  gap: 6px;
}

.row .form-input {
  flex: 1;
  min-width: 0;
}

.chip {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  padding: 2px 6px;
  white-space: nowrap;
  border: 1px solid var(--border-primary);
  border-radius: var(--radius-sm);
  color: var(--text-secondary);
}

.options {
  margin: 6px 0 0;
  padding: 0;
  list-style: none;
}

.option {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 5px 6px;
  border-radius: var(--radius-sm);
  cursor: pointer;
  color: var(--text-primary);
}

.option.active {
  background: var(--bg-hover);
}

.what {
  flex: 1;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.hint {
  white-space: nowrap;
  color: var(--text-tertiary);
}

.dot {
  flex: none;
  width: 8px;
  height: 8px;
  border-radius: 50%;
}
</style>
