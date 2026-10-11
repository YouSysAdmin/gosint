<script setup lang="ts">
// The admin pages, as tabs under the page header.
import { useRoute } from 'vue-router'

const route = useRoute()
const tabs = [
  { to: '/admin/users', label: 'Accounts' },
  { to: '/admin/providers', label: 'Identity providers' },
  { to: '/admin/types', label: 'Entity types' },
  { to: '/admin/models', label: 'Models' },
  { to: '/admin/mail', label: 'Mail' },
  { to: '/admin/worker', label: 'Worker' },
  { to: '/admin/tools', label: 'Tools' },
  { to: '/admin/files', label: 'Files' },
  { to: '/admin/jobs', label: 'Maintenance' },
]
</script>

<template>
  <div class="tabs mb-3">
    <router-link
      v-for="t in tabs"
      :key="t.to"
      :to="t.to"
      class="tab"
      :class="{ active: route.path === t.to }"
    >
      {{ t.label }}
    </router-link>
  </div>
</template>
