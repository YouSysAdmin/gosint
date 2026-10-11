<script setup lang="ts">
// The full-block spinner a page shows while its first load is in
// flight.
//
// Not for the QUIET refresh path: an automatic refresh deliberately
// leaves the current rows on screen (see useAutoRefresh), so a page that
// swaps its table for this every ten seconds is unreadable.
</script>

<template>
  <div class="loading-page">
    <div class="spinner"></div>
  </div>
</template>
