<script setup lang="ts">
// The sketch's own settings: title, description and status.
import { ref } from 'vue'
import { apiErrorMessage } from '@/api/client'
import { updateSketch } from '@/api/sketches'
import type { Sketch } from '@/api/types'
import BaseModal from '@/components/BaseModal.vue'
import FormField from '@/components/FormField.vue'
import { useNotificationStore } from '@/stores/notification'

const props = defineProps<{ sketch: Sketch }>()
const emit = defineEmits<{ (e: 'close'): void; (e: 'saved', s: Sketch): void }>()

const toast = useNotificationStore()
const title = ref(props.sketch.title)
const description = ref(props.sketch.description)
const status = ref<'active' | 'archived'>(props.sketch.status)
const busy = ref(false)

async function save() {
  busy.value = true
  try {
    emit(
      'saved',
      await updateSketch(props.sketch.id, {
        title: title.value.trim(),
        description: description.value.trim(),
        status: status.value,
      }),
    )
    toast.success('Sketch saved')
  } catch (err) {
    toast.error(apiErrorMessage(err))
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <BaseModal title="Sketch settings" size="modal-w480" form @close="emit('close')" @submit="save">
    <FormField label="Title" required>
      <input v-model="title" class="form-input" required maxlength="200" autofocus />
    </FormField>
    <FormField label="Description">
      <textarea v-model="description" class="form-textarea" rows="3" />
    </FormField>
    <FormField label="Status" hint="An archived sketch is kept but sinks in the lists">
      <select v-model="status" class="form-select">
        <option value="active">Active</option>
        <option value="archived">Archived</option>
      </select>
    </FormField>
    <template #footer>
      <button type="button" class="btn btn-secondary" @click="emit('close')">Cancel</button>
      <button class="btn btn-primary" :disabled="busy || !title.trim()">Save</button>
    </template>
  </BaseModal>
</template>
