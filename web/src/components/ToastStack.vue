<script setup lang="ts">
// Every transient message the console raises, in one corner.
//
// Its own component rather than markup in App.vue: the root is where
// the application is assembled, and a list of toasts with its own
// classes and click handling is a piece of UI, not assembly. Mounted
// once, near the root, because the store it reads is global and two
// stacks would render every message twice.
import { useNotificationStore } from '../stores/notification'

const notify = useNotificationStore()
</script>

<template>
  <!-- Teleported, like the confirm dialog: a toast belongs to the
       viewport rather than to whatever happened to be on screen when
       it was raised, and a stacking context somewhere up the tree
       would otherwise be able to trap it. -->
  <Teleport to="body">
    <div class="toast-container">
      <button
        v-for="t in notify.toasts"
        :key="t.key"
        type="button"
        class="toast"
        :class="`toast-${t.kind}`"
        @click="notify.dismiss(t.key)"
      >
        {{ t.text }}
      </button>
    </div>
  </Teleport>
</template>
