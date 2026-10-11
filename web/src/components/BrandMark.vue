<script setup lang="ts">
// The gosint mark: a lowercase g drawn as a graph, a ring and one
// relationship, the lit entity in the graph's accent where the stem
// starts. Ink follows the text colour, so it sits on either theme.
withDefaults(defineProps<{ size?: number }>(), { size: 22 })
</script>

<template>
  <svg
    class="brand-mark"
    :width="size"
    :height="size"
    viewBox="0 0 64 64"
    role="img"
    aria-label="gosint"
  >
    <circle cx="29" cy="25" r="14" fill="none" stroke="currentColor" stroke-width="5" />
    <path
      d="M43 13V43Q43 55 31 55H22"
      fill="none"
      stroke="currentColor"
      stroke-width="5"
      stroke-linecap="round"
    />
    <circle cx="43" cy="12" r="6.5" fill="var(--graph-accent)" />
    <circle cx="18" cy="55" r="5.5" fill="currentColor" />
  </svg>
</template>

<style scoped>
.brand-mark {
  flex: none;
  display: block;
}
</style>
