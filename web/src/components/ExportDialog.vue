<script setup lang="ts">
// Downloads an export, sealed under a passphrase when one is typed.
// The caller hands in what to download, the dialog asks for the
// passphrase and saves the file.
import { ref } from 'vue'
import { apiErrorMessage } from '@/api/client'
import type { Download } from '@/api/sketches'
import BaseModal from '@/components/BaseModal.vue'
import FormField from '@/components/FormField.vue'
import { saveBlob } from '@/composables/download'
import { useNotificationStore } from '@/stores/notification'

const props = defineProps<{
  title: string
  what: string
  exporter: (passphrase: string) => Promise<Download>
}>()
const emit = defineEmits<{ (e: 'close'): void }>()

const toast = useNotificationStore()
const passphrase = ref('')
const again = ref('')
const busy = ref(false)

async function run() {
  if (passphrase.value && passphrase.value !== again.value) {
    toast.error('The two passphrases differ')
    return
  }
  busy.value = true
  try {
    const d = await props.exporter(passphrase.value)
    saveBlob(d.blob, d.name)
    emit('close')
  } catch (err) {
    toast.error(apiErrorMessage(err))
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <BaseModal :title="title" size="modal-w480" form @close="emit('close')" @submit="run">
    <p class="text-secondary mb-3">
      {{ what }} downloads as one file. With a passphrase the file is encrypted and only opens with
      it, without one it is plain JSON.
    </p>
    <FormField label="Passphrase" hint="Leave empty for a plain file">
      <input v-model="passphrase" type="password" class="form-input" autocomplete="off" />
    </FormField>
    <FormField v-if="passphrase" label="Repeat the passphrase">
      <input v-model="again" type="password" class="form-input" autocomplete="off" />
    </FormField>
    <template #footer>
      <button type="button" class="btn btn-secondary" @click="emit('close')">Cancel</button>
      <button class="btn btn-primary" :disabled="busy">
        {{ passphrase ? 'Download encrypted' : 'Download' }}
      </button>
    </template>
  </BaseModal>
</template>
