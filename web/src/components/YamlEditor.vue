<script setup lang="ts">
// A YAML editor on CodeMirror: line numbers, highlighting, bracket
// matching, following the console's theme.
import { yaml } from '@codemirror/lang-yaml'
import { oneDark } from '@codemirror/theme-one-dark'
import { Compartment, EditorState } from '@codemirror/state'
import { EditorView, basicSetup } from 'codemirror'
import { onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useThemeStore } from '@/stores/theme'

const props = withDefaults(defineProps<{ modelValue: string; readonly?: boolean }>(), {
  readonly: false,
})
const emit = defineEmits<{ (e: 'update:modelValue', v: string): void }>()

const theme = useThemeStore()
const host = ref<HTMLDivElement | null>(null)
let view: EditorView | null = null
const themeSlot = new Compartment()
const editSlot = new Compartment()

// light keeps CodeMirror's default colours but on the console's surface.
const light = EditorView.theme({
  '&': { backgroundColor: 'var(--bg-primary)', color: 'var(--text-primary)' },
  '.cm-gutters': {
    backgroundColor: 'var(--bg-secondary)',
    color: 'var(--text-muted)',
    border: 'none',
  },
  '.cm-activeLine': { backgroundColor: 'var(--bg-tertiary)' },
  '.cm-activeLineGutter': { backgroundColor: 'var(--bg-tertiary)' },
})

const base = EditorView.theme({
  '&': { fontSize: '13px', height: '100%' },
  '.cm-scroller': { fontFamily: 'var(--font-mono, ui-monospace, monospace)', overflow: 'auto' },
  '&.cm-focused': { outline: '2px solid transparent' },
})

onMounted(() => {
  if (!host.value) return
  view = new EditorView({
    parent: host.value,
    state: EditorState.create({
      doc: props.modelValue,
      extensions: [
        basicSetup,
        yaml(),
        base,
        themeSlot.of(theme.isDark ? oneDark : light),
        editSlot.of(EditorState.readOnly.of(props.readonly)),
        EditorView.updateListener.of((u) => {
          if (u.docChanged) emit('update:modelValue', u.state.doc.toString())
        }),
      ],
    }),
  })
})

// An outside change (a draft from the model) replaces the text, a
// change typed here does not come back.
watch(
  () => props.modelValue,
  (v) => {
    if (view && v !== view.state.doc.toString())
      view.dispatch({ changes: { from: 0, to: view.state.doc.length, insert: v } })
  },
)

watch(
  () => theme.isDark,
  (dark) => view?.dispatch({ effects: themeSlot.reconfigure(dark ? oneDark : light) }),
)

watch(
  () => props.readonly,
  (ro) => view?.dispatch({ effects: editSlot.reconfigure(EditorState.readOnly.of(ro)) }),
)

onBeforeUnmount(() => view?.destroy())
</script>

<template>
  <div ref="host" class="yaml-editor"></div>
</template>

<style scoped>
/* A column so the editor fills the box: a percentage height of a box
   sized by its content resolves to nothing, which left the code's
   background short of the frame. */
.yaml-editor {
  display: flex;
  flex-direction: column;
  min-height: 60vh;
  border: 1px solid var(--border-primary);
  border-radius: var(--radius-md);
  overflow: hidden;
}

/* The editor's own outline is off, the box around it shows the focus
   like an input does. */
.yaml-editor:focus-within {
  border-color: var(--border-focus);
  box-shadow: var(--shadow-focus);
}

.yaml-editor :deep(.cm-editor) {
  flex: 1;
  min-height: 0;
}
</style>
