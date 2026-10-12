<script setup lang="ts">
// The routed page, plus the two surfaces raised from anywhere.
import ConfirmDialog from './components/ConfirmDialog.vue'
import ToastStack from './components/ToastStack.vue'
import { installDesktopLinks } from './composables/desktopLinks'
import { useThemeStore } from './stores/theme'

useThemeStore()
installDesktopLinks()
</script>

<template>
  <router-view />
  <ConfirmDialog />
  <ToastStack />
</template>
