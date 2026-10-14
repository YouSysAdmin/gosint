<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { apiErrorMessage } from '@/api/client'
import { searchUsers } from '@/api/auth'
import {
  addMember,
  createInvitation,
  createSketch,
  deleteInvestigation,
  exportInvestigation,
  getInvestigation,
  importSketchInto,
  listInvitations,
  listMembers,
  removeMember,
  revokeInvitation,
  setMemberRole,
  transferOwner,
  updateInvestigation,
} from '@/api/investigations'
import { createAnalysis, deleteAnalysis, listAnalyses } from '@/api/analyses'
import { deleteSketch } from '@/api/sketches'
import type {
  Analysis,
  ImportResult,
  Invitation,
  Investigation,
  Member,
  Role,
  Sketch,
} from '@/api/types'
import BaseModal from '@/components/BaseModal.vue'
import CopyButton from '@/components/CopyButton.vue'
import EmptyState from '@/components/EmptyState.vue'
import ExportDialog from '@/components/ExportDialog.vue'
import ImportFileDialog from '@/components/ImportFileDialog.vue'
import FormField from '@/components/FormField.vue'
import LoadingBlock from '@/components/LoadingBlock.vue'
import PageHeader from '@/components/PageHeader.vue'
import StatusBadge from '@/components/StatusBadge.vue'
import { timeAgo, timeUntil } from '@/composables/formatDate'
import { useConfirm } from '@/composables/useConfirm'
import { useNotificationStore } from '@/stores/notification'

const route = useRoute()
const router = useRouter()
const toast = useNotificationStore()
const { confirm } = useConfirm()
const id = computed(() => String(route.params.id))

const inv = ref<Investigation | null>(null)
const error = ref('')
const sketches = ref<Sketch[]>([])
const members = ref<Member[]>([])
const analyses = ref<Analysis[]>([])
const newSketch = ref(false)
const exporting = ref(false)
const importing = ref(false)
const sketchTitle = ref('')
const sketchDescription = ref('')
const editing = ref(false)
const editName = ref('')
const editDescription = ref('')
const sharing = ref(false)
const shareEmail = ref('')
const shareRole = ref<Role>('viewer')
const invitations = ref<Invitation[]>([])

// The share field suggests the people this account already works with.
// The server answers a prefix with colleagues only and a whole address
// with that one account, so anyone else is typed in full.
const suggestions = ref<string[]>([])
let suggestTimer: ReturnType<typeof setTimeout> | undefined
let suggestSeq = 0

watch(shareEmail, (q) => {
  clearTimeout(suggestTimer)
  const term = q.trim()
  if (term.length < 2) {
    suggestions.value = []
    return
  }

  suggestTimer = setTimeout(() => void suggest(term), 150)
})

async function suggest(term: string) {
  const seq = ++suggestSeq
  try {
    const found = await searchUsers(term)
    if (seq === suggestSeq) suggestions.value = found.map((u) => u.email)
  } catch {
    if (seq === suggestSeq) suggestions.value = []
  }
}

onBeforeUnmount(() => clearTimeout(suggestTimer))

// A share with an address that has no account becomes an invitation
// by link, shown once here and mailed when the server can.
const inviteLink = ref<{ link: string; emailed: boolean } | null>(null)

const role = computed(() => inv.value?.current_user_role)
const canCreate = computed(
  () => role.value === 'owner' || role.value === 'admin' || role.value === 'editor',
)
const canManage = computed(() => role.value === 'owner' || role.value === 'admin')

async function load() {
  try {
    const r = await getInvestigation(id.value)
    inv.value = r.investigation
    sketches.value = r.sketches
    ;[members.value, analyses.value] = await Promise.all([
      listMembers(id.value),
      listAnalyses(id.value),
    ])
    await loadInvitations()
  } catch (err) {
    if (inv.value) toast.error(apiErrorMessage(err))
    else error.value = apiErrorMessage(err)
  }
}

// openSketch is the card as a mouse target, the title is the link. A
// click on a button or link inside is theirs.
function openSketch(ev: MouseEvent, s: Sketch) {
  if ((ev.target as HTMLElement).closest('a, button')) return
  void router.push(`/sketches/${s.id}`)
}

// openAnalysis is the same for an analysis row.
function openAnalysis(ev: MouseEvent, a: Analysis) {
  if ((ev.target as HTMLElement).closest('a, button')) return
  void router.push(`/investigations/${id.value}/analyses/${a.id}`)
}

function initials(email: string): string {
  return email.split('@')[0].slice(0, 2).toUpperCase()
}

function startEdit() {
  if (!inv.value) return
  editName.value = inv.value.name
  editDescription.value = inv.value.description
  editing.value = true
}

async function saveEdit() {
  if (!inv.value) return
  try {
    inv.value = await updateInvestigation(id.value, {
      name: editName.value.trim(),
      description: editDescription.value.trim(),
      status: inv.value.status,
    })
    editing.value = false
  } catch (err) {
    toast.error(apiErrorMessage(err))
  }
}

async function create() {
  try {
    const s = await createSketch(id.value, sketchTitle.value, sketchDescription.value)
    await router.push(`/sketches/${s.id}`)
  } catch (err) {
    toast.error(apiErrorMessage(err))
  }
}

// An imported sketch opens at once, its import summary as a toast.
async function imported(result: unknown) {
  const r = result as { sketch: Sketch; result: ImportResult }
  importing.value = false
  const summary = `${r.result.nodes_created} entities and ${r.result.edges_created} relationships`
  if (r.result.errors.length) toast.info(`${summary}, ${r.result.errors.length} skipped`)
  else toast.success(`${summary} imported`)
  await router.push(`/sketches/${r.sketch.id}`)
}

async function loadInvitations() {
  if (!canManage.value) return
  try {
    invitations.value = await listInvitations(id.value)
  } catch {
    invitations.value = []
  }
}

// closeShare leaves nothing of one share for the next: the link shown
// once is gone, the field and its suggestions are empty.
function closeShare() {
  sharing.value = false
  inviteLink.value = null
  shareEmail.value = ''
  suggestions.value = []
}

async function share() {
  try {
    await addMember(id.value, shareEmail.value, shareRole.value)
    closeShare()
    members.value = await listMembers(id.value)
  } catch (err) {
    const status = (err as { response?: { status?: number } }).response?.status
    if (status !== 404) {
      toast.error(apiErrorMessage(err))
      return
    }
    // No account yet: invite the address instead.
    try {
      const res = await createInvitation(id.value, shareEmail.value, shareRole.value)
      inviteLink.value = { link: res.link, emailed: res.emailed }
      shareEmail.value = ''
      await loadInvitations()
    } catch (err2) {
      toast.error(apiErrorMessage(err2))
    }
  }
}

async function revoke(i: Invitation) {
  try {
    await revokeInvitation(id.value, i.id)
    await loadInvitations()
  } catch (err) {
    toast.error(apiErrorMessage(err))
  }
}

async function makeOwner(m: Member) {
  if (
    !(await confirm({
      title: 'Hand the investigation over',
      message: `Make ${m.user.email} the owner? You become an admin on it.`,
      variant: 'danger',
    }))
  )
    return
  try {
    await transferOwner(id.value, m.user_id)
    await load()
  } catch (err) {
    toast.error(apiErrorMessage(err))
  }
}

// A refused role goes back to the one the member has: the select is
// bound one way and would keep showing the refusal.
async function changeRole(ev: Event, m: Member) {
  const sel = ev.target as HTMLSelectElement
  try {
    await setMemberRole(id.value, m.user_id, sel.value as Role)
    members.value = await listMembers(id.value)
  } catch (err) {
    sel.value = m.role
    toast.error(apiErrorMessage(err))
  }
}

async function remove(m: Member) {
  if (
    !(await confirm({
      title: 'Remove member',
      message: `Remove ${m.user.email} from this investigation?`,
      variant: 'danger',
    }))
  )
    return
  try {
    await removeMember(id.value, m.user_id)
    members.value = await listMembers(id.value)
  } catch (err) {
    toast.error(apiErrorMessage(err))
  }
}

async function setStatus(ev: Event) {
  const sel = ev.target as HTMLSelectElement
  const status = sel.value
  if (status !== 'active' && status !== 'archived') return
  if (!inv.value) return
  try {
    inv.value = await updateInvestigation(id.value, {
      name: inv.value.name,
      description: inv.value.description,
      status,
    })
  } catch (err) {
    sel.value = inv.value.status
    toast.error(apiErrorMessage(err))
  }
}

async function newAnalysis() {
  try {
    const a = await createAnalysis(id.value, 'Untitled analysis')
    await router.push(`/investigations/${id.value}/analyses/${a.id}`)
  } catch (err) {
    toast.error(apiErrorMessage(err))
  }
}

async function removeAnalysis(a: Analysis) {
  if (
    !(await confirm({
      title: 'Delete analysis',
      message: `Delete "${a.title}"?`,
      variant: 'danger',
    }))
  )
    return
  try {
    await deleteAnalysis(id.value, a.id)
    analyses.value = analyses.value.filter((x) => x.id !== a.id)
  } catch (err) {
    toast.error(apiErrorMessage(err))
  }
}

async function removeSketch(s: Sketch) {
  if (
    !(await confirm({
      title: 'Delete sketch',
      message: `Delete ${s.title} with its graph, scans and log? This cannot be undone.`,
      confirmText: 'Delete sketch',
      variant: 'danger',
    }))
  )
    return
  try {
    await deleteSketch(s.id)
    sketches.value = sketches.value.filter((x) => x.id !== s.id)
  } catch (err) {
    toast.error(apiErrorMessage(err))
  }
}

async function destroy() {
  if (
    !(await confirm({
      title: 'Delete investigation',
      message: 'This deletes every sketch and graph in it, for everyone.',
      confirmText: 'Delete',
      variant: 'danger',
    }))
  )
    return
  try {
    await deleteInvestigation(id.value)
    await router.push('/investigations')
  } catch (err) {
    toast.error(apiErrorMessage(err))
  }
}

onMounted(load)
</script>

<template>
  <div v-if="error" class="card">
    <EmptyState title="The investigation could not be opened">
      <p>{{ error }}</p>
      <router-link class="btn btn-secondary" to="/investigations"
        >Back to investigations</router-link
      >
    </EmptyState>
  </div>
  <LoadingBlock v-else-if="!inv" />
  <template v-else>
    <PageHeader :title="inv.name">
      <select
        v-if="canManage"
        class="form-select status"
        :value="inv.status"
        title="Status of the investigation"
        @change="setStatus"
      >
        <option value="active">Active</option>
        <option value="archived">Archived</option>
      </select>
      <span v-else-if="inv.status !== 'active'" class="badge badge-neutral">{{ inv.status }}</span>
      <button v-if="canManage" class="btn btn-secondary" @click="startEdit">Edit</button>
      <button class="btn btn-secondary" @click="exporting = true">Export</button>
      <button v-if="canCreate" class="btn btn-secondary" @click="importing = true">
        Import sketch
      </button>
      <button v-if="canCreate" class="btn btn-primary" @click="newSketch = true">New sketch</button>
      <button v-if="role === 'owner'" class="btn btn-danger" @click="destroy">Delete</button>
    </PageHeader>
    <div class="meta mb-4">
      <p class="text-secondary">{{ inv.description || 'No description' }}</p>
      <div class="people" :title="members.map((m) => m.user.email).join(', ')">
        <span v-for="m in members.slice(0, 6)" :key="m.user_id" class="avatar">{{
          initials(m.user.email)
        }}</span>
        <span class="text-xs text-muted"
          >{{ members.length }} {{ members.length === 1 ? 'investigator' : 'investigators' }}</span
        >
      </div>
    </div>

    <div class="section-head">
      <h3>Sketches</h3>
      <div class="section-actions">
        <button v-if="canCreate" class="btn btn-sm btn-secondary" @click="importing = true">
          Import
        </button>
        <button v-if="canCreate" class="btn btn-sm btn-secondary" @click="newSketch = true">
          + New sketch
        </button>
      </div>
    </div>
    <div v-if="!sketches.length" class="card mb-4">
      <EmptyState title="No sketch yet" text="A sketch is one graph of entities." />
    </div>
    <div v-else class="sketches mb-4">
      <div
        v-for="s in sketches"
        :key="s.id"
        class="card sk"
        :class="{ archived: s.status === 'archived' }"
        @click="openSketch($event, s)"
      >
        <div class="flex items-center gap-2">
          <router-link class="name" :to="`/sketches/${s.id}`">{{ s.title }}</router-link>
          <span v-if="s.status === 'archived'" class="badge badge-neutral">archived</span>
          <button
            v-if="role === 'owner'"
            class="btn btn-sm btn-secondary ml-auto"
            title="Delete the sketch"
            @click.stop="removeSketch(s)"
          >
            Delete
          </button>
        </div>
        <p class="desc text-sm text-muted">{{ s.description || 'No description' }}</p>
        <div class="text-xs text-muted">Updated {{ timeAgo(s.updated_at) }}</div>
      </div>
    </div>

    <div class="section-head">
      <h3>Analyses</h3>
      <button v-if="canCreate" class="btn btn-sm btn-secondary" @click="newAnalysis">
        + New analysis
      </button>
    </div>
    <div v-if="!analyses.length" class="card mb-4">
      <EmptyState
        title="No analysis yet"
        text="An analysis is a written account of the case, with references to its entities."
      />
    </div>
    <div v-else class="card mb-4 table-wrapper">
      <table class="table">
        <tbody>
          <tr
            v-for="a in analyses"
            :key="a.id"
            class="row-clickable"
            @click="openAnalysis($event, a)"
          >
            <td class="cell-title">
              <router-link class="row-link" :to="`/investigations/${id}/analyses/${a.id}`">{{
                a.title
              }}</router-link>
            </td>
            <td class="cell-nowrap text-muted text-right">updated {{ timeAgo(a.updated_at) }}</td>
            <td v-if="canManage" class="cell-nowrap text-right">
              <button class="btn btn-sm btn-secondary" @click.stop="removeAnalysis(a)">
                Delete
              </button>
            </td>
          </tr>
        </tbody>
      </table>
    </div>

    <div class="card">
      <div class="card-header flex justify-between items-center">
        <h3>Members</h3>
        <button v-if="canManage" class="btn btn-sm btn-secondary" @click="sharing = true">
          Share
        </button>
      </div>
      <div class="table-wrapper">
        <table class="table">
          <tbody>
            <tr v-for="m in members" :key="m.user_id">
              <td>{{ m.user.email }}</td>
              <td>
                <select
                  v-if="canManage && m.role !== 'owner'"
                  class="form-select"
                  :value="m.role"
                  @change="changeRole($event, m)"
                >
                  <option value="admin">admin</option>
                  <option value="editor">editor</option>
                  <option value="viewer">viewer</option>
                </select>
                <StatusBadge v-else :status="m.role" scope="role" />
              </td>
              <td class="text-right">
                <button
                  v-if="role === 'owner' && m.role !== 'owner'"
                  class="btn btn-sm btn-secondary"
                  @click="makeOwner(m)"
                >
                  Make owner
                </button>
                <button
                  v-if="canManage && m.role !== 'owner'"
                  class="btn btn-sm btn-secondary"
                  @click="remove(m)"
                >
                  Remove
                </button>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
      <div v-if="canManage && invitations.length" class="card-body">
        <div class="text-xs text-muted mb-2">Invited, not yet joined</div>
        <table class="table">
          <tbody>
            <tr v-for="i in invitations" :key="i.id">
              <td>{{ i.email }}</td>
              <td><StatusBadge :status="i.role" scope="role" /></td>
              <td class="text-xs text-muted">expires {{ timeUntil(i.expires_at) }}</td>
              <td class="text-right">
                <button class="btn btn-sm btn-secondary" @click="revoke(i)">Revoke</button>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>

    <ExportDialog
      v-if="exporting"
      title="Export investigation"
      what="The investigation with every sketch, its files and the analyses"
      :exporter="(p) => exportInvestigation(id, p)"
      @close="exporting = false"
    />
    <ImportFileDialog
      v-if="importing"
      title="Import a sketch"
      what="An export of a sketch becomes a new sketch here, named as the file says."
      :importer="(f, p) => importSketchInto(id, f, p)"
      @close="importing = false"
      @done="imported"
    />

    <BaseModal
      v-if="newSketch"
      title="New sketch"
      size="modal-w480"
      form
      @close="newSketch = false"
      @submit="create"
    >
      <FormField label="Title" required>
        <input v-model="sketchTitle" class="form-input" required maxlength="200" autofocus />
      </FormField>
      <FormField label="Description">
        <textarea v-model="sketchDescription" class="form-textarea" rows="2" />
      </FormField>
      <template #footer>
        <button type="button" class="btn btn-secondary" @click="newSketch = false">Cancel</button>
        <button class="btn btn-primary">Create</button>
      </template>
    </BaseModal>

    <BaseModal
      v-if="editing"
      title="Edit investigation"
      size="modal-w480"
      form
      @close="editing = false"
      @submit="saveEdit"
    >
      <FormField label="Name" required>
        <input v-model="editName" class="form-input" required maxlength="200" autofocus />
      </FormField>
      <FormField label="Description">
        <textarea v-model="editDescription" class="form-textarea" rows="3" />
      </FormField>
      <template #footer>
        <button type="button" class="btn btn-secondary" @click="editing = false">Cancel</button>
        <button class="btn btn-primary" :disabled="!editName.trim()">Save</button>
      </template>
    </BaseModal>

    <BaseModal
      v-if="sharing"
      title="Share investigation"
      size="modal-w480"
      form
      @close="closeShare"
      @submit="share"
    >
      <div v-if="inviteLink" class="notice notice-info text-sm mb-3">
        <p class="mb-2">
          {{
            inviteLink.emailed
              ? 'No account yet, so an invitation was mailed. The link, in case it is needed:'
              : 'No account yet, so an invitation was made. Mail is not configured, hand the link over:'
          }}
        </p>
        <code class="text-xs invite-link">{{ inviteLink.link }}</code>
        <CopyButton class="mt-2" :value="inviteLink.link" label="Copy the link" />
      </div>
      <FormField
        label="Email"
        hint="People you already work with are suggested. Anyone else is added by their full address, and an address without an account is invited by link."
        required
      >
        <input
          v-model="shareEmail"
          class="form-input"
          type="email"
          list="share-suggestions"
          autocomplete="off"
          required
        />
        <datalist id="share-suggestions">
          <option v-for="e in suggestions" :key="e" :value="e" />
        </datalist>
      </FormField>
      <FormField label="Role">
        <select v-model="shareRole" class="form-select">
          <option value="viewer">viewer, reads</option>
          <option value="editor">editor, reads and changes sketches</option>
          <option value="admin">admin, also manages members</option>
        </select>
      </FormField>
      <template #footer>
        <button type="button" class="btn btn-secondary" @click="closeShare">Cancel</button>
        <button class="btn btn-primary">Share</button>
      </template>
    </BaseModal>
  </template>
</template>

<style scoped>
.invite-link {
  display: block;
  overflow-wrap: anywhere;
}

.meta {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 16px;
}

.meta p {
  margin: 0;
}

.people {
  display: flex;
  align-items: center;
  gap: 4px;
  flex: none;
}

.avatar {
  width: 26px;
  height: 26px;
  border-radius: 50%;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  font-size: 10px;
  font-weight: 600;
  background: var(--bg-tertiary);
  border: 1px solid var(--border-primary);
  color: var(--text-secondary);
  margin-left: -6px;
}

.avatar:first-child {
  margin-left: 0;
}

.section-actions {
  display: flex;
  gap: 8px;
}

.section-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 8px;
}

.sketches {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(260px, 1fr));
  gap: 12px;
}

/* The stacking margin a card gets after another would push every card
   but the first down a row of the grid. */
.sketches > .card + .card {
  margin-top: 0;
}

.sk {
  display: flex;
  flex-direction: column;
  gap: 6px;
  padding: 14px 16px;
  cursor: pointer;
}

.sk:hover {
  background: var(--bg-hover);
}

.sk .name {
  font-weight: 600;
  color: inherit;
  text-decoration: none;
}

.sk .desc {
  margin: 0;
  display: -webkit-box;
  -webkit-line-clamp: 2;
  -webkit-box-orient: vertical;
  overflow: hidden;
  min-height: 2.6em;
}

/* The date sits at the bottom, whatever the description's length. */
.sk > :last-child {
  margin-top: auto;
}

.sk.archived .name {
  color: var(--text-muted);
}

.status {
  width: auto;
}
</style>
