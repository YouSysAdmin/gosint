<script setup lang="ts">
// The accounts of the install, for an administrator: list and search,
// create, and per account the flags, a password, a reset link to hand
// over, the second factor and passkeys to clear, the sessions to end,
// and delete. The server keeps the last enabled administrator.
import { onMounted, ref, watch } from 'vue'
import {
  createUser,
  deleteUser,
  getUser,
  listUsers,
  patchUser,
  resetLink,
  resetPasskeys,
  resetTOTP,
  revokeSessions,
} from '@/api/admin'
import { apiErrorMessage } from '@/api/client'
import type { AdminUser, Investigation } from '@/api/types'
import BaseModal from '@/components/BaseModal.vue'
import CopyButton from '@/components/CopyButton.vue'
import EmptyState from '@/components/EmptyState.vue'
import FormField from '@/components/FormField.vue'
import LoadingBlock from '@/components/LoadingBlock.vue'
import PageHeader from '@/components/PageHeader.vue'
import AdminNav from '@/views/admin/AdminNav.vue'
import { useFieldErrors } from '@/composables/fieldErrors'
import { formatDate } from '@/composables/formatDate'
import { useConfirm } from '@/composables/useConfirm'
import { useAuthStore } from '@/stores/auth'
import { useNotificationStore } from '@/stores/notification'

const auth = useAuthStore()
const toast = useNotificationStore()
const { confirm } = useConfirm()
const { capture, clear } = useFieldErrors()

const users = ref<AdminUser[]>([])
const total = ref(0)
const search = ref('')
// The first load shows the spinner, or a retry when it failed. A
// search or a refresh after an action keeps the rows on screen.
const loaded = ref(false)
const loadError = ref('')
const loading = ref(false)
const creating = ref<{ email: string; password: string; admin: boolean } | null>(null)
const editing = ref<{ user: AdminUser; owns: Investigation[]; password: string } | null>(null)
const link = ref<{ link: string; emailed: boolean } | null>(null)
const busy = ref(false)

const isSelf = (u: AdminUser) => auth.user?.id === u.id

// A search is asked a moment after the typing stops, and an older
// answer arriving late is dropped.
let asked = 0
let timer = 0
watch(search, () => {
  window.clearTimeout(timer)
  timer = window.setTimeout(load, 150)
})

async function load() {
  loading.value = true
  const n = ++asked
  try {
    const res = await listUsers(search.value.trim())
    if (n !== asked) return
    users.value = res.users
    total.value = res.total
    loaded.value = true
    loadError.value = ''
  } catch (err) {
    toast.error(apiErrorMessage(err))
    if (!loaded.value) loadError.value = apiErrorMessage(err)
  } finally {
    if (n === asked) loading.value = false
  }
}

async function open(u: AdminUser) {
  try {
    const res = await getUser(u.id)
    editing.value = { user: res.user, owns: res.owns, password: '' }
    link.value = null
  } catch (err) {
    toast.error(apiErrorMessage(err))
  }
}

// act runs a change, then reads the list and the open account again.
// The account read back is kept only if the dialog still shows it.
async function act(what: () => Promise<unknown>, done: string) {
  busy.value = true
  try {
    await what()
    if (done) toast.success(done)
    await load()
    const id = editing.value?.user.id
    if (id) {
      const res = await getUser(id)
      if (editing.value?.user.id === id) {
        editing.value = { ...editing.value, user: res.user, owns: res.owns }
      }
    }
  } catch (err) {
    if (!capture(err)) toast.error(apiErrorMessage(err))
  } finally {
    busy.value = false
  }
}

function startCreate() {
  clear()
  creating.value = { email: '', password: '', admin: false }
}

async function create() {
  const c = creating.value
  if (!c) return
  clear()
  await act(async () => {
    await createUser(c.email.trim(), c.password, c.admin)
    creating.value = null
  }, 'Account created')
}

// setFlag reads the box, and puts it back as the account says when the
// server refused: the box is bound one way and would keep the refusal.
async function setFlag(ev: Event, u: AdminUser, flag: 'admin' | 'disabled') {
  const box = ev.target as HTMLInputElement
  let applied = false
  await act(async () => {
    await patchUser(u.id, { [flag]: box.checked })
    applied = true
  }, '')
  if (!applied) box.checked = flag === 'admin' ? u.admin : !u.is_active
}

async function setPassword() {
  const e = editing.value
  if (!e || e.password.length < 8) return
  await act(async () => {
    await patchUser(e.user.id, { password: e.password })
    e.password = ''
  }, 'Password set, the account was signed out everywhere')
}

async function handLink(u: AdminUser) {
  await act(async () => {
    link.value = await resetLink(u.id)
  }, '')
}

async function removeTOTP(u: AdminUser) {
  const ok = await confirm({
    title: 'Remove the second factor',
    message: `Remove the authenticator of ${u.email}? They sign in with the password alone until they set one up again.`,
    confirmText: 'Remove',
    variant: 'warning',
  })
  if (!ok) return
  await act(() => resetTOTP(u.id), 'Second factor removed')
}

async function removePasskeys(u: AdminUser) {
  const ok = await confirm({
    title: 'Remove the passkeys',
    message: `Remove every passkey of ${u.email}? They sign in with the password until they enrol one again.`,
    confirmText: 'Remove',
    variant: 'warning',
  })
  if (!ok) return
  await act(() => resetPasskeys(u.id), 'Passkeys removed')
}

async function signOutEverywhere(u: AdminUser) {
  const ok = await confirm({
    title: 'Sign out everywhere',
    message: `End every session of ${u.email}? They have to sign in again on each device.`,
    confirmText: 'Sign out',
    variant: 'danger',
  })
  if (!ok) return
  await act(() => revokeSessions(u.id), 'Signed out everywhere')
}

async function remove(u: AdminUser) {
  const ok = await confirm({
    title: 'Delete account',
    message: `Delete ${u.email}? Their flows, chats, secrets and templates go with it. This cannot be undone.`,
    confirmText: 'Delete',
    variant: 'danger',
  })
  if (!ok) return
  await act(async () => {
    await deleteUser(u.id)
    editing.value = null
  }, 'Account deleted')
}

onMounted(load)
</script>

<template>
  <PageHeader title="Accounts" :subtitle="`${total} ${total === 1 ? 'account' : 'accounts'}`">
    <button class="btn btn-primary" @click="startCreate">Add an account</button>
  </PageHeader>
  <AdminNav />

  <LoadingBlock v-if="!loaded && !loadError" />
  <div v-else-if="!loaded" class="card">
    <EmptyState title="The accounts could not be loaded">
      <p>{{ loadError }}</p>
      <button class="btn btn-secondary" @click="load">Retry</button>
    </EmptyState>
  </div>
  <div v-else class="card">
    <div class="card-body">
      <div class="search-row mb-3">
        <input
          v-model="search"
          class="form-input"
          placeholder="Search by email"
          aria-label="Search accounts by email"
        />
        <span v-if="loading" class="text-xs text-muted">Updating...</span>
      </div>
      <table v-if="users.length">
        <thead>
          <tr>
            <th>Email</th>
            <th>Role</th>
            <th>Sign-in</th>
            <th>Since</th>
            <th></th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="u in users" :key="u.id" :class="{ muted: !u.is_active }">
            <td>
              <strong>{{ u.email }}</strong>
              <span v-if="isSelf(u)" class="text-xs text-muted ml-2">you</span>
              <span v-if="!u.is_active" class="badge badge-neutral ml-2">disabled</span>
            </td>
            <td>{{ u.admin ? 'administrator' : 'member' }}</td>
            <td class="text-sm text-muted">
              {{ u.account_type === 2 ? 'identity provider' : 'password' }}
              <span v-if="u.totp_enabled">, 2FA</span>
              <span v-if="u.passkeys"
                >, {{ u.passkeys }} {{ u.passkeys === 1 ? 'passkey' : 'passkeys' }}</span
              >
            </td>
            <td>{{ formatDate(u.created_at) }}</td>
            <td class="text-right">
              <button class="btn btn-sm btn-secondary" @click="open(u)">Manage</button>
            </td>
          </tr>
        </tbody>
      </table>
      <p v-else class="text-sm text-muted">No account matches.</p>
    </div>
  </div>

  <BaseModal
    v-if="creating"
    title="Add an account"
    size="modal-w480"
    form
    @close="creating = null"
    @submit="create"
  >
    <FormField label="Email" field="email" required>
      <input v-model="creating.email" class="form-input" type="email" required autofocus />
    </FormField>
    <FormField
      label="Password"
      field="password"
      hint="Optional. Without one, hand the person a reset link afterwards."
    >
      <input v-model="creating.password" class="form-input" type="password" minlength="8" />
    </FormField>
    <label class="checkbox-label mb-3">
      <input v-model="creating.admin" type="checkbox" />
      Administrator
    </label>
    <template #footer>
      <button type="button" class="btn btn-secondary" @click="creating = null">Cancel</button>
      <button class="btn btn-primary" :disabled="busy || !creating.email.trim()">Create</button>
    </template>
  </BaseModal>

  <BaseModal v-if="editing" :title="editing.user.email" size="modal-w560" @close="editing = null">
    <div class="flags">
      <label class="checkbox-label">
        <input
          :checked="editing.user.admin"
          type="checkbox"
          :disabled="busy || isSelf(editing.user)"
          @change="setFlag($event, editing.user, 'admin')"
        />
        Administrator
      </label>
      <label class="checkbox-label">
        <input
          :checked="!editing.user.is_active"
          type="checkbox"
          :disabled="busy || isSelf(editing.user)"
          @change="setFlag($event, editing.user, 'disabled')"
        />
        Disabled, cannot sign in
      </label>
      <p v-if="isSelf(editing.user)" class="text-xs text-muted">
        Your own flags are changed by another administrator.
      </p>
    </div>

    <template v-if="editing.user.account_type !== 2">
      <h4 class="section">Password</h4>
      <div class="row">
        <input
          v-model="editing.password"
          class="form-input"
          type="password"
          placeholder="Set a new password"
          minlength="8"
        />
        <button
          class="btn btn-secondary"
          :disabled="busy || editing.password.length < 8"
          @click="setPassword"
        >
          Set
        </button>
        <button class="btn btn-secondary" :disabled="busy" @click="handLink(editing.user)">
          Reset link
        </button>
      </div>
      <div v-if="link" class="link-box">
        <code class="text-xs">{{ link.link }}</code>
        <CopyButton :value="link.link" />
        <span class="text-xs text-muted">{{
          link.emailed
            ? 'Also mailed to the person.'
            : 'Mail is not configured, hand it over yourself.'
        }}</span>
      </div>
    </template>
    <p v-else class="text-sm text-muted">This account signs in through an identity provider.</p>

    <h4 class="section">Second factors and sessions</h4>
    <div class="row wrap">
      <button
        class="btn btn-sm btn-secondary"
        :disabled="busy || isSelf(editing.user) || !editing.user.totp_enabled"
        @click="removeTOTP(editing.user)"
      >
        Remove 2FA
      </button>
      <button
        class="btn btn-sm btn-secondary"
        :disabled="busy || isSelf(editing.user) || !editing.user.passkeys"
        @click="removePasskeys(editing.user)"
      >
        Remove passkeys
      </button>
      <button
        class="btn btn-sm btn-secondary"
        :disabled="busy"
        @click="signOutEverywhere(editing.user)"
      >
        Sign out everywhere
      </button>
    </div>

    <h4 class="section">Owns</h4>
    <p v-if="!editing.owns.length" class="text-sm text-muted">No investigations.</p>
    <ul v-else class="text-sm owns">
      <li v-for="inv in editing.owns" :key="inv.id">
        <router-link :to="`/investigations/${inv.id}`">{{ inv.name }}</router-link>
      </li>
    </ul>

    <template #footer>
      <button
        class="btn btn-danger"
        :disabled="busy || isSelf(editing.user) || editing.owns.length > 0"
        :title="editing.owns.length ? 'Hand the investigations over first' : ''"
        @click="remove(editing.user)"
      >
        Delete account
      </button>
      <button class="btn btn-secondary" @click="editing = null">Close</button>
    </template>
  </BaseModal>
</template>

<style scoped>
tr.muted td {
  color: var(--text-muted);
}

.search-row {
  display: flex;
  align-items: center;
  gap: 10px;
}

.search-row .form-input {
  flex: 1;
}

.flags {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.section {
  margin: 18px 0 8px;
  font-size: 12px;
  text-transform: uppercase;
  letter-spacing: 0.04em;
  color: var(--text-muted);
}

.row {
  display: flex;
  gap: 8px;
  align-items: center;
}

.row .form-input {
  flex: 1;
}

.wrap {
  flex-wrap: wrap;
}

.link-box {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 8px;
  margin-top: 8px;
  padding: 8px 10px;
  border-radius: var(--radius-sm);
  background: var(--bg-tertiary);
}

.link-box code {
  flex: 1 1 100%;
  overflow-wrap: anywhere;
}

.owns {
  margin: 0;
  padding-left: 18px;
}
</style>
