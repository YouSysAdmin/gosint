<template>
  <div class="nf-page">
    <div class="nf-code">404</div>
    <p class="text-secondary">This page does not exist.</p>
    <router-link class="btn btn-primary" to="/">Back to investigations</router-link>
  </div>
</template>

<style scoped>
.nf-page {
  min-height: 100vh;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 12px;
  background: var(--bg-secondary);
}

.nf-code {
  font-size: 64px;
  font-weight: 700;
  color: var(--text-muted);
}
</style>
