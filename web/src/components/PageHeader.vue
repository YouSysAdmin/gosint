<script setup lang="ts">
// The title row of a page, and the group its controls live in.
//
// `.page-header` is space-between and expects exactly two children, so a
// control added as a third one lands in the middle of the page - which
// is how a Refresh button ends up between the title and the button
// beside it. Rendering the group here means a slotted control cannot be
// anywhere else.
//
// `subtitle` is a muted line under the heading. The title slot is for
// a page drawing its own heading, and it has to be one element, or
// the count is three again.
withDefaults(defineProps<{ title?: string; subtitle?: string }>(), { title: '', subtitle: '' })
</script>

<template>
  <div class="page-header">
    <!-- The title side is wrapped here, so a page that puts a subtitle
         under its heading still counts as ONE child. Slotting the two
         straight in is how this rule gets broken by the component that
         exists to keep it. -->
    <div>
      <slot name="title">
        <h1>{{ title }}</h1>
      </slot>
      <p v-if="subtitle" class="header-sub">{{ subtitle }}</p>
    </div>
    <div v-if="$slots.default" class="page-actions"><slot /></div>
  </div>
</template>
