<script setup lang="ts">
// One analysis on its own page: the title to rename in place, the
// document saving itself a moment after typing stops, and @ asking the
// server for the entities of the investigation's sketches as the person
// types. A reference clicked opens its sketch on that entity.
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { onBeforeRouteLeave, useRoute, useRouter } from 'vue-router'
import { deleteAnalysis, getAnalysis, saveAnalysis } from '@/api/analyses'
import { apiErrorMessage } from '@/api/client'
import { investigationEntities } from '@/api/catalog'
import { getInvestigation } from '@/api/investigations'
import type { Analysis, Investigation } from '@/api/types'
import AnalysisEditor from '@/components/analysis/AnalysisEditor.vue'
import type { MentionItem } from '@/components/analysis/MentionList.vue'
import EmptyState from '@/components/EmptyState.vue'
import LoadingBlock from '@/components/LoadingBlock.vue'
import { modKey } from '@/composables/modKey'
import { formatDate } from '@/composables/formatDate'
import { useConfirm } from '@/composables/useConfirm'
import { useCatalogStore } from '@/stores/catalog'
import { useNotificationStore } from '@/stores/notification'

const route = useRoute()
const router = useRouter()
const toast = useNotificationStore()
const catalog = useCatalogStore()
const { confirm } = useConfirm()

const invId = computed(() => String(route.params.id))
const id = computed(() => String(route.params.aid))
const inv = ref<Investigation | null>(null)
const analysis = ref<Analysis | null>(null)
const doc = ref<unknown>({})
const title = ref('')
const error = ref('')
const renaming = ref(false)
const titleInput = ref<HTMLInputElement | null>(null)
const state = ref<'saved' | 'dirty' | 'saving'>('saved')
let timer: ReturnType<typeof setTimeout> | null = null

// Every edit bumps rev, a save remembers the rev it sent, so an edit
// typed while it ran is seen when it lands and saved after. One save
// runs at a time, pending is the one in flight.
let rev = 0
let pending: Promise<void> | null = null

const canEdit = computed(() => inv.value?.current_user_role !== 'viewer')
const canDelete = computed(
  () => inv.value?.current_user_role === 'owner' || inv.value?.current_user_role === 'admin',
)

// mentionsFor finds what @ offers, any entity of the investigation.
async function mentionsFor(q: string): Promise<MentionItem[]> {
  const hits = await investigationEntities(invId.value, q)
  return hits.map((h) => ({
    id: h.id,
    label: h.label,
    type: h.type,
    sketch: h.sketch_id,
    sketchTitle: h.sketch_title,
  }))
}

async function load() {
  try {
    await catalog.load()
    const r = await getInvestigation(invId.value)
    inv.value = r.investigation
    analysis.value = await getAnalysis(invId.value, id.value)
    doc.value = analysis.value.content ?? {}
    title.value = analysis.value.title
  } catch (err) {
    error.value = apiErrorMessage(err)
  }
}

function mark() {
  rev++
  state.value = 'dirty'
}

function schedule() {
  if (timer) clearTimeout(timer)
  timer = setTimeout(() => void save(), 1000)
}

function changed(next: unknown) {
  doc.value = next
  mark()
  schedule()
}

function save(): Promise<void> {
  if (timer) clearTimeout(timer)
  timer = null
  if (state.value === 'saved' || !analysis.value) return Promise.resolve()
  if (pending) return pending
  pending = write().finally(() => {
    pending = null
  })

  return pending
}

async function write() {
  if (!analysis.value) return
  const sent = rev
  state.value = 'saving'
  try {
    analysis.value = await saveAnalysis(invId.value, id.value, {
      title: title.value.trim() || analysis.value.title,
      content: doc.value,
    })
    if (rev === sent) {
      state.value = 'saved'
    } else {
      state.value = 'dirty'
      schedule()
    }
  } catch (err) {
    state.value = 'dirty'
    toast.error(apiErrorMessage(err))
  }
}

// flush writes what is unsaved before the page goes, waiting for a
// save in flight first since it may leave a later edit behind.
async function flush() {
  if (pending) await pending
  if (state.value !== 'saved') await save()
}

function startRename() {
  if (!canEdit.value) return
  renaming.value = true
  void nextTick(() => titleInput.value?.focus())
}

async function rename() {
  renaming.value = false
  if (!analysis.value || !title.value.trim() || title.value.trim() === analysis.value.title) {
    title.value = analysis.value?.title ?? ''
    return
  }
  mark()
  await save()
}

async function remove() {
  if (
    !analysis.value ||
    !(await confirm({
      title: 'Delete analysis',
      message: `Delete "${analysis.value.title}"?`,
      variant: 'danger',
    }))
  )
    return
  try {
    await deleteAnalysis(invId.value, id.value)
    await router.push(`/investigations/${invId.value}`)
  } catch (err) {
    toast.error(apiErrorMessage(err))
  }
}

function openEntity(nodeId: string, sketchId: string) {
  if (sketchId) void router.push({ path: `/sketches/${sketchId}`, query: { focus: nodeId } })
}

function onKey(ev: KeyboardEvent) {
  if ((ev.metaKey || ev.ctrlKey) && (ev.key === 's' || ev.key === 'S')) {
    ev.preventDefault()
    void save()
  }
}

// The browser asks before closing the tab while an edit is unsaved.
function onUnload(ev: BeforeUnloadEvent) {
  ev.preventDefault()
}

watch(state, (s) => {
  if (s === 'saved') window.removeEventListener('beforeunload', onUnload)
  else window.addEventListener('beforeunload', onUnload)
})

onBeforeRouteLeave(async () => {
  await flush()
})

onMounted(() => {
  void load()
  window.addEventListener('keydown', onKey)
})

onBeforeUnmount(() => {
  window.removeEventListener('keydown', onKey)
  window.removeEventListener('beforeunload', onUnload)
  void flush()
})
</script>

<template>
  <div v-if="error" class="card">
    <EmptyState title="The analysis could not be opened">
      <p>{{ error }}</p>
      <router-link class="btn btn-secondary" :to="`/investigations/${invId}`">
        Back to the investigation
      </router-link>
    </EmptyState>
  </div>
  <LoadingBlock v-else-if="!analysis || !inv" />
  <div v-else class="page">
    <header class="head">
      <router-link class="back" :to="`/investigations/${invId}`">&larr; {{ inv.name }}</router-link>
      <input
        v-if="renaming"
        ref="titleInput"
        v-model="title"
        class="form-input title-input"
        @keydown.enter.prevent="rename"
        @keydown.escape="((renaming = false), (title = analysis.title))"
        @blur="rename"
      />
      <h2
        v-else
        class="title"
        :class="{ editable: canEdit }"
        :title="canEdit ? 'Click to rename' : ''"
        @click="startRename"
      >
        {{ analysis.title }}
      </h2>
      <span class="text-xs text-muted ml-auto">
        {{
          state === 'saved'
            ? 'Saved ' + formatDate(analysis.updated_at)
            : state === 'saving'
              ? 'Saving...'
              : `Unsaved, ${modKey}+S saves now`
        }}
      </span>
      <button v-if="canDelete" class="btn btn-sm btn-danger" @click="remove">Delete</button>
    </header>
    <div class="card editor">
      <AnalysisEditor
        :model-value="doc"
        :search="mentionsFor"
        :editable="canEdit"
        @update:model-value="changed"
        @mention="openEntity"
      />
    </div>
  </div>
</template>

<style scoped>
.page {
  display: flex;
  flex-direction: column;
  height: calc(100vh - var(--topbar) - 48px);
}

.head {
  display: flex;
  align-items: center;
  gap: 12px;
  margin-bottom: 12px;
}

.back {
  color: var(--text-secondary);
  text-decoration: none;
  white-space: nowrap;
}

.title {
  margin: 0;
  font-size: 18px;
}

.title.editable {
  cursor: text;
}

.title-input {
  font-size: 18px;
  font-weight: 600;
  max-width: 480px;
}

.editor {
  flex: 1;
  min-height: 0;
  overflow: hidden;
  display: flex;
  flex-direction: column;
}
</style>
