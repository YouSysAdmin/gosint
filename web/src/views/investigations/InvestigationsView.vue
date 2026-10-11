<script setup lang="ts">
// The investigations: searchable, filtered by status, as cards or a
// list, archived ones sinking to the bottom.
import { computed, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { apiErrorMessage } from '@/api/client'
import { createInvestigation, importInvestigation, listInvestigations } from '@/api/investigations'
import type { Investigation, InvestigationImportResult } from '@/api/types'
import BaseModal from '@/components/BaseModal.vue'
import EmptyState from '@/components/EmptyState.vue'
import FormField from '@/components/FormField.vue'
import ImportFileDialog from '@/components/ImportFileDialog.vue'
import LoadingBlock from '@/components/LoadingBlock.vue'
import PageHeader from '@/components/PageHeader.vue'
import StatusBadge from '@/components/StatusBadge.vue'
import { timeAgo } from '@/composables/formatDate'
import { storageGet, storageSet } from '@/composables/storage'
import { useNotificationStore } from '@/stores/notification'

type Filter = 'all' | 'active' | 'archived'

const router = useRouter()
const toast = useNotificationStore()
const list = ref<Investigation[]>([])
const loading = ref(true)
const creating = ref(false)
const importing = ref(false)
const name = ref('')
const description = ref('')
const search = ref('')
const filter = ref<Filter>('all')
const layout = ref<'cards' | 'list'>(
  storageGet('gosint_investigations_layout') === 'list' ? 'list' : 'cards',
)

const counts = computed(() => ({
  all: list.value.length,
  active: list.value.filter((i) => i.status === 'active').length,
  archived: list.value.filter((i) => i.status === 'archived').length,
}))

const shown = computed(() => {
  const q = search.value.trim().toLowerCase()
  return [...list.value]
    .filter((i) => filter.value === 'all' || i.status === filter.value)
    .filter(
      (i) => !q || i.name.toLowerCase().includes(q) || i.description.toLowerCase().includes(q),
    )
    .sort(
      (a, b) =>
        Number(a.status === 'archived') - Number(b.status === 'archived') ||
        b.updated_at.localeCompare(a.updated_at),
    )
})

function setLayout(l: 'cards' | 'list') {
  layout.value = l
  storageSet('gosint_investigations_layout', l)
}

async function load() {
  try {
    list.value = await listInvestigations()
  } catch (err) {
    toast.error(apiErrorMessage(err))
  } finally {
    loading.value = false
  }
}

async function create() {
  try {
    const inv = await createInvestigation(name.value, description.value)
    creating.value = false
    await router.push(`/investigations/${inv.id}`)
  } catch (err) {
    toast.error(apiErrorMessage(err))
  }
}

// An imported investigation opens at once, its import summary as a toast.
async function imported(result: unknown) {
  const r = result as InvestigationImportResult
  importing.value = false
  const summary = `${r.sketches.length} sketches, ${r.nodes_created} entities, ${r.analyses_created} analyses`
  if (r.errors.length) toast.info(`${summary}, ${r.errors.length} skipped`)
  else toast.success(`${summary} imported`)
  await router.push(`/investigations/${r.investigation.id}`)
}

onMounted(load)
</script>

<template>
  <PageHeader title="Investigations" subtitle="Each one holds the sketches and people of a case">
    <button class="btn btn-secondary" @click="importing = true">Import</button>
    <button class="btn btn-primary" @click="creating = true">New investigation</button>
  </PageHeader>

  <LoadingBlock v-if="loading" />
  <div v-else-if="!list.length" class="card">
    <EmptyState
      title="No investigation yet"
      text="An investigation holds the sketches and people of one case."
    />
  </div>
  <template v-else>
    <div class="toolbar">
      <input v-model="search" class="form-input w-search" placeholder="Search investigations" />
      <div class="chips">
        <button
          v-for="f in ['all', 'active', 'archived'] as Filter[]"
          :key="f"
          class="chip"
          :class="{ on: filter === f }"
          @click="filter = f"
        >
          {{ f[0].toUpperCase() + f.slice(1) }} <span class="text-muted">{{ counts[f] }}</span>
        </button>
      </div>
      <div class="segmented ml-auto">
        <button :class="{ active: layout === 'cards' }" @click="setLayout('cards')">Cards</button>
        <button :class="{ active: layout === 'list' }" @click="setLayout('list')">List</button>
      </div>
    </div>

    <p v-if="!shown.length" class="text-muted">No investigation matches.</p>

    <div v-else-if="layout === 'cards'" class="cards">
      <button
        v-for="i in shown"
        :key="i.id"
        class="card inv"
        :class="{ archived: i.status === 'archived' }"
        @click="router.push(`/investigations/${i.id}`)"
      >
        <div class="flex items-center gap-2">
          <span class="status-dot" :class="i.status"></span>
          <span class="name">{{ i.name }}</span>
          <StatusBadge :status="i.current_user_role" scope="role" class="ml-auto" />
        </div>
        <p class="desc text-sm text-muted">{{ i.description || 'No description' }}</p>
        <div class="text-xs text-muted">
          {{ i.status === 'archived' ? 'Archived, ' : '' }}updated {{ timeAgo(i.updated_at) }}
        </div>
      </button>
    </div>

    <div v-else class="card table-wrapper">
      <table class="table">
        <thead>
          <tr>
            <th>Name</th>
            <th>Your role</th>
            <th>Status</th>
            <th>Changed</th>
          </tr>
        </thead>
        <tbody>
          <tr
            v-for="i in shown"
            :key="i.id"
            class="row-clickable"
            :class="{ archived: i.status === 'archived' }"
            @click="router.push(`/investigations/${i.id}`)"
          >
            <td class="cell-title">{{ i.name }}</td>
            <td><StatusBadge :status="i.current_user_role" scope="role" /></td>
            <td>
              <span v-if="i.status === 'archived'" class="badge badge-neutral">archived</span>
              <template v-else>active</template>
            </td>
            <td class="cell-nowrap">{{ timeAgo(i.updated_at) }}</td>
          </tr>
        </tbody>
      </table>
    </div>
  </template>

  <ImportFileDialog
    v-if="importing"
    title="Import an investigation"
    what="An export of an investigation, or of one sketch, becomes a new investigation you own."
    :importer="(f, p) => importInvestigation(f, p)"
    @close="importing = false"
    @done="imported"
  />

  <BaseModal
    v-if="creating"
    title="New investigation"
    size="modal-w480"
    form
    @close="creating = false"
    @submit="create"
  >
    <FormField label="Name" required>
      <input v-model="name" class="form-input" required maxlength="200" autofocus />
    </FormField>
    <FormField label="Description">
      <textarea v-model="description" class="form-textarea" rows="3" />
    </FormField>
    <template #footer>
      <button type="button" class="btn btn-secondary" @click="creating = false">Cancel</button>
      <button class="btn btn-primary">Create</button>
    </template>
  </BaseModal>
</template>

<style scoped>
.toolbar {
  display: flex;
  align-items: center;
  gap: 12px;
  margin-bottom: 16px;
  flex-wrap: wrap;
}

.chips {
  display: flex;
  gap: 4px;
}

.chip {
  padding: 4px 10px;
  font-size: 12px;
  border: 1px solid var(--border-primary);
  border-radius: 999px;
  background: var(--bg-primary);
  color: var(--text-secondary);
  cursor: pointer;
}

.chip.on {
  background: var(--bg-active);
  color: var(--text-primary);
}

.segmented {
  display: inline-flex;
  border: 1px solid var(--border-button);
  border-radius: var(--radius-sm);
  overflow: hidden;
}

.segmented button {
  padding: 4px 10px;
  font-size: 12px;
  background: var(--bg-primary);
  color: var(--text-secondary);
  border: 0;
  cursor: pointer;
}

.segmented button + button {
  border-left: 1px solid var(--border-button);
}

.segmented button.active {
  background: var(--bg-active);
  color: var(--text-primary);
}

.cards {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(280px, 1fr));
  gap: 12px;
}

/* The stacking margin a card gets after another would push every card
   but the first down a row of the grid. */
.cards > .card + .card {
  margin-top: 0;
}

.inv {
  display: flex;
  flex-direction: column;
  gap: 8px;
  padding: 14px 16px;
  text-align: left;
  cursor: pointer;
  color: var(--text-primary);
}

.inv:hover {
  background: var(--bg-hover);
}

.inv .name {
  font-weight: 600;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.inv .desc {
  margin: 0;
  display: -webkit-box;
  -webkit-line-clamp: 2;
  -webkit-box-orient: vertical;
  overflow: hidden;
  min-height: 2.6em;
}

/* The date sits at the bottom, whatever the description's length. */
.inv > :last-child {
  margin-top: auto;
}

.archived .name,
.archived .cell-title {
  color: var(--text-muted);
}

.status-dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  flex: none;
  background: var(--text-muted);
}

.status-dot.active {
  background: #22c55e;
}
</style>
