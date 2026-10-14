<script setup lang="ts">
// The identity providers a team signs in through. THE SECRET IS NEVER
// READ BACK: an empty field on an edit leaves the stored one alone and
// the body omits it. Lists are edited as comma separated text.
import { onMounted, ref } from 'vue'
import { apiErrorMessage } from '@/api/client'
import {
  createProvider,
  deleteProvider,
  listProviders,
  testProvider,
  updateProvider,
} from '@/api/providers'
import type { OAuthProvider, OAuthProviderInput, OAuthTestResult } from '@/api/types'
import BaseModal from '@/components/BaseModal.vue'
import CopyButton from '@/components/CopyButton.vue'
import EmptyState from '@/components/EmptyState.vue'
import FormField from '@/components/FormField.vue'
import LoadingBlock from '@/components/LoadingBlock.vue'
import PageHeader from '@/components/PageHeader.vue'
import AdminNav from '@/views/admin/AdminNav.vue'
import { useFieldErrors } from '@/composables/fieldErrors'
import { useConfirm } from '@/composables/useConfirm'
import { useNotificationStore } from '@/stores/notification'

const toast = useNotificationStore()
const { confirm } = useConfirm()
const { capture, clear } = useFieldErrors()
const providers = ref<OAuthProvider[]>([])
const tests = ref<Record<string, OAuthTestResult>>({})
const busy = ref('')
// The first load shows the spinner, or a retry when it failed. A
// refresh after a change keeps the list in place.
const loaded = ref(false)
const loadError = ref('')

interface Form {
  name: string
  slug: string
  type: string
  client_id: string
  client_secret: string
  issuer: string
  auth_url: string
  token_url: string
  userinfo_url: string
  scopes: string
  enabled: boolean
  hidden: boolean
  auto_register: boolean
  require_email_verified: boolean
  link_by_email: boolean
  allow_private_targets: boolean
  allowed_domains: string
  allowed_emails: string
  groups_claim: string
  allowed_groups: string
}

const blank = (): Form => ({
  name: '',
  slug: '',
  type: 'oidc',
  client_id: '',
  client_secret: '',
  issuer: '',
  auth_url: '',
  token_url: '',
  userinfo_url: '',
  scopes: '',
  enabled: true,
  hidden: false,
  auto_register: true,
  require_email_verified: true,
  link_by_email: false,
  allow_private_targets: false,
  allowed_domains: '',
  allowed_emails: '',
  groups_claim: '',
  allowed_groups: '',
})

const editing = ref<{ id: string; form: Form; advanced: boolean } | null>(null)

const split = (s: string) =>
  s
    .split(/[\n,]/)
    .map((x) => x.trim())
    .filter(Boolean)
const join = (v: string[] | undefined) => (v ?? []).join(', ')

async function load() {
  try {
    providers.value = await listProviders()
    loaded.value = true
    loadError.value = ''
  } catch (err) {
    toast.error(apiErrorMessage(err))
    if (!loaded.value) loadError.value = apiErrorMessage(err)
  }
}

function open(p: OAuthProvider | null) {
  clear()
  editing.value = {
    id: p?.id ?? '',
    advanced: false,
    form: p
      ? {
          name: p.name,
          slug: p.slug,
          type: p.type,
          client_id: p.client_id,
          client_secret: '',
          issuer: p.issuer,
          auth_url: p.auth_url,
          token_url: p.token_url,
          userinfo_url: p.userinfo_url,
          scopes: join(p.scopes),
          enabled: p.enabled,
          hidden: p.hidden,
          auto_register: p.auto_register,
          require_email_verified: p.require_email_verified,
          link_by_email: p.link_by_email,
          allow_private_targets: p.allow_private_targets,
          allowed_domains: join(p.allowed_domains),
          allowed_emails: join(p.allowed_emails),
          groups_claim: p.groups_claim,
          allowed_groups: join(p.allowed_groups),
        }
      : blank(),
  }
}

async function save() {
  const e = editing.value
  if (!e) return
  const f = e.form
  const body: OAuthProviderInput = {
    name: f.name,
    slug: f.slug,
    type: f.type,
    client_id: f.client_id,
    issuer: f.issuer,
    auth_url: f.auth_url,
    token_url: f.token_url,
    userinfo_url: f.userinfo_url,
    scopes: split(f.scopes),
    enabled: f.enabled,
    hidden: f.hidden,
    auto_register: f.auto_register,
    require_email_verified: f.require_email_verified,
    link_by_email: f.link_by_email,
    allow_private_targets: f.allow_private_targets,
    allowed_domains: split(f.allowed_domains),
    allowed_emails: split(f.allowed_emails),
    groups_claim: f.groups_claim,
    allowed_groups: split(f.allowed_groups),
  }
  // Only a typed secret travels, so an edit keeps the stored one.
  if (f.client_secret) body.client_secret = f.client_secret
  busy.value = 'save'
  clear()
  try {
    if (e.id) await updateProvider(e.id, body)
    else await createProvider(body)
    editing.value = null
    toast.success('Provider saved')
    await load()
  } catch (err) {
    if (!capture(err)) toast.error(apiErrorMessage(err))
  } finally {
    busy.value = ''
  }
}

async function remove(p: OAuthProvider) {
  const ok = await confirm({
    title: 'Delete provider',
    message: `Delete ${p.name}? Anyone who signs in only through it loses access until an administrator hands them a reset link.`,
    confirmText: 'Delete',
    variant: 'danger',
  })
  if (!ok) return
  try {
    await deleteProvider(p.id)
    await load()
  } catch (err) {
    toast.error(apiErrorMessage(err))
  }
}

async function runTest(p: OAuthProvider) {
  busy.value = p.id
  try {
    tests.value = { ...tests.value, [p.id]: await testProvider(p.id) }
  } catch (err) {
    toast.error(apiErrorMessage(err))
  } finally {
    busy.value = ''
  }
}

onMounted(load)
</script>

<template>
  <PageHeader title="Identity providers">
    <button class="btn btn-primary" @click="open(null)">Add a provider</button>
  </PageHeader>
  <AdminNav />

  <LoadingBlock v-if="!loaded && !loadError" />
  <div v-else-if="!loaded" class="card">
    <EmptyState title="The providers could not be loaded">
      <p>{{ loadError }}</p>
      <button class="btn btn-secondary" @click="load">Retry</button>
    </EmptyState>
  </div>
  <div v-else class="card">
    <div class="card-body">
      <p v-if="!providers.length" class="text-sm text-muted">
        No identity provider yet. Add one and the sign-in page offers it.
      </p>
      <div v-for="p in providers" :key="p.id" class="provider">
        <div class="head">
          <div>
            <strong>{{ p.name }}</strong>
            <span class="text-xs text-muted ml-2">/{{ p.slug }}</span>
            <span v-if="!p.enabled" class="badge badge-neutral ml-2">disabled</span>
            <span v-else-if="!p.usable" class="badge badge-warning ml-2">incomplete</span>
            <span v-if="p.hidden" class="badge badge-neutral ml-2">hidden</span>
          </div>
          <div class="actions">
            <button class="btn btn-sm btn-secondary" :disabled="busy === p.id" @click="runTest(p)">
              Test
            </button>
            <button class="btn btn-sm btn-secondary" @click="open(p)">Edit</button>
            <button class="btn btn-sm btn-danger" @click="remove(p)">Delete</button>
          </div>
        </div>
        <div class="text-xs text-muted callback">
          Redirect URI to register at the provider:
          <code>{{ p.callback_url }}</code>
          <CopyButton :value="p.callback_url" />
        </div>
        <div v-if="tests[p.id]" class="test text-xs" :class="tests[p.id].ok ? 'ok' : 'bad'">
          <div v-if="tests[p.id].ok">
            Discovery {{ tests[p.id].discovered ? 'answered' : 'skipped, manual endpoints' }},
            authorization at <code>{{ tests[p.id].auth_url }}</code>
          </div>
          <div v-else>{{ tests[p.id].error }}</div>
          <ul v-if="tests[p.id].warnings.length">
            <li v-for="w in tests[p.id].warnings" :key="w">{{ w }}</li>
          </ul>
        </div>
      </div>
    </div>
  </div>

  <BaseModal
    v-if="editing"
    :title="editing.id ? 'Edit provider' : 'Add a provider'"
    size="modal-w560"
    form
    @close="editing = null"
    @submit="save"
  >
    <FormField label="Name" field="name" required>
      <input v-model="editing.form.name" class="form-input" required maxlength="100" />
    </FormField>
    <div class="two">
      <FormField label="Type" field="type">
        <select v-model="editing.form.type" class="form-select">
          <option value="oidc">OpenID Connect</option>
          <option value="google">Google</option>
        </select>
      </FormField>
      <FormField
        label="Slug"
        field="slug"
        hint="In the sign-in URL, derived from the name when empty"
      >
        <input v-model="editing.form.slug" class="form-input" maxlength="60" />
      </FormField>
    </div>
    <FormField
      v-if="editing.form.type === 'oidc'"
      label="Issuer"
      field="issuer"
      hint="The provider's URL, discovery reads its document"
    >
      <input
        v-model="editing.form.issuer"
        class="form-input"
        placeholder="https://idp.example.com/realms/team"
      />
    </FormField>
    <div class="two">
      <FormField label="Client id" field="client_id">
        <input v-model="editing.form.client_id" class="form-input" />
      </FormField>
      <FormField
        label="Client secret"
        field="client_secret"
        :hint="editing.id ? 'Leave empty to keep the stored one' : ''"
      >
        <input
          v-model="editing.form.client_secret"
          class="form-input"
          type="password"
          autocomplete="off"
        />
      </FormField>
    </div>
    <label class="checkbox-label mb-2">
      <input v-model="editing.form.enabled" type="checkbox" /> Enabled
    </label>
    <label class="checkbox-label mb-2">
      <input v-model="editing.form.auto_register" type="checkbox" />
      Create an account on first sign-in (off means invited or existing accounts only)
    </label>
    <label class="checkbox-label mb-2">
      <input v-model="editing.form.require_email_verified" type="checkbox" />
      Require a verified email before linking to an existing account
    </label>
    <label class="checkbox-label">
      <input v-model="editing.form.link_by_email" type="checkbox" />
      Link to an existing account by a verified email
    </label>
    <p class="text-xs text-muted mb-2 link-hint">
      Off, a person with an account links the provider from their profile. An administrator's
      account is never linked this way.
    </p>
    <FormField
      label="Allowed domains"
      field="allowed_domains"
      hint="Comma separated, empty admits every domain"
    >
      <input v-model="editing.form.allowed_domains" class="form-input" placeholder="corp.example" />
    </FormField>
    <button
      type="button"
      class="btn btn-sm btn-secondary mb-3"
      @click="editing.advanced = !editing.advanced"
    >
      {{ editing.advanced ? 'Fewer settings' : 'More settings' }}
    </button>
    <template v-if="editing.advanced">
      <label class="checkbox-label mb-2">
        <input v-model="editing.form.hidden" type="checkbox" />
        Hidden from the sign-in page, reachable by its URL
      </label>
      <label class="checkbox-label">
        <input v-model="editing.form.allow_private_targets" type="checkbox" />
        The provider sits on a private address, inside the network
      </label>
      <p class="text-xs text-muted mb-2 link-hint">
        Off, discovery and token calls to loopback, private and link-local addresses are refused.
      </p>
      <FormField
        label="Allowed emails"
        field="allowed_emails"
        hint="Comma separated. When set, only these addresses get in and the domains and groups are not checked"
      >
        <input v-model="editing.form.allowed_emails" class="form-input" />
      </FormField>
      <div class="two">
        <FormField label="Groups claim" field="groups_claim">
          <input v-model="editing.form.groups_claim" class="form-input" placeholder="groups" />
        </FormField>
        <FormField label="Allowed groups" field="allowed_groups" hint="Comma separated">
          <input v-model="editing.form.allowed_groups" class="form-input" />
        </FormField>
      </div>
      <FormField
        label="Scopes"
        field="scopes"
        hint="Comma separated, openid email profile by default"
      >
        <input v-model="editing.form.scopes" class="form-input" />
      </FormField>
      <FormField
        label="Authorization URL"
        field="auth_url"
        hint="Manual endpoints, for a provider without discovery"
      >
        <input v-model="editing.form.auth_url" class="form-input" />
      </FormField>
      <div class="two">
        <FormField label="Token URL" field="token_url">
          <input v-model="editing.form.token_url" class="form-input" />
        </FormField>
        <FormField label="UserInfo URL" field="userinfo_url">
          <input v-model="editing.form.userinfo_url" class="form-input" />
        </FormField>
      </div>
    </template>
    <template #footer>
      <button type="button" class="btn btn-secondary" @click="editing = null">Cancel</button>
      <button class="btn btn-primary" :disabled="busy === 'save' || !editing.form.name.trim()">
        Save
      </button>
    </template>
  </BaseModal>
</template>

<style scoped>
.link-hint {
  margin-left: 24px;
}

.provider {
  padding: 12px 0;
  border-bottom: 1px solid var(--border-primary);
}

.provider:last-child {
  border-bottom: 0;
}

.head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
}

.actions {
  display: flex;
  gap: 6px;
}

.callback {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 8px;
  margin-top: 6px;
}

.callback code {
  overflow-wrap: anywhere;
}

.test {
  margin-top: 8px;
  padding: 8px 10px;
  border-radius: var(--radius-sm);
  background: var(--bg-tertiary);
}

.test.bad {
  color: var(--danger-fg);
}

.test ul {
  margin: 6px 0 0;
  padding-left: 18px;
  color: var(--text-muted);
}

.two {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 12px;
}
</style>
