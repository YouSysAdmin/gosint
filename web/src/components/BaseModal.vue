<script lang="ts">
// ESCAPE BELONGS TO THE TOP DIALOG, and only to it.
//
// Every open modal listens on `document`, so without this one press
// would reach all of them and dismissing a confirmation would take the
// dialog underneath it too.
//
// A stack in mount order, so the newest wins. It cannot be a z-index or
// a DOM query: two dialogs from different components sit in different
// places in the tree and share one overlay class.
//
// IT HAS TO BE THIS BLOCK, not the one below. A `const` at the top of
// <script setup> is compiled INTO setup(), so every dialog would get a
// stack of its own holding only itself - indistinguishable from no check
// at all.
const openModals: symbol[] = []
</script>

<script setup lang="ts">
// Every dialog in the console is this component.
//
// Left to each view, closing one becomes three different things. A bare
// @click.self closes the dialog when a drag that started inside it
// happens to end on the overlay, so selecting text in a field and
// releasing past the edge throws the form away. Escape usually ends up
// unhandled.
//
// Mount it behind v-if, which is what every call site already does with
// its own showX flag. That matters: the keydown listener then lives
// exactly as long as the open dialog. Registering it when the VIEW
// mounts instead means a page with three dialogs arms three Escape
// handlers that fire whether anything is open or not.
import { onMounted, onUnmounted, ref } from 'vue'

const props = withDefaults(
  defineProps<{
    // Heading text. Use the header slot instead when it is not a plain
    // string.
    title?: string
    // Extra class on the box, from the width scale in the stylesheet:
    // modal-w440 through modal-w900.
    size?: string
    // Wrap body and footer in a form, so a footer submit button and the
    // Enter key both reach @submit. Twenty of these dialogs are forms.
    form?: boolean
    // Neither the overlay nor Escape closes it - only whatever the
    // footer offers. For the dialogs that show a secret exactly once: a
    // stray Escape there costs the reader the only copy of a token.
    persistent?: boolean
  }>(),
  { title: '', size: '', form: false, persistent: false },
)

const emit = defineEmits<{
  (e: 'close'): void
  (e: 'submit'): void
}>()

// A click that STARTS on the overlay and ENDS on the overlay is a click
// outside the box. Anything else is a drag, and @click.self cannot tell
// the two apart because it only ever sees the release.
const startedOnOverlay = ref(false)

function watchClickStart(event: MouseEvent) {
  startedOnOverlay.value = event.target === event.currentTarget
}

function confirmClickEnd(event: MouseEvent) {
  if (props.persistent) return

  if (startedOnOverlay.value && event.target === event.currentTarget) {
    emit('close')
  }
  startedOnOverlay.value = false
}

const id = Symbol('modal')

function onKeydown(event: KeyboardEvent) {
  if (event.key !== 'Escape' || props.persistent) return
  // Not mine to answer. A persistent dialog on top absorbs the press
  // rather than passing it down, which is the point of it being on top.
  if (openModals[openModals.length - 1] !== id) return

  emit('close')
}

onMounted(() => {
  openModals.push(id)
  document.addEventListener('keydown', onKeydown)
})

onUnmounted(() => {
  const at = openModals.indexOf(id)
  if (at !== -1) openModals.splice(at, 1)
  document.removeEventListener('keydown', onKeydown)
})
</script>

<template>
  <div class="modal-overlay" @mousedown="watchClickStart" @mouseup="confirmClickEnd">
    <div class="modal" :class="size" role="dialog" aria-modal="true" @mousedown.stop @mouseup.stop>
      <div class="modal-header">
        <slot name="header"
          ><h3>{{ title }}</h3></slot
        >
      </div>

      <!-- The two branches carry the same two rows on purpose. Wrapping
           them in one element that is a form or a div would put an extra
           div around the body of every non-form dialog. -->
      <form v-if="form" @submit.prevent="emit('submit')">
        <div class="modal-body"><slot /></div>
        <div v-if="$slots.footer" class="modal-footer"><slot name="footer" /></div>
      </form>
      <template v-else>
        <div class="modal-body"><slot /></div>
        <div v-if="$slots.footer" class="modal-footer"><slot name="footer" /></div>
      </template>
    </div>
  </div>
</template>
