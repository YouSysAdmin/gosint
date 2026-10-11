<script setup lang="ts">
// The recovery codes, shown once. The server keeps hashes, so this is
// the only time anybody can read them, which is why the dialog is
// persistent: a stray Escape would cost the reader the only copy.
import BaseModal from '@/components/BaseModal.vue'
import CopyButton from '@/components/CopyButton.vue'

const props = defineProps<{ codes: string[] }>()
defineEmits<{ (e: 'close'): void }>()

const joined = () => props.codes.join('\n')
</script>

<template>
  <BaseModal title="Your recovery codes" persistent>
    <p class="notice notice-warning text-sm mb-3">
      Each code signs you in once in place of your authenticator app. They are stored hashed, so
      this is the only time they can be read. Keep them somewhere that is not the phone.
    </p>
    <div class="codes code-font">
      <span v-for="code in codes" :key="code">{{ code }}</span>
    </div>
    <CopyButton class="mt-3" :value="joined()" label="Copy the codes" />
    <template #footer>
      <button class="btn btn-primary" @click="$emit('close')">I saved them</button>
    </template>
  </BaseModal>
</template>

<style scoped>
.codes {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 6px 24px;
  padding: 12px;
  border-radius: var(--radius-sm);
  background: var(--bg-tertiary);
  font-variant-numeric: tabular-nums;
  user-select: all;
}
</style>
