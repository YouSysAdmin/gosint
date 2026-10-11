<script setup lang="ts">
// The frame of every page shown before sign-in: the mark and one card
// in the middle.
import BrandMark from '@/components/BrandMark.vue'

defineProps<{ title: string }>()
</script>

<template>
  <div class="auth">
    <div class="wordmark"><BrandMark :size="34" />gosint</div>
    <div class="card auth-card">
      <h1>{{ title }}</h1>
      <slot />
    </div>
  </div>
</template>

<style scoped>
.auth {
  min-height: 100vh;
  display: flex;
  flex-direction: column;
  gap: 20px;
  align-items: center;
  justify-content: center;
  background: var(--bg-secondary);
  padding: 16px;
}

.wordmark {
  display: flex;
  align-items: center;
  gap: 10px;
  font-size: 24px;
  font-weight: 600;
  letter-spacing: -0.01em;
  color: var(--text-primary);
}

.auth-card {
  width: 100%;
  max-width: 380px;
  padding: 28px;
}

h1 {
  font-size: 20px;
  margin: 0 0 20px;
}
</style>
