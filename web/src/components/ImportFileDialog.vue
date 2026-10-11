<script setup lang="ts">
// Imports an export file: the file and, for an encrypted one, its
// passphrase. The caller hands in what to do with them and gets the
// answer back.
import { ref } from 'vue'
import { apiErrorMessage } from '@/api/client'
import BaseModal from '@/components/BaseModal.vue'
import FormField from '@/components/FormField.vue'
import { useNotificationStore } from '@/stores/notification'

const props = defineProps<{
  title: string
  what: string
  importer: (file: File, passphrase: string) => Promise<unknown>
}>()
const emit = defineEmits<{ (e: 'close'): void; (e: 'done', result: unknown): void }>()

const toast = useNotificationStore()
const file = ref<File | null>(null)
const passphrase = ref('')
const busy = ref(false)

function pick(ev: Event) {
  file.value = (ev.target as HTMLInputElement).files?.[0] ?? null
}

async function run() {
  if (!file.value) return
  busy.value = true
  try {
    emit('done', await props.importer(file.value, passphrase.value))
  } catch (err) {
    toast.error(apiErrorMessage(err))
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <BaseModal :title="title" size="modal-w480" form @close="emit('close')" @submit="run">
    <p class="text-secondary mb-3">{{ what }}</p>
    <FormField label="File" required>
      <input type="file" accept=".json,.gosint,application/json" required @change="pick" />
    </FormField>
    <FormField label="Passphrase" hint="Only for an encrypted export">
      <input v-model="passphrase" type="password" class="form-input" autocomplete="off" />
    </FormField>
    <template #footer>
      <button type="button" class="btn btn-secondary" @click="emit('close')">Cancel</button>
      <button class="btn btn-primary" :disabled="!file || busy">Import</button>
    </template>
  </BaseModal>
</template>
