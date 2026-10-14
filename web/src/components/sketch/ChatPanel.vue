<script setup lang="ts">
// The assistant: a conversation about the sketch, with the selected
// entities handed to the model as context. Several chats per sketch,
// each the account's own, the first question naming a new one.
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import axios from 'axios'
import {
  createChat,
  deleteChat,
  listChats,
  listMessages,
  streamChat,
  splitProposal,
} from '@/api/chat'
import { apiErrorMessage } from '@/api/client'
import ProposalCards from '@/components/sketch/ProposalCards.vue'
import type { Chat, ChatMessage, GraphNode } from '@/api/types'
import { useLlmModels } from '@/composables/llmModels'
import { useConfirm } from '@/composables/useConfirm'
import { useNotificationStore } from '@/stores/notification'

const props = defineProps<{
  sketchId: string
  selected: GraphNode[]
  nodes: GraphNode[]
  canEdit: boolean
}>()
const emit = defineEmits<{ (e: 'applied'): void }>()

const toast = useNotificationStore()
const { confirm } = useConfirm()
const chats = ref<Chat[]>([])
const current = ref<string>('')
// placeholders numbers the client-side ids a question and a cut-off
// answer carry until the server's replace them (a UUID would need a
// secure context, which a console over plain http is not).
let placeholders = 0
const messages = ref<ChatMessage[]>([])
const prompt = ref('')
const useContext = ref(true)
const withChildren = ref(false)
const busy = ref(false)
const partial = ref('')

// What the model is doing while nothing of the answer has arrived: its
// reasoning as it streams, and how long since the last sign of life.
const thinking = ref('')
const drafting = ref(0)
const actions = ref<string[]>([])
const lastSign = ref(0)
const now = ref(Date.now())
let ticker: ReturnType<typeof setInterval> | null = null

const waited = computed(() => Math.max(0, Math.round((now.value - lastSign.value) / 1000)))
const status = computed(() => {
  if (!busy.value) return ''
  if (actions.value.length && waited.value < 30) return actions.value[actions.value.length - 1]
  if (drafting.value) return `Preparing changes, ${drafting.value} characters so far`
  if (waited.value >= 20) return `Waiting for the model, ${waited.value}s without a sign`
  if (partial.value) return ''
  if (thinking.value) return `Thinking, ${thinking.value.length} characters so far`
  return waited.value < 3
    ? 'Asking the model'
    : `Waiting for the model, ${waited.value}s without a sign`
})

function sign() {
  lastSign.value = Date.now()
  now.value = lastSign.value
}
const problem = ref('')
const { models, current: model, loaded: modelsLoaded, load: loadModels, pick } = useLlmModels()
const log = ref<HTMLElement | null>(null)
let abort: AbortController | null = null

const chat = computed(() => chats.value.find((c) => c.id === current.value) ?? null)

// The chats about the selected entities come first, since a chat is
// found again by what it was about, not by its first line.
const selectedIds = computed(() => new Set(props.selected.map((n) => n.id)))
const about = computed(() =>
  props.selected.length
    ? chats.value.filter((c) => c.entities?.some((id) => selectedIds.value.has(id)))
    : [],
)
const others = computed(() => chats.value.filter((c) => !about.value.includes(c)))

// A new selection opens its newest chat, unless a question is in
// flight. With none about it, a fresh chat.
watch(
  () => [...selectedIds.value].sort().join(','),
  () => {
    if (busy.value || !props.selected.length) return
    if (about.value.some((c) => c.id === current.value)) return
    current.value = about.value[0]?.id ?? ''
    if (!current.value) messages.value = []
  },
)
const context = computed(() => (useContext.value ? props.selected : []))

async function loadChats() {
  chats.value = await listChats(props.sketchId)
  if (chats.value.some((c) => c.id === current.value)) return
  // Nothing picked yet: the newest chat about the selection, else the
  // newest chat, else a fresh one.
  current.value = (about.value[0] ?? chats.value[0])?.id ?? ''
}

// loadMessages reads the chat's turns. A load that comes back after the
// chat changed, or while a question is in flight, must not throw away
// what the panel holds: send() adds the question and the answer itself.
async function loadMessages() {
  const id = current.value
  const list = id ? await listMessages(props.sketchId, id) : []
  if (current.value !== id) return
  if (busy.value) {
    const extra = messages.value.filter((m) => !list.some((x) => x.id === m.id))
    messages.value = [...list, ...extra]
  } else {
    messages.value = list
  }
  await scroll()
}

async function scroll() {
  await nextTick()
  if (log.value) log.value.scrollTop = log.value.scrollHeight
}

// stick follows a streaming answer one frame at a time, and only while
// the reader is near the end, so a scroll up to read is not undone.
let frame = 0
function stick() {
  if (frame) return
  const el = log.value
  const near = !el || el.scrollHeight - el.scrollTop - el.clientHeight < 40
  frame = requestAnimationFrame(() => {
    frame = 0
    if (near && log.value) log.value.scrollTop = log.value.scrollHeight
  })
}

function fresh() {
  current.value = ''
  messages.value = []
  problem.value = ''
}

async function remove() {
  if (!chat.value) return
  if (
    !(await confirm({
      title: 'Delete chat',
      message: `Delete the chat "${chat.value.title || 'untitled'}"?`,
      variant: 'danger',
    }))
  )
    return
  try {
    await deleteChat(props.sketchId, chat.value.id)
    current.value = ''
    await loadChats()
  } catch (err) {
    toast.error(apiErrorMessage(err))
  }
}

async function send() {
  const text = prompt.value.trim()
  if (!text || busy.value) return
  busy.value = true
  problem.value = ''
  partial.value = ''
  try {
    if (!current.value) {
      const c = await createChat(props.sketchId)
      chats.value.unshift(c)
      current.value = c.id
    }

    const ids = context.value.map((n) => n.id)
    // The question gets an id of its own until the server's replaces
    // it after the answer, so two questions never share a key.
    messages.value.push({
      id: 'pending-' + ++placeholders,
      chat_id: current.value,
      role: 'user',
      content: text,
      context: ids,
      created_at: new Date().toISOString(),
    })
    prompt.value = ''
    thinking.value = ''
    drafting.value = 0
    actions.value = []
    sign()
    ticker = setInterval(() => (now.value = Date.now()), 1000)
    await scroll()
    abort = new AbortController()
    await streamChat(
      props.sketchId,
      current.value,
      text,
      ids,
      {
        token: (t) => {
          partial.value += t
          sign()
          stick()
        },
        thinking: (t) => {
          thinking.value += t
          sign()
        },
        drafting: (t) => {
          drafting.value += t.length
          sign()
        },
        action: (t) => {
          actions.value.push(t)
          drafting.value = 0
          sign()
        },
        progress: (t) => {
          actions.value.splice(actions.value.length - 1, 1, t)
          sign()
        },
        done: (m) => {
          messages.value.push(m)
          partial.value = ''
          thinking.value = ''
        },
      },
      abort.signal,
      useContext.value && withChildren.value,
      model.value?.id ?? '',
    )
    await loadChats()
    // The server's ids replace the pending ones. Not busy any more, or
    // the load would keep the pending question beside the stored one.
    busy.value = false
    await loadMessages()
  } catch (err) {
    if (!(err instanceof DOMException && err.name === 'AbortError'))
      problem.value =
        err instanceof Error && !axios.isAxiosError(err) ? err.message : apiErrorMessage(err)
    if (partial.value) {
      // Keep what arrived, the server stored the same.
      messages.value.push({
        id: 'partial-' + ++placeholders,
        chat_id: current.value,
        role: 'assistant',
        content: partial.value,
        context: [],
        created_at: new Date().toISOString(),
      })
      partial.value = ''
    }
  } finally {
    busy.value = false
    abort = null
    thinking.value = ''
    drafting.value = 0
    actions.value = []
    if (ticker) clearInterval(ticker)
    ticker = null
    await scroll()
  }
}

function stop() {
  abort?.abort()
}

function onKey(ev: KeyboardEvent) {
  if (ev.key === 'Enter' && !ev.shiftKey) {
    ev.preventDefault()
    void send()
  }
}

// turns is each message with its proposal block split out once: the
// text draws as is and the changes as cards.
const turns = computed(() =>
  messages.value.map((m) => {
    const { text, changes } = splitProposal(m.content)
    return { m, text, changes }
  }),
)

// The answer under way, its block held back while it streams.
const partialText = computed(() => splitProposal(partial.value).text)

// render keeps the text as written, with `code` spans marked, which is
// how the model is asked to name an entity.
function render(text: string): string {
  const safe = text.replace(/[&<>]/g, (c) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;' })[c] ?? c)
  return safe.replace(/`([^`\n]+)`/g, '<code>$1</code>')
}

watch(current, () => loadMessages().catch((err) => toast.error(apiErrorMessage(err))))

onMounted(async () => {
  // The models are their own call: without them the chat still lists.
  try {
    await loadModels()
  } catch (err) {
    toast.error(apiErrorMessage(err))
  }

  try {
    await loadChats()
    await loadMessages()
  } catch (err) {
    toast.error(apiErrorMessage(err))
  }
})

onBeforeUnmount(() => {
  stop()
  cancelAnimationFrame(frame)
})
</script>

<template>
  <div class="chat">
    <div class="top">
      <select v-model="current" class="form-select pick" title="Earlier chats" aria-label="Chat">
        <option value="">New chat</option>
        <template v-if="about.length">
          <optgroup label="About the selected entities">
            <option v-for="c in about" :key="c.id" :value="c.id">
              {{ c.title || 'untitled' }}
            </option>
          </optgroup>
          <optgroup v-if="others.length" label="Other chats">
            <option v-for="c in others" :key="c.id" :value="c.id">
              {{ c.title || 'untitled' }}
            </option>
          </optgroup>
        </template>
        <option v-for="c in others" v-else :key="c.id" :value="c.id">
          {{ c.title || 'untitled' }}
        </option>
      </select>
      <button v-if="current" class="btn btn-sm btn-secondary" title="Start another" @click="fresh">
        New
      </button>
      <button
        v-if="current"
        class="btn btn-sm btn-secondary"
        title="Delete this chat"
        aria-label="Delete this chat"
        @click="remove"
      >
        <span aria-hidden="true">&times;</span>
      </button>
    </div>

    <div ref="log" class="log">
      <p v-if="!messages.length && !partial" class="text-sm text-muted hint">
        Ask about the sketch. Select entities first and they are handed to the model as context.
        <template v-if="canEdit">
          The assistant can run transforms and flows on them for you.
        </template>
        <template v-if="model">
          Answers come from <strong>{{ model.name }}</strong
          ><template v-if="model.needs_key">
            with your <span class="code-font">{{ model.key_name }}</span> from Secrets</template
          >.
        </template>
        <template v-else-if="modelsLoaded">
          No language model is set up yet, an administrator adds one under Administration, Models.
        </template>
      </p>
      <div v-for="{ m, text, changes } in turns" :key="m.id" class="msg" :class="m.role">
        <div class="who text-xs text-muted">
          {{ m.role === 'user' ? 'You' : 'Assistant' }}
          <span v-if="m.context?.length"> , {{ m.context.length }} entities as context</span>
        </div>
        <div v-if="text" class="text" v-html="render(text)"></div>
        <ProposalCards
          v-if="m.role === 'assistant' && changes.length"
          :sketch-id="sketchId"
          :chat-id="m.chat_id"
          :message-id="m.id"
          :changes="changes"
          :nodes="nodes"
          :can-edit="canEdit"
          @applied="emit('applied')"
        />
      </div>
      <div v-if="partial || busy" class="msg assistant">
        <div class="who text-xs text-muted">Assistant</div>
        <div v-if="status" class="status text-sm text-muted">
          <span class="pulse"></span>{{ status }}
        </div>
        <ul v-if="actions.length" class="lookups text-xs text-muted">
          <li v-for="(a, i) in actions" :key="i">{{ a }}</li>
        </ul>
        <details v-if="thinking" class="reasoning">
          <summary class="text-xs text-muted">Show the reasoning</summary>
          <pre class="text-xs text-muted">{{ thinking.slice(-2000) }}</pre>
        </details>
        <div v-if="partial" class="text" v-html="render(partialText)"></div>
      </div>
      <p v-if="problem" class="notice notice-danger text-sm">
        {{ problem }}
        <router-link v-if="/secret|vault/i.test(problem)" to="/secrets">Open Secrets</router-link>
      </p>
    </div>

    <div class="compose">
      <label v-if="selected.length" class="checkbox-label text-sm">
        <input v-model="useContext" type="checkbox" />
        Use the {{ selected.length }} selected {{ selected.length === 1 ? 'entity' : 'entities' }}
        as context
      </label>
      <label v-if="selected.length && useContext" class="checkbox-label text-sm">
        <input v-model="withChildren" type="checkbox" />
        Also what was found through them
      </label>
      <textarea
        v-model="prompt"
        class="form-textarea"
        rows="3"
        placeholder="Ask something, Enter sends, Shift+Enter breaks the line"
        aria-label="Question"
        :disabled="busy"
        @keydown="onKey"
      />
      <div class="flex gap-2 justify-end">
        <select
          v-if="models.length > 1"
          class="form-select model"
          title="The model that answers"
          aria-label="The model that answers"
          :value="model?.id"
          :disabled="busy"
          @change="pick(($event.target as HTMLSelectElement).value)"
        >
          <option v-for="m in models" :key="m.id" :value="m.id">{{ m.name }}</option>
        </select>
        <button v-if="busy" class="btn btn-sm btn-secondary" @click="stop">Stop</button>
        <button class="btn btn-sm btn-primary" :disabled="busy || !prompt.trim()" @click="send">
          Send
        </button>
      </div>
    </div>
  </div>
</template>

<style scoped>
.chat {
  display: flex;
  flex-direction: column;
  height: 100%;
}

.top {
  display: flex;
  gap: 6px;
  padding: 12px 16px 8px;
}

.pick {
  flex: 1;
  min-width: 0;
}

.model {
  width: auto;
  max-width: 60%;
  height: var(--control-h-sm);
  margin-right: auto;
  font-size: 13px;
}

.log {
  flex: 1;
  min-height: 0;
  overflow-y: auto;
  padding: 8px 16px;
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.hint {
  margin: auto 0;
}

.status {
  display: flex;
  align-items: center;
  gap: 8px;
}

.pulse {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  background: var(--text-muted);
  animation: pulse 1.2s ease-in-out infinite;
}

@keyframes pulse {
  0%,
  100% {
    opacity: 0.3;
  }
  50% {
    opacity: 1;
  }
}

.lookups {
  margin: 4px 0;
  padding-left: 16px;
}

.reasoning {
  margin: 4px 0 8px;
}

.reasoning pre {
  max-height: 160px;
  overflow: auto;
  margin: 4px 0 0;
  padding: 6px 8px;
  border-left: 2px solid var(--border-primary);
  white-space: pre-wrap;
  font-family: inherit;
}

.msg .text {
  font-size: 13px;
  line-height: 1.5;
  white-space: pre-wrap;
  overflow-wrap: anywhere;
}

.msg.user .text {
  padding: 8px 10px;
  border-radius: var(--radius-md);
  background: var(--bg-tertiary);
}

.msg .text :deep(code) {
  padding: 0 4px;
  border-radius: var(--radius-sm);
  background: var(--bg-code);
  color: var(--text-code);
  font-size: 12px;
}

.compose {
  display: flex;
  flex-direction: column;
  gap: 8px;
  padding: 8px 16px 16px;
  border-top: 1px solid var(--border-primary);
}
</style>
