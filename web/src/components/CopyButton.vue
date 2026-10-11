<script setup lang="ts">
// Copies a value to the clipboard and says so for a moment.
import { ref } from 'vue'

const props = defineProps<{ value: string; label?: string }>()
const copied = ref(false)

async function copy() {
  try {
    await navigator.clipboard.writeText(props.value)
    copied.value = true
    setTimeout(() => (copied.value = false), 1500)
  } catch {
    copied.value = false
  }
}
</script>

<template>
  <button type="button" class="btn btn-sm btn-secondary" @click="copy">
    {{ copied ? 'Copied' : (label ?? 'Copy') }}
  </button>
</template>
