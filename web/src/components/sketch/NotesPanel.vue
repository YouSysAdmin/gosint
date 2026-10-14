<script setup lang="ts">
// The investigation's analyses beside the graph: pick one or start one,
// write with @ for the sketch's entities, a reference clicked shows the
// entity on the canvas.
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import { createAnalysis, getAnalysis, listAnalyses, saveAnalysis } from '@/api/analyses'
import { apiErrorMessage } from '@/api/client'
import type { Analysis, GraphNode } from '@/api/types'
import AnalysisEditor from '@/components/analysis/AnalysisEditor.vue'
import type { MentionItem } from '@/components/analysis/MentionList.vue'
import { useNotificationStore } from '@/stores/notification'

const props = defineProps<{
  investigationId: string
  sketchId: string
  sketchTitle: string
  nodes: GraphNode[]
  canEdit: boolean
}>()
const emit = defineEmits<{ (e: 'focus', id: string): void }>()

const router = useRouter()
const toast = useNotificationStore()
const list = ref<Analysis[]>([])
const current = ref('')
const analysis = ref<Analysis | null>(null)
const doc = ref<unknown>({})
const state = ref<'saved' | 'dirty' | 'saving'>('saved')
let timer: ReturnType<typeof setTimeout> | null = null

// Every edit bumps rev, a save records what it sent: an edit typed
// while a save is out is saved by the next round, never overwritten
// by the first one finishing. One save is out at a time (inflight).
let rev = 0
let saved = 0
let inflight: Promise<void> | null = null

const entities = computed<MentionItem[]>(() =>
  props.nodes.map((n) => ({ id: n.id, label: n.label, type: n.type, sketch: props.sketchId })),
)

async function loadList() {
  list.value = await listAnalyses(props.investigationId)
  if (!current.value && list.value[0]) current.value = list.value[0].id
}

// open loads the picked analysis once the current one is saved. A load
// overtaken by another pick changes nothing.
async function open(id: string) {
  await flush()
  if (current.value !== id) return
  let next: Analysis | null = null
  if (id) {
    try {
      next = await getAnalysis(props.investigationId, id)
    } catch (err) {
      toast.error(apiErrorMessage(err))
    }
    if (current.value !== id) return
  }

  analysis.value = next
  doc.value = next?.content ?? {}
  saved = rev
  state.value = 'saved'
}

async function fresh() {
  if (!props.canEdit) return
  try {
    const a = await createAnalysis(props.investigationId, `Notes on ${props.sketchTitle}`)
    list.value.unshift(a)
    current.value = a.id
  } catch (err) {
    toast.error(apiErrorMessage(err))
  }
}

function changed(next: unknown) {
  doc.value = next
  rev++
  state.value = 'dirty'
  if (timer) clearTimeout(timer)
  timer = setTimeout(flush, 1000)
}

// flush saves until nothing is left unsaved, joining a save already
// out instead of starting a second one.
function flush(): Promise<void> {
  if (timer) clearTimeout(timer)
  timer = null
  if (inflight) return inflight
  inflight = (async () => {
    while (analysis.value && rev !== saved) {
      if (!(await save())) break
    }
  })().finally(() => (inflight = null))

  return inflight
}

// save sends the document as it is now and reports whether it landed.
// An edit made meanwhile leaves the state dirty for the next round.
async function save(): Promise<boolean> {
  const a = analysis.value
  if (!a) return false
  const sent = rev
  state.value = 'saving'
  try {
    const next = await saveAnalysis(props.investigationId, a.id, { content: doc.value })
    if (analysis.value?.id === a.id) analysis.value = next
    saved = sent
    state.value = rev === sent ? 'saved' : 'dirty'

    return true
  } catch (err) {
    state.value = 'dirty'
    toast.error(apiErrorMessage(err))

    return false
  }
}

function mention(nodeId: string, sketchId: string) {
  if (!sketchId || sketchId === props.sketchId) emit('focus', nodeId)
  else void router.push({ path: `/sketches/${sketchId}`, query: { focus: nodeId } })
}

watch(current, (id) => void open(id))

onMounted(() => loadList().catch((err) => toast.error(apiErrorMessage(err))))

onBeforeUnmount(() => {
  if (timer) clearTimeout(timer)
  void flush()
})
</script>

<template>
  <div class="notes">
    <div class="top">
      <select v-model="current" class="form-select pick" title="Analyses of the investigation">
        <option v-if="!list.length" value="">No analysis yet</option>
        <option v-for="a in list" :key="a.id" :value="a.id">{{ a.title }}</option>
      </select>
      <button v-if="canEdit" class="btn btn-sm btn-secondary" title="Start another" @click="fresh">
        New
      </button>
      <router-link
        v-if="analysis"
        class="btn btn-sm btn-secondary"
        :to="`/investigations/${investigationId}/analyses/${analysis.id}`"
        title="Open on its own page"
        >&#8599;</router-link
      >
    </div>
    <p v-if="analysis" class="text-xs text-muted status">
      {{ state === 'saved' ? 'Saved' : state === 'saving' ? 'Saving...' : 'Unsaved' }}
    </p>
    <AnalysisEditor
      v-if="analysis"
      :key="analysis.id"
      :model-value="doc"
      :entities="entities"
      :editable="canEdit"
      placeholder="Notes on this sketch. Type @ to reference an entity."
      @update:model-value="changed"
      @mention="mention"
    />
    <p v-else class="text-sm text-muted hint">
      An analysis is a written account of the investigation, with references to the entities of its
      sketches.
      <template v-if="canEdit"> Start one with New.</template>
    </p>
  </div>
</template>

<style scoped>
.notes {
  display: flex;
  flex-direction: column;
  height: 100%;
  min-height: 0;
}

.top {
  display: flex;
  gap: 6px;
  padding: 12px 16px 6px;
}

.pick {
  flex: 1;
  min-width: 0;
}

.status {
  margin: 0 16px 4px;
}

.hint {
  padding: 8px 16px;
}
</style>
