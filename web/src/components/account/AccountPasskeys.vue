<script setup lang="ts">
// Passkeys on this account. A passkey signs in ON ITS OWN, so adding and
// removing one asks for the password first.
import { ref } from 'vue'
import { passkeyDelete, passkeyRegisterBegin, passkeyRegisterFinish, passkeys } from '@/api/auth'
import { apiErrorMessage } from '@/api/client'
import type { Passkey } from '@/api/types'
import * as webauthn from '@/api/webauthn'
import BaseModal from '@/components/BaseModal.vue'
import FormField from '@/components/FormField.vue'
import { formatDate } from '@/composables/formatDate'
import { useAuthStore } from '@/stores/auth'
import { useNotificationStore } from '@/stores/notification'

const auth = useAuthStore()
const toast = useNotificationStore()
const keys = ref<Passkey[]>([])
const offered = ref(true)
const refusal = ref('')
const usable = webauthn.supported()
const dialog = ref<{
  mode: 'add' | 'remove'
  target?: Passkey
  name: string
  password: string
} | null>(null)
const busy = ref(false)

async function load() {
  try {
    keys.value = await passkeys()
    offered.value = true
  } catch (err) {
    const status = (err as { response?: { status?: number } }).response?.status
    if (status === 403) {
      offered.value = false
      refusal.value = apiErrorMessage(err)
      return
    }
    toast.error(apiErrorMessage(err))
  }
}

async function submit() {
  const d = dialog.value
  if (!d || !d.password) return
  busy.value = true
  try {
    if (d.mode === 'add') {
      // The password is checked BEFORE the browser prompt.
      const begin = await passkeyRegisterBegin(d.password)
      const credential = await webauthn.createCredential(begin)
      await passkeyRegisterFinish(d.name.trim() || 'Passkey', credential)
      toast.success('Passkey added')
    } else if (d.target) {
      await passkeyDelete(d.target.id, d.password)
      toast.success('Passkey removed')
    }
    dialog.value = null
    await load()
    await auth.refresh()
  } catch (err) {
    const fromServer = (err as { response?: unknown }).response
    toast.error(
      fromServer
        ? apiErrorMessage(err)
        : webauthn.ceremonyErrorMessage(err, 'The passkey could not be saved'),
    )
  } finally {
    busy.value = false
  }
}

void load()
</script>

<template>
  <div class="card">
    <div class="card-header">
      <h2>Passkeys</h2>
      <button
        v-if="offered && usable"
        class="btn btn-primary btn-sm"
        @click="dialog = { mode: 'add', name: '', password: '' }"
      >
        Add a passkey
      </button>
    </div>
    <div class="card-body">
      <p v-if="!offered" class="text-sm text-muted">{{ refusal }}</p>
      <p v-else-if="!usable" class="text-sm text-muted">This browser cannot use passkeys.</p>
      <p v-else-if="!keys.length" class="text-sm text-muted">
        A passkey signs you in with your fingerprint, face or device PIN, and cannot be phished the
        way a password and a code can.
      </p>
      <table v-else>
        <thead>
          <tr>
            <th>Name</th>
            <th>Added</th>
            <th>Last used</th>
            <th></th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="k in keys" :key="k.id">
            <td>
              <strong>{{ k.name }}</strong>
            </td>
            <td>{{ formatDate(k.created_at) }}</td>
            <td>{{ k.last_used_at ? formatDate(k.last_used_at) : 'Never' }}</td>
            <td class="text-right">
              <button
                class="btn btn-danger btn-sm"
                :disabled="busy"
                @click="dialog = { mode: 'remove', target: k, name: '', password: '' }"
              >
                Remove
              </button>
            </td>
          </tr>
        </tbody>
      </table>
    </div>

    <BaseModal
      v-if="dialog"
      :title="dialog.mode === 'add' ? 'Add a passkey' : 'Remove a passkey'"
      form
      @submit="submit"
      @close="dialog = null"
    >
      <FormField
        v-if="dialog.mode === 'add'"
        label="Name"
        for="passkey-name"
        hint="So you can tell it apart later"
      >
        <input
          id="passkey-name"
          v-model="dialog.name"
          class="form-input"
          placeholder="MacBook Touch ID"
          maxlength="60"
        />
      </FormField>
      <p v-else class="text-sm mb-3">
        <strong>{{ dialog.target?.name }}</strong> will no longer sign in.
      </p>
      <FormField label="Your password" for="passkey-password" hint="Confirms it is you">
        <input
          id="passkey-password"
          v-model="dialog.password"
          type="password"
          class="form-input"
          autocomplete="current-password"
          required
        />
      </FormField>
      <template #footer>
        <button type="button" class="btn btn-secondary" @click="dialog = null">Cancel</button>
        <button
          type="submit"
          class="btn"
          :class="dialog.mode === 'add' ? 'btn-primary' : 'btn-danger'"
          :disabled="busy || !dialog.password"
        >
          {{ dialog.mode === 'add' ? 'Continue' : 'Remove' }}
        </button>
      </template>
    </BaseModal>
  </div>
</template>
