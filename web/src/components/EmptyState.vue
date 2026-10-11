<script setup lang="ts">
// The "there is nothing here yet" block.
//
// Text goes in the props for the ordinary case. The default slot is for
// the handful that need markup - a link into another page, a second
// paragraph that depends on a filter being set.
//
// The heading is optional because a dozen of these have none: inside a
// card that already carries the title, a second one repeats it.
withDefaults(
  defineProps<{
    title?: string
    text?: string
  }>(),
  { title: '', text: '' },
)
</script>

<template>
  <div class="empty-state">
    <h3 v-if="title">{{ title }}</h3>
    <slot>
      <p v-if="text">{{ text }}</p>
    </slot>
  </div>
</template>
