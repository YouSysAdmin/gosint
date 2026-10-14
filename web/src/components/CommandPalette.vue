<script setup lang="ts">
// Cmd/Ctrl+J: type to jump to an investigation, a sketch, a page, or an
// entity of any sketch you may read, found on the server as you type.
// Mounted once in the layout.
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import { searchEntities, type SearchHit } from '@/api/catalog'
import { listInvestigations } from '@/api/investigations'
import { listSketches } from '@/api/sketches'
import type { Investigation, Sketch } from '@/api/types'
import { isModalOpen } from '@/components/BaseModal.vue'
import { useAuthStore } from '@/stores/auth'

interface Item {
  id: string
  group: string
  title: string
  hint: string
  to: string
  // The lowercased title and hint and their words, made once per item
  // so a keystroke scores without splitting the list again.
  lower: string
  words: string[]
  hintLower: string
  hintWords: string[]
}

type ItemFields = Pick<Item, 'id' | 'group' | 'title' | 'hint' | 'to'>

function item(f: ItemFields): Item {
  const lower = f.title.toLowerCase()
  const hintLower = f.hint.toLowerCase()

  return { ...f, lower, words: lower.split(/\s+/), hintLower, hintWords: hintLower.split(/\s+/) }
}

// The investigations and sketches are kept across openings and asked
// again only once a minute old, the kept lists shown meanwhile.
const LISTS_TTL = 60_000
let lists: { investigations: Investigation[]; sketches: Sketch[]; at: number } | null = null
let listing: Promise<void> | null = null

const router = useRouter()
const auth = useAuthStore()
const open = ref(false)
const query = ref('')
const cursor = ref(0)
const input = ref<HTMLInputElement | null>(null)
const investigations = ref<Investigation[]>([])
const sketches = ref<Sketch[]>([])

// Entities matching the query, asked of the server a moment after the
// typing stops, from two characters on. A late answer to an older
// query is dropped, and the older query's hits go at once, or Enter
// could jump to one of them.
const hits = ref<SearchHit[]>([])
let timer = 0
let asked = 0
watch(query, (q) => {
  window.clearTimeout(timer)
  hits.value = []
  const text = q.trim()
  if (text.length < 2) return
  const n = ++asked
  timer = window.setTimeout(async () => {
    try {
      const found = await searchEntities({ q: text })
      if (n === asked) hits.value = found
    } catch {
      // the local items still work
    }
  }, 150)
})

const allPages: Item[] = [
  { id: 'p-inv', group: 'Pages', title: 'Investigations', hint: '', to: '/investigations' },
  { id: 'p-flows', group: 'Pages', title: 'Flows', hint: '', to: '/flows' },
  { id: 'p-newflow', group: 'Pages', title: 'New flow', hint: '', to: '/flows/new' },
  { id: 'p-enr', group: 'Pages', title: 'Transforms', hint: '', to: '/transforms' },
  { id: 'p-tpl', group: 'Pages', title: 'New transform template', hint: '', to: '/templates/new' },
  {
    id: 'p-secrets',
    group: 'Pages',
    title: 'Secrets',
    hint: 'API keys and tokens',
    to: '/secrets',
  },
  { id: 'p-admin', group: 'Pages', title: 'Accounts', hint: 'administration', to: '/admin/users' },
  {
    id: 'p-profile',
    group: 'Pages',
    title: 'Profile',
    hint: 'your name and avatar',
    to: '/profile',
  },
].map(item)

// The administration page is offered to administrators only, the
// router sends anyone else back without a word.
const pages = computed(() => allPages.filter((p) => !p.to.startsWith('/admin') || auth.user?.admin))

const items = computed<Item[]>(() => {
  const invName = new Map(investigations.value.map((i) => [i.id, i.name]))
  return [
    ...investigations.value.map((i) =>
      item({
        id: 'i-' + i.id,
        group: 'Investigations',
        title: i.name,
        hint: i.status === 'archived' ? 'archived' : '',
        to: `/investigations/${i.id}`,
      }),
    ),
    ...sketches.value.map((s) =>
      item({
        id: 's-' + s.id,
        group: 'Sketches',
        title: s.title,
        hint: invName.get(s.investigation_id) ?? '',
        to: `/sketches/${s.id}`,
      }),
    ),
    ...pages.value,
    ...hits.value.map((h) =>
      item({
        id: 'e-' + h.id,
        group: 'Entities',
        title: h.label,
        hint: `${h.type} in ${h.sketch_title}, ${h.investigation_name}`,
        to: `/sketches/${h.sketch_id}?focus=${h.id}`,
      }),
    ),
  ]
})

// score ranks a match: a prefix first, then a word start, then anywhere.
function score(t: string, words: string[], q: string): number {
  if (t.startsWith(q)) return 3
  if (words.some((w) => w.startsWith(q))) return 2
  if (t.includes(q)) return 1
  return 0
}

const shown = computed(() => {
  const q = query.value.trim().toLowerCase()
  if (!q) return items.value.slice(0, 12)
  return items.value
    .map((it) => ({
      it,
      s: Math.max(score(it.lower, it.words, q), score(it.hintLower, it.hintWords, q) - 1),
    }))
    .filter((x) => x.s > 0 || x.it.group === 'Entities')
    .sort((a, b) => b.s - a.s || a.it.title.localeCompare(b.it.title))
    .slice(0, 20)
    .map((x) => x.it)
})

watch(shown, () => (cursor.value = 0))

// loadLists asks the server for both lists, one request pair at a time.
function loadLists(): Promise<void> {
  if (listing) return listing
  listing = (async () => {
    try {
      const [inv, sk] = await Promise.all([listInvestigations(), listSketches()])
      lists = { investigations: inv, sketches: sk, at: Date.now() }
      investigations.value = inv
      sketches.value = sk
    } catch {
      // the pages still work
    } finally {
      listing = null
    }
  })()

  return listing
}

async function show() {
  open.value = true
  query.value = ''
  await nextTick()
  input.value?.focus()
  if (lists) {
    investigations.value = lists.investigations
    sketches.value = lists.sketches
  }
  if (!lists || Date.now() - lists.at > LISTS_TTL) void loadLists()
}

function go(it: Item) {
  open.value = false
  void router.push(it.to)
}

function onKey(ev: KeyboardEvent) {
  if ((ev.metaKey || ev.ctrlKey) && (ev.key === 'j' || ev.key === 'J')) {
    ev.preventDefault()
    if (open.value) open.value = false
    // Under a dialog the palette would open behind it.
    else if (!isModalOpen()) void show()
    return
  }
  if (!open.value) return
  if (ev.key === 'Escape') {
    // The page's own Escape (a cleared selection) is not meant.
    ev.stopPropagation()
    open.value = false
  } else if (ev.key === 'ArrowDown') {
    ev.preventDefault()
    cursor.value = Math.min(cursor.value + 1, shown.value.length - 1)
  } else if (ev.key === 'ArrowUp') {
    ev.preventDefault()
    cursor.value = Math.max(cursor.value - 1, 0)
  } else if (ev.key === 'Enter' && shown.value[cursor.value]) {
    ev.preventDefault()
    go(shown.value[cursor.value])
  }
}

onMounted(() => window.addEventListener('keydown', onKey))
onBeforeUnmount(() => window.removeEventListener('keydown', onKey))

defineExpose({ show })
</script>

<template>
  <Teleport to="body">
    <div v-if="open" class="palette-overlay" @mousedown.self="open = false">
      <div class="palette">
        <input
          ref="input"
          v-model="query"
          class="form-input"
          placeholder="Jump to an investigation, a sketch, a page or an entity"
        />
        <ul class="results">
          <li
            v-for="(it, i) in shown"
            :key="it.id"
            :class="{ on: i === cursor }"
            @mouseenter="cursor = i"
            @click="go(it)"
          >
            <span class="group text-xs text-muted">{{ it.group }}</span>
            <span class="title">{{ it.title }}</span>
            <span class="hint text-xs text-muted">{{ it.hint }}</span>
          </li>
          <li v-if="!shown.length" class="text-muted text-sm none">Nothing matches.</li>
        </ul>
      </div>
    </div>
  </Teleport>
</template>

<style scoped>
.palette-overlay {
  position: fixed;
  inset: 0;
  /* Above a dialog, under the toasts. */
  z-index: var(--z-palette);
  background: var(--overlay);
  display: flex;
  justify-content: center;
  padding-top: 12vh;
}

.palette {
  width: 560px;
  max-width: calc(100vw - 32px);
  height: fit-content;
  padding: 8px;
  background: var(--bg-popover);
  border: 1px solid var(--border-primary);
  border-radius: var(--radius-md);
  box-shadow: var(--shadow-lg);
}

.results {
  list-style: none;
  margin: 6px 0 0;
  padding: 0;
  max-height: 50vh;
  overflow-y: auto;
}

.results li {
  display: grid;
  grid-template-columns: 100px 1fr auto;
  align-items: center;
  gap: 10px;
  padding: 7px 10px;
  border-radius: var(--radius-sm);
  font-size: 13px;
  cursor: pointer;
}

.results li.on {
  background: var(--bg-hover);
}

.results li.none {
  display: block;
  cursor: default;
}

.title {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
</style>
