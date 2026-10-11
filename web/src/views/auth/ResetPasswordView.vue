<script setup lang="ts">
import { computed, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { resetConfirm } from '@/api/auth'
import { apiErrorMessage } from '@/api/client'
import AuthPage from '@/components/AuthPage.vue'
import FormField from '@/components/FormField.vue'

const route = useRoute()
const router = useRouter()

const token = computed(() => (typeof route.query.token === 'string' ? route.query.token : ''))
const password = ref('')
const again = ref('')
const busy = ref(false)
const error = ref('')
const done = ref(false)

// Mirrors the server rule (min 8) so a mismatch is caught before a
// round trip.
const tooShort = computed(() => password.value.length > 0 && password.value.length < 8)
const mismatch = computed(() => again.value.length > 0 && again.value !== password.value)
const canSubmit = computed(
  () => token.value !== '' && password.value.length >= 8 && again.value === password.value,
)

async function submit() {
  if (!canSubmit.value) return
  busy.value = true
  error.value = ''
  try {
    await resetConfirm(token.value, password.value)
    done.value = true
    setTimeout(() => router.push('/login'), 2500)
  } catch (err) {
    error.value = apiErrorMessage(err)
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <AuthPage title="Choose a new password">
    <template v-if="done">
      <p class="text-sm">Password updated. Taking you to the sign-in page.</p>
      <router-link class="btn btn-primary btn-block mt-3" to="/login">Sign in</router-link>
    </template>
    <template v-else-if="!token">
      <p class="text-sm">
        This link is missing its token. Request a new one and open the most recent message.
      </p>
      <router-link class="btn btn-secondary btn-block mt-3" to="/forgot-password">
        Request a new link
      </router-link>
    </template>
    <form v-else @submit.prevent="submit">
      <FormField
        label="New password"
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
      <button class="btn btn-primary btn-block" :disabled="busy || !canSubmit">Set password</button>
      <p class="text-sm text-muted mt-3">
        <router-link to="/login">Back to sign in</router-link>
      </p>
    </form>
  </AuthPage>
</template>
