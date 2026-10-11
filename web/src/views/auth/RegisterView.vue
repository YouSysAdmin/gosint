<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { apiErrorMessage } from '@/api/client'
import { info as authInfo, register } from '@/api/auth'
import AuthPage from '@/components/AuthPage.vue'
import FormField from '@/components/FormField.vue'
import { useAuthStore } from '@/stores/auth'

const route = useRoute()
const router = useRouter()
const auth = useAuthStore()

// The sign-in link keeps the invitation, so either way lands on it.
const invite = typeof route.query.invite === 'string' ? route.query.invite : ''

const email = ref('')
const password = ref('')
const again = ref('')
const error = ref('')
const busy = ref(false)

// The first account of a fresh install becomes its administrator.
const setup = ref(false)

// Mirrors the server rule (min 8) so a mismatch is caught before a
// round trip.
const tooShort = computed(() => password.value.length > 0 && password.value.length < 8)
const mismatch = computed(() => again.value.length > 0 && again.value !== password.value)
const canSubmit = computed(
  () => email.value !== '' && password.value.length >= 8 && again.value === password.value,
)

onMounted(async () => {
  try {
    setup.value = (await authInfo()).setup_required
  } catch {
    setup.value = false
  }
})

async function submit() {
  if (!canSubmit.value) return
  busy.value = true
  error.value = ''
  try {
    auth.set(await register(email.value, password.value, invite))
    // An invitation is why this person registers at all: it wins over next.
    const next = invite
      ? `/invitations/${invite}`
      : typeof route.query.next === 'string' && route.query.next.startsWith('/')
        ? route.query.next
        : '/'
    await router.replace(next)
  } catch (err) {
    error.value = apiErrorMessage(err)
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <AuthPage :title="setup ? 'Set up gosint' : 'Create an account'">
    <form @submit.prevent="submit">
      <p v-if="setup" class="text-sm text-muted mb-3">
        No account exists yet. This one becomes the administrator of the install.
      </p>
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
      <FormField
        label="Password"
        for="password"
        hint="At least 8 characters"
        :error="tooShort ? 'At least 8 characters' : ''"
      >
        <input
          id="password"
          v-model="password"
          class="form-input"
          type="password"
          autocomplete="new-password"
          minlength="8"
          required
        />
      </FormField>
      <FormField
        label="Confirm password"
        for="again"
        :error="mismatch ? 'The two passwords do not match' : ''"
      >
        <input
          id="again"
          v-model="again"
          class="form-input"
          type="password"
          autocomplete="new-password"
          required
        />
      </FormField>
      <p v-if="error" class="form-error">{{ error }}</p>
      <button class="btn btn-primary btn-block" :disabled="busy || !canSubmit">
        {{ setup ? 'Create the administrator' : 'Create account' }}
      </button>
      <p v-if="!setup" class="text-sm text-muted mt-3">
        Already have one?
        <router-link :to="{ name: 'login', query: invite ? { invite } : {} }">Sign in</router-link>
      </p>
    </form>
  </AuthPage>
</template>
