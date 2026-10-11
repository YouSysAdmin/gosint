<script setup lang="ts">
// A sealed server opens nothing until its unseal key unwraps the master
// key. The first time, the unseal key is chosen here, and it cannot be
// recovered: without it the data stays sealed for good.
import { computed, onMounted, ref } from 'vue'
import { useRoute } from 'vue-router'
import { initialize, sealStatus, unseal, type SealStatus } from '@/api/sys'
import { apiErrorMessage } from '@/api/client'
import AuthPage from '@/components/AuthPage.vue'
import FormField from '@/components/FormField.vue'

const route = useRoute()
const status = ref<SealStatus | null>(null)
const unsealKey = ref('')
const again = ref('')
const busy = ref(false)
const error = ref('')

const firstTime = computed(() => status.value !== null && !status.value.initialized)
const title = computed(() => (firstTime.value ? 'Protect this server' : 'Unseal this server'))

// Back where the person was, a full load so every store starts fresh.
function leave() {
  const next =
    typeof route.query.next === 'string' && route.query.next.startsWith('/')
      ? route.query.next
      : '/'
  location.assign(import.meta.env.BASE_URL + next.replace(/^\//, ''))
}

onMounted(async () => {
  try {
    status.value = await sealStatus()
    if (!status.value.sealed) leave()
  } catch (err) {
    error.value = apiErrorMessage(err)
  }
})

async function submit() {
  error.value = ''
  if (firstTime.value) {
    if (unsealKey.value.length < (status.value?.min_unseal_key ?? 12)) {
      error.value = `The unseal key needs ${status.value?.min_unseal_key ?? 12} characters or more`
      return
    }

    if (unsealKey.value !== again.value) {
      error.value = 'The two unseal keys differ'
      return
    }
  }

  busy.value = true
  try {
    status.value = firstTime.value
      ? await initialize(unsealKey.value)
      : await unseal(unsealKey.value)
    unsealKey.value = ''
    again.value = ''
    leave()
  } catch (err) {
    error.value = apiErrorMessage(err)
    unsealKey.value = ''
    again.value = ''
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <AuthPage :title="title">
    <p v-if="!status && !error" class="text-sm text-muted">Loading...</p>
    <form v-else-if="status" @submit.prevent="submit">
      <template v-if="firstTime">
        <p class="text-sm text-muted mb-3">
          Everything this server stores is encrypted, and only this unseal key opens it. The server
          starts sealed after every restart and waits for it.
        </p>
        <p class="notice notice-warning text-sm mb-3">
          The unseal key cannot be recovered. Without it the data cannot be read by anyone.
        </p>
      </template>
      <p v-else class="text-sm text-muted mb-3">The server restarted and holds no key.</p>
      <p class="notice notice-warning text-sm mb-3">Enter the unseal key to unseal the data.</p>
      <FormField label="Unseal key" for="unseal-key">
        <input
          id="unseal-key"
          v-model="unsealKey"
          class="form-input"
          type="password"
          :autocomplete="firstTime ? 'new-password' : 'off'"
          data-1p-ignore
          data-lpignore="true"
          required
        />
      </FormField>
      <FormField v-if="firstTime" label="Unseal key again" for="again">
        <input
          id="again"
          v-model="again"
          class="form-input"
          type="password"
          autocomplete="new-password"
          data-1p-ignore
          data-lpignore="true"
          required
        />
      </FormField>
      <p v-if="error" class="form-error">{{ error }}</p>
      <button class="btn btn-primary btn-block" :disabled="busy">
        {{ busy ? 'Opening...' : firstTime ? 'Encrypt and unseal' : 'Unseal' }}
      </button>
    </form>
    <p v-else class="form-error">{{ error }}</p>
  </AuthPage>
</template>
