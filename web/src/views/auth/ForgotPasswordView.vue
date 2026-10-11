<script setup lang="ts">
// The server answers the same way whether or not the address has an
// account, so the page shows that answer rather than a success state
// that would confirm the address exists.
import { onMounted, ref } from 'vue'
import { info as authInfo, resetRequest } from '@/api/auth'
import { apiErrorMessage } from '@/api/client'
import AuthPage from '@/components/AuthPage.vue'
import FormField from '@/components/FormField.vue'

const email = ref('')
const busy = ref(false)
const error = ref('')
const sent = ref(false)
const mailEnabled = ref(true)

onMounted(async () => {
  try {
    mailEnabled.value = (await authInfo()).mail_enabled
  } catch {
    mailEnabled.value = true
  }
})

async function submit() {
  busy.value = true
  error.value = ''
  try {
    await resetRequest(email.value)
    sent.value = true
  } catch (err) {
    error.value = apiErrorMessage(err)
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <AuthPage title="Reset your password">
    <template v-if="sent">
      <p class="text-sm">
        If that address has an account, a reset link is on its way. The link expires in 30 minutes
        and can be used once.
      </p>
      <router-link class="btn btn-secondary btn-block mt-3" to="/login"
        >Back to sign in</router-link
      >
    </template>
    <form v-else @submit.prevent="submit">
      <p v-if="!mailEnabled" class="notice notice-warning text-sm mb-3">
        This install sends no mail. Ask an administrator for a reset link instead.
      </p>
      <p class="text-sm text-muted mb-3">
        Enter the address you sign in with and a link to choose a new password is mailed to it.
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
      <p v-if="error" class="form-error">{{ error }}</p>
      <button class="btn btn-primary btn-block" :disabled="busy">Send the link</button>
      <p class="text-sm text-muted mt-3">
        <router-link to="/login">Back to sign in</router-link>
      </p>
    </form>
  </AuthPage>
</template>
