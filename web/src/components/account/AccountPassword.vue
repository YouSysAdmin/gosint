<script setup lang="ts">
// Change the account password. The server signs every other session
// out on a change, so the sessions list is stale the moment this works.
import { ref } from 'vue'
import { changePassword } from '@/api/auth'
import { apiErrorMessage } from '@/api/client'
import BaseModal from '@/components/BaseModal.vue'
import FormField from '@/components/FormField.vue'
import { useNotificationStore } from '@/stores/notification'

const emit = defineEmits<{ (e: 'changed'): void }>()
const toast = useNotificationStore()
const form = ref<{ current: string; next: string } | null>(null)
const busy = ref(false)

async function submit() {
  if (!form.value || busy.value) return
  busy.value = true
  try {
    await changePassword(form.value.current, form.value.next)
    form.value = null
    toast.success('Password changed. Every other session was signed out.')
    emit('changed')
  } catch (err) {
    toast.error(apiErrorMessage(err))
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <div class="card">
    <div class="card-header"><h2>Password</h2></div>
    <div class="card-body">
      <p class="text-sm text-muted mb-3">
        Changing it signs out every other session. This one stays.
      </p>
      <button class="btn btn-secondary" @click="form = { current: '', next: '' }">
        Change password
      </button>
    </div>
    <BaseModal v-if="form" title="Change password" form @submit="submit" @close="form = null">
      <FormField label="Current password" for="current-password">
        <input
          id="current-password"
          v-model="form.current"
          type="password"
          class="form-input"
          autocomplete="current-password"
          required
        />
      </FormField>
      <FormField label="New password" for="new-password" hint="At least 8 characters">
        <input
          id="new-password"
          v-model="form.next"
          type="password"
          class="form-input"
          autocomplete="new-password"
          minlength="8"
          required
        />
      </FormField>
      <template #footer>
        <button type="button" class="btn btn-secondary" @click="form = null">Cancel</button>
        <button
          type="submit"
          class="btn btn-primary"
          :disabled="busy || !form.current || form.next.length < 8"
        >
          Change password
        </button>
      </template>
    </BaseModal>
  </div>
</template>
