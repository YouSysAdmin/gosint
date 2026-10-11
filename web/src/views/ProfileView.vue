<script setup lang="ts">
// The account: name and avatar, the email being the sign-in and fixed.
import { computed, onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { updateMe } from '@/api/auth'
import { apiErrorMessage } from '@/api/client'
import FormField from '@/components/FormField.vue'
import PageHeader from '@/components/PageHeader.vue'
import AccountPassword from '@/components/account/AccountPassword.vue'
import AccountPasskeys from '@/components/account/AccountPasskeys.vue'
import AccountProviders from '@/components/account/AccountProviders.vue'
import AccountSessions from '@/components/account/AccountSessions.vue'
import AccountTotp from '@/components/account/AccountTotp.vue'
import { useAuthStore } from '@/stores/auth'
import { useNotificationStore } from '@/stores/notification'

const auth = useAuthStore()
const toast = useNotificationStore()
const route = useRoute()
const router = useRouter()
const first = ref('')
const last = ref('')
const avatar = ref('')
const busy = ref(false)
const sessionsCard = ref<InstanceType<typeof AccountSessions> | null>(null)

// A local account holds its own password and second factors. One an
// identity provider signs in manages those there.
const local = computed(() => auth.user?.account_type !== 2)

async function save() {
  busy.value = true
  try {
    auth.set(
      await updateMe({
        first_name: first.value.trim(),
        last_name: last.value.trim(),
        avatar_url: avatar.value.trim(),
      }),
    )
    toast.success('Profile saved')
  } catch (err) {
    toast.error(apiErrorMessage(err))
  } finally {
    busy.value = false
  }
}

// linkErrors are the codes a provider link comes back with.
const linkErrors: Record<string, string> = {
  sso_link_session: 'The link finished in another session, start it again from here',
  sso_identity_taken: 'That identity is already linked to another account',
  sso_access_denied: 'The provider does not admit that identity here',
}

// reportLink tells how a provider link went and clears the query, so a
// reload does not say it again.
function reportLink() {
  const { linked, err } = route.query
  if (typeof linked === 'string') toast.success('Provider linked')
  if (typeof err === 'string') toast.error(linkErrors[err] ?? 'Linking the provider failed')
  if (linked !== undefined || err !== undefined) void router.replace({ query: {} })
}

onMounted(async () => {
  reportLink()
  const u = await auth.load()
  first.value = u?.first_name ?? ''
  last.value = u?.last_name ?? ''
  avatar.value = u?.avatar_url ?? ''
})
</script>

<template>
  <PageHeader title="Profile" />
  <form class="card narrow" @submit.prevent="save">
    <div class="card-body">
      <div class="flex items-center gap-3 mb-4">
        <img v-if="avatar" :src="avatar" class="avatar" alt="" />
        <span v-else class="avatar initials">{{
          (first[0] ?? '') + (last[0] ?? '') || (auth.user?.email[0] ?? '?').toUpperCase()
        }}</span>
        <div>
          <div class="fw-medium">{{ auth.user?.email }}</div>
          <div class="text-xs text-muted">The email is the sign-in and cannot change here.</div>
        </div>
      </div>
      <div class="form-row">
        <FormField label="First name">
          <input v-model="first" class="form-input" maxlength="100" />
        </FormField>
        <FormField label="Last name">
          <input v-model="last" class="form-input" maxlength="100" />
        </FormField>
      </div>
      <FormField label="Avatar URL" hint="A picture shown beside your name">
        <input v-model="avatar" class="form-input" type="url" placeholder="https://" />
      </FormField>
      <button class="btn btn-primary" :disabled="busy">Save</button>
    </div>
  </form>

  <h2 class="section-title">Security</h2>
  <div class="security">
    <template v-if="local">
      <AccountPassword @changed="sessionsCard?.reload()" />
      <AccountTotp />
      <AccountPasskeys />
    </template>
    <p v-else class="text-sm text-muted">
      This account signs in through an identity provider, its password and second factor are managed
      there.
    </p>
    <AccountProviders />
    <AccountSessions ref="sessionsCard" />
  </div>
</template>

<style scoped>
.narrow {
  max-width: 560px;
}

.section-title {
  font-size: 16px;
  margin: 24px 0 12px;
}

.security {
  display: flex;
  flex-direction: column;
  gap: 16px;
  max-width: 760px;
}

.avatar {
  width: 44px;
  height: 44px;
  border-radius: 50%;
  object-fit: cover;
  border: 1px solid var(--border-primary);
}

.initials {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  background: var(--bg-tertiary);
  color: var(--text-secondary);
  font-weight: 600;
}
</style>
