<script setup lang="ts">
// Sign in: the password, then the authenticator code when the account
// asks for one, or a passkey on its own.
import { nextTick, onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { apiErrorMessage } from '@/api/client'
import {
  info as authInfo,
  login,
  loginSecondFactor,
  passkeyLoginBegin,
  passkeyLoginFinish,
} from '@/api/auth'
import type { AuthInfo } from '@/api/types'
import * as webauthn from '@/api/webauthn'
import AuthPage from '@/components/AuthPage.vue'
import FormField from '@/components/FormField.vue'
import { useAuthStore } from '@/stores/auth'

const route = useRoute()
const router = useRouter()
const auth = useAuthStore()

const email = ref('')
const password = ref('')
const code = ref('')
const codeInput = ref<HTMLInputElement | null>(null)
// An identity provider sign-in to an account with a second factor
// lands here owing the code.
const secondFactor = ref(route.query.sso_2fa === '1')
const error = ref('')
const busy = ref(false)
const passkeyBusy = ref(false)
const passkeyReady = ref(false)
const info = ref<AuthInfo | null>(null)

// What an identity provider's redirect may say went wrong.
const ssoErrors: Record<string, string> = {
  sso_idp_error: 'The identity provider reported an error',
  sso_bad_callback: 'The sign-in callback was malformed',
  sso_state_missing: 'The sign-in state expired, try again',
  sso_state_mismatch: 'The sign-in attempt did not match, try again',
  sso_token_invalid: 'The identity provider token could not be verified',
  sso_access_denied: 'Your account is not allowed to sign in',
  sso_unknown_provider: 'That sign-in provider is not available',
  sso_provider_error: 'The sign-in provider is misconfigured, ask an administrator',
  sso_no_account: 'No account exists for that identity, ask an administrator or an invitation',
  sso_link_required:
    'An account with this address exists. Sign in with it and link the provider from your profile.',
}

onMounted(async () => {
  const errCode = typeof route.query.err === 'string' ? route.query.err : ''
  // Only known codes: the query string is anyone's to write, and a free
  // sentence shown as the site's own error is a phishing page.
  if (errCode) error.value = ssoErrors[errCode] ?? 'Sign-in failed, try again'
  try {
    info.value = await authInfo()
    // A fresh install has no account to sign in to: the first one
    // registers and claims it.
    if (info.value.setup_required) {
      await router.replace({ name: 'register', query: route.query })
      return
    }
    passkeyReady.value = info.value.passkeys_enabled && (await webauthn.available())
  } catch {
    info.value = null
  }
})

function afterSignIn(invite = '') {
  // An invitation is why this person signs in at all: it wins over next.
  if (!invite) invite = typeof route.query.invite === 'string' ? route.query.invite : ''
  const next = invite
    ? `/invitations/${invite}`
    : typeof route.query.next === 'string' && route.query.next.startsWith('/')
      ? route.query.next
      : '/'
  return router.replace(next)
}

// ssoStart sends the browser to the provider, the invitation along.
function ssoStart(startUrl: string) {
  const url = new URL(startUrl, window.location.origin)
  const invite = typeof route.query.invite === 'string' ? route.query.invite : ''
  if (invite) url.searchParams.set('invite', invite)
  window.location.href = url.toString()
}

async function submit() {
  busy.value = true
  error.value = ''
  try {
    if (secondFactor.value) {
      const res = await loginSecondFactor(code.value.trim())
      if (res.user) auth.set(res.user)
      await afterSignIn(res.invite ?? '')
      return
    }
    const res = await login(email.value, password.value)
    if (res.second_factor) {
      secondFactor.value = true
      await nextTick()
      codeInput.value?.focus()
      return
    }
    if (res.user) auth.set(res.user)
    await afterSignIn()
  } catch (err) {
    error.value = apiErrorMessage(err)
  } finally {
    busy.value = false
  }
}

function startOver() {
  secondFactor.value = false
  code.value = ''
  error.value = ''
}

async function passkeyLogin() {
  passkeyBusy.value = true
  error.value = ''
  try {
    const begin = await passkeyLoginBegin()
    const assertion = await webauthn.getCredential(begin)
    auth.set(await passkeyLoginFinish(assertion))
    await afterSignIn()
  } catch (err) {
    // A refused request carries a response, a dismissed prompt does not.
    const fromServer = (err as { response?: unknown }).response
    error.value = fromServer
      ? apiErrorMessage(err)
      : webauthn.ceremonyErrorMessage(err, 'Passkey sign-in failed')
  } finally {
    passkeyBusy.value = false
  }
}
</script>

<template>
  <AuthPage title="Sign in to gosint">
    <form @submit.prevent="submit">
      <template v-if="!secondFactor">
        <FormField label="Email" for="email">
          <input
            id="email"
            v-model="email"
            class="form-input"
            type="email"
            autocomplete="username"
            required
          />
        </FormField>
        <FormField label="Password" for="password">
          <input
            id="password"
            v-model="password"
            class="form-input"
            type="password"
            autocomplete="current-password"
            required
          />
        </FormField>
      </template>
      <FormField
        v-else
        label="Authenticator or recovery code"
        for="code"
        hint="The six digits from your app, or one of your recovery codes"
      >
        <input
          id="code"
          ref="codeInput"
          v-model="code"
          class="form-input"
          maxlength="19"
          autocomplete="one-time-code"
          placeholder="123456 or xxxx-xxxx-xxxx-xxxx"
          required
        />
      </FormField>
      <p v-if="error" class="form-error">{{ error }}</p>
      <button class="btn btn-primary btn-block" :disabled="busy">
        {{ secondFactor ? 'Continue' : 'Sign in' }}
      </button>
      <button
        v-if="secondFactor"
        type="button"
        class="btn btn-secondary btn-block mt-2"
        @click="startOver"
      >
        Start over
      </button>
    </form>

    <template v-if="!secondFactor">
      <button
        v-if="passkeyReady"
        type="button"
        class="btn btn-secondary btn-block mt-3"
        :disabled="passkeyBusy"
        @click="passkeyLogin"
      >
        {{ passkeyBusy ? 'Waiting for your passkey...' : 'Sign in with a passkey' }}
      </button>
      <button
        v-for="p in info?.providers ?? []"
        :key="p.slug"
        type="button"
        class="btn btn-secondary btn-block mt-2"
        @click="ssoStart(p.start_url)"
      >
        Continue with {{ p.name }}
      </button>
      <p class="text-sm text-muted mt-3">
        <router-link to="/forgot-password">Forgot your password?</router-link>
      </p>
      <p v-if="info?.registration_open !== false" class="text-sm text-muted mt-1">
        First time here? <router-link to="/register">Create an account</router-link>
      </p>
    </template>
  </AuthPage>
</template>
