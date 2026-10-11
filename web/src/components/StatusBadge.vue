<script setup lang="ts">
// A status pill. The colour comes from composables/statusBadge, which is
// the one place that knows what a status means.
//
// The label defaults to the status itself, which is what every call site
// rendered. Pass one where the wire value is not what a reader should see.
import { computed } from 'vue'
import { statusBadgeClass, type StatusScope } from '../composables/statusBadge'

const props = withDefaults(
  defineProps<{
    status: string
    scope: StatusScope
    label?: string
  }>(),
  { label: '' },
)

const cls = computed(() => statusBadgeClass(props.status, props.scope))
</script>

<template>
  <span :class="cls">{{ label || status }}</span>
</template>
