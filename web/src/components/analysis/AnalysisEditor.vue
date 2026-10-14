<script setup lang="ts">
// The analysis editor: rich text on TipTap, with @ to reference an
// entity of the investigation's sketches. A reference is a chip that
// reports a click, so the host can show the entity on its graph.
import Mention from '@tiptap/extension-mention'
import Placeholder from '@tiptap/extension-placeholder'
import StarterKit from '@tiptap/starter-kit'
import type { SuggestionProps } from '@tiptap/suggestion'
import { EditorContent, VueRenderer, useEditor } from '@tiptap/vue-3'
import { onBeforeUnmount, watch } from 'vue'
import MentionList, { type MentionItem } from '@/components/analysis/MentionList.vue'

const props = withDefaults(
  defineProps<{
    modelValue: unknown
    // entities is a list filtered here, search asks the server instead.
    entities?: MentionItem[]
    search?: (q: string) => Promise<MentionItem[]>
    editable?: boolean
    placeholder?: string
  }>(),
  {
    entities: () => [],
    search: undefined,
    editable: true,
    placeholder: 'Write the analysis. Type @ to reference an entity.',
  },
)

const emit = defineEmits<{
  (e: 'update:modelValue', doc: unknown): void
  (e: 'mention', nodeId: string, sketchId: string): void
}>()

// EntityMention keeps the sketch and the type on the chip, so a click
// knows where to go and the chip can show its colour.
const EntityMention = Mention.extend({
  addAttributes() {
    return {
      ...this.parent?.(),
      sketch: {
        default: null,
        parseHTML: (el: HTMLElement) => el.getAttribute('data-sketch'),
        renderHTML: (attrs: Record<string, unknown>) => ({ 'data-sketch': attrs.sketch }),
      },
      type: {
        default: null,
        parseHTML: (el: HTMLElement) => el.getAttribute('data-type'),
        renderHTML: (attrs: Record<string, unknown>) => ({ 'data-type': attrs.type }),
      },
    }
  },
})

function isDoc(v: unknown): v is object {
  return !!v && typeof v === 'object' && (v as { type?: string }).type === 'doc'
}

// The server search waits for typing to stop: a call overtaken by a
// newer one within the wait asks nothing and hands back the last list,
// and so does an answer that arrives after a newer call began. The
// newest call then replaces it. Every call resolves.
let searchSeq = 0
let lastFound: MentionItem[] = []

function searched(q: string): Promise<MentionItem[]> {
  const seq = ++searchSeq

  return new Promise((resolve) => {
    setTimeout(async () => {
      if (seq !== searchSeq || !props.search) return resolve(lastFound)
      try {
        const found = await props.search(q)
        if (seq === searchSeq) lastFound = found.slice(0, 10)
      } catch {
        if (seq === searchSeq) lastFound = []
      }
      resolve(lastFound)
    }, 150)
  })
}

function filtered(query: string): MentionItem[] {
  const q = query.toLowerCase()
  return props.entities
    .filter((e) => !q || e.label.toLowerCase().includes(q) || e.type.includes(q))
    .slice(0, 10)
}

function suggestion() {
  let component: VueRenderer | null = null
  let popup: HTMLElement | null = null

  const place = (rect: (() => DOMRect | null) | null | undefined) => {
    const r = rect?.()
    if (!r || !popup) return
    popup.style.left = Math.min(r.left, window.innerWidth - 380) + 'px'
    popup.style.top = Math.min(r.bottom + 4, window.innerHeight - 260) + 'px'
  }

  return {
    char: '@',
    items: ({ query }: { query: string }): MentionItem[] | Promise<MentionItem[]> =>
      props.search ? searched(query) : filtered(query),
    render: () => ({
      onStart: (p: SuggestionProps<MentionItem>) => {
        component = new VueRenderer(MentionList, { props: p, editor: p.editor })
        popup = document.createElement('div')
        popup.className = 'mention-popup'
        popup.appendChild(component.element as HTMLElement)
        document.body.appendChild(popup)
        place(p.clientRect)
      },
      onUpdate: (p: SuggestionProps<MentionItem>) => {
        component?.updateProps(p)
        place(p.clientRect)
      },
      onKeyDown: (p: { event: KeyboardEvent }) => {
        if (p.event.key === 'Escape') {
          popup?.remove()
          popup = null
          return true
        }
        return (
          (
            component?.ref as { onKeyDown?: (p: { event: KeyboardEvent }) => boolean } | null
          )?.onKeyDown?.(p) ?? false
        )
      },
      onExit: () => {
        popup?.remove()
        popup = null
        component?.destroy()
        component = null
      },
    }),
  }
}

const editor = useEditor({
  // An empty object is what a new analysis holds, which is no document.
  content: isDoc(props.modelValue) ? props.modelValue : undefined,
  editable: props.editable,
  extensions: [
    StarterKit,
    Placeholder.configure({ placeholder: props.placeholder }),
    EntityMention.configure({
      HTMLAttributes: { class: 'mention' },
      renderText: ({ node }) => `@${node.attrs.label ?? node.attrs.id}`,
      deleteTriggerWithBackspace: true,
      suggestion: suggestion(),
    }),
  ],
  editorProps: {
    handleClick: (_view, _pos, event) => {
      const el = (event.target as HTMLElement).closest('.mention') as HTMLElement | null
      if (el?.dataset.id) {
        emit('mention', el.dataset.id, el.dataset.sketch ?? '')
        return true
      }
      return false
    },
  },
  onUpdate: ({ editor }) => emit('update:modelValue', editor.getJSON()),
})

watch(
  () => props.editable,
  (v) => editor.value?.setEditable(v),
)

function run(action: string) {
  const e = editor.value
  if (!e) return
  const c = e.chain().focus()
  switch (action) {
    case 'bold':
      c.toggleBold().run()
      break
    case 'italic':
      c.toggleItalic().run()
      break
    case 'strike':
      c.toggleStrike().run()
      break
    case 'code':
      c.toggleCode().run()
      break
    case 'h1':
      c.toggleHeading({ level: 1 }).run()
      break
    case 'h2':
      c.toggleHeading({ level: 2 }).run()
      break
    case 'h3':
      c.toggleHeading({ level: 3 }).run()
      break
    case 'bullets':
      c.toggleBulletList().run()
      break
    case 'numbers':
      c.toggleOrderedList().run()
      break
    case 'quote':
      c.toggleBlockquote().run()
      break
    case 'codeblock':
      c.toggleCodeBlock().run()
      break
    case 'hr':
      c.setHorizontalRule().run()
      break
    case 'link': {
      const prev = e.getAttributes('link').href as string | undefined
      const url = window.prompt('Link address', prev ?? 'https://')
      if (url === null) return
      if (!url.trim()) c.unsetLink().run()
      else c.extendMarkRange('link').setLink({ href: url.trim() }).run()
      break
    }
    case 'undo':
      c.undo().run()
      break
    case 'redo':
      c.redo().run()
      break
    case 'mention':
      c.insertContent('@').run()
      break
  }
}

const tools: [string, string, string][] = [
  ['bold', 'B', 'Bold'],
  ['italic', 'I', 'Italic'],
  ['strike', 'S', 'Strikethrough'],
  ['code', '<>', 'Code'],
  ['h1', 'H1', 'Heading 1'],
  ['h2', 'H2', 'Heading 2'],
  ['h3', 'H3', 'Heading 3'],
  ['bullets', '•', 'Bullet list'],
  ['numbers', '1.', 'Numbered list'],
  ['quote', '“', 'Quote'],
  ['codeblock', '{ }', 'Code block'],
  ['hr', '—', 'Divider'],
  ['link', '\u{1F517}', 'Link'],
  ['mention', '@', 'Reference an entity'],
]

function active(action: string): boolean {
  const e = editor.value
  if (!e) return false
  switch (action) {
    case 'h1':
    case 'h2':
    case 'h3':
      return e.isActive('heading', { level: Number(action[1]) })
    case 'bullets':
      return e.isActive('bulletList')
    case 'numbers':
      return e.isActive('orderedList')
    case 'quote':
      return e.isActive('blockquote')
    case 'codeblock':
      return e.isActive('codeBlock')
    default:
      return e.isActive(action)
  }
}

onBeforeUnmount(() => {
  searchSeq++
  editor.value?.destroy()
})
</script>

<template>
  <div class="analysis-editor">
    <div v-if="editable" class="toolbar">
      <button
        v-for="[action, glyph, title] in tools"
        :key="action"
        class="tool"
        :class="{ [action]: true, on: active(action) }"
        :title="title"
        type="button"
        @mousedown.prevent="run(action)"
      >
        {{ glyph }}
      </button>
      <span class="sep"></span>
      <button class="tool" title="Undo" type="button" @mousedown.prevent="run('undo')">
        &#8630;
      </button>
      <button class="tool" title="Redo" type="button" @mousedown.prevent="run('redo')">
        &#8631;
      </button>
    </div>
    <EditorContent :editor="editor" class="content" />
  </div>
</template>

<style scoped>
.analysis-editor {
  display: flex;
  flex-direction: column;
  min-height: 0;
  height: 100%;
}

.toolbar {
  display: flex;
  flex-wrap: wrap;
  gap: 2px;
  padding: 6px 8px;
  border-bottom: 1px solid var(--border-primary);
}

.tool {
  min-width: 28px;
  height: 26px;
  padding: 0 6px;
  border: 0;
  border-radius: var(--radius-sm);
  background: none;
  color: var(--text-secondary);
  font-size: 12px;
  cursor: pointer;
}

.tool.bold {
  font-weight: 700;
}

.tool.italic {
  font-style: italic;
}

.tool.strike {
  text-decoration: line-through;
}

.tool:hover {
  background: var(--bg-hover);
}

.tool.on {
  background: var(--bg-active);
  color: var(--text-primary);
}

.sep {
  width: 1px;
  margin: 4px 4px;
  background: var(--border-primary);
}

.content {
  flex: 1;
  min-height: 0;
  overflow-y: auto;
}

/* The editor's own outline is off, the box around it shows the focus
   like an input does. Inset, since the card holding it clips. */
.analysis-editor:focus-within {
  box-shadow: inset 0 0 0 3px var(--ring);
}

.content :deep(.tiptap) {
  min-height: 200px;
  padding: 14px 16px;
  outline: 2px solid transparent;
  font-size: 14px;
  line-height: 1.6;
}

.content :deep(.tiptap p.is-editor-empty:first-child::before) {
  content: attr(data-placeholder);
  float: left;
  height: 0;
  color: var(--text-placeholder);
  pointer-events: none;
}

.content :deep(.tiptap h1) {
  font-size: 22px;
  margin: 12px 0 6px;
}

.content :deep(.tiptap h2) {
  font-size: 18px;
  margin: 12px 0 6px;
}

.content :deep(.tiptap h3) {
  font-size: 15px;
  margin: 10px 0 4px;
}

.content :deep(.tiptap p) {
  margin: 0 0 8px;
}

.content :deep(.tiptap blockquote) {
  margin: 8px 0;
  padding-left: 12px;
  border-left: 3px solid var(--border-strong);
  color: var(--text-secondary);
}

.content :deep(.tiptap pre) {
  padding: 10px 12px;
  border-radius: var(--radius-sm);
  background: var(--bg-code);
  color: var(--text-code);
  font-size: 12px;
  overflow-x: auto;
}

.content :deep(.tiptap code) {
  padding: 0 4px;
  border-radius: var(--radius-sm);
  background: var(--bg-code);
  color: var(--text-code);
  font-size: 12px;
}

.content :deep(.tiptap pre code) {
  padding: 0;
  background: none;
}

.content :deep(.tiptap ul),
.content :deep(.tiptap ol) {
  padding-left: 22px;
  margin: 0 0 8px;
}

.content :deep(.tiptap hr) {
  border: 0;
  border-top: 1px solid var(--border-primary);
  margin: 12px 0;
}

.content :deep(.tiptap a) {
  color: var(--text-primary);
  text-decoration: underline;
}

.content :deep(.mention) {
  display: inline-block;
  padding: 0 6px;
  border-radius: 999px;
  background: var(--bg-tertiary);
  border: 1px solid var(--border-primary);
  color: var(--text-primary);
  font-size: 12px;
  cursor: pointer;
  text-decoration: none;
}

.content :deep(.mention:hover) {
  background: var(--bg-hover);
}
</style>

<style>
/* The suggestion popup is appended to body, outside this component. */
.mention-popup {
  position: fixed;
  z-index: var(--z-float);
}
</style>
