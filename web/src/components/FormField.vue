<script setup lang="ts">
// A labelled field: label, the control, and whatever guidance goes under
// it.
//
// Errors come from two places and they answer different questions.
//
// `field` names the server's. Every handler behind validation.Bind
// replies with `fields: [{field, rule, message}]` naming the json field
// it refused, and useFieldErrors turns that array back into per-field
// messages instead of one run-on toast. The field registers its key with
// the nearest useFieldErrors while mounted, which is how capture() knows
// the message has a place on screen. `error` is a message the view
// decides itself.
//
// The other source is the browser, handled here rather than in the views
// because the constraint is already on the control - `type=number`,
// `min`, `max`, `required`. Type letters into a number input and `value`
// comes back empty with `validity.badInput` set, so the model receives
// '' and the page sends a missing field or quietly turns it into 0, with
// nothing on screen to say the value was dropped.
//
// The native check reports on blur, never before the field has been
// touched, and clears as soon as the value is valid. A field that turns
// red while you are still typing the first character is worse than one
// that says nothing.
//
// The control stays in the slot rather than becoming props. These are
// inputs, selects, textareas, checkbox rows and two custom pickers, and
// a component taking a `type` would grow a branch for each.
import { computed, onBeforeUnmount, onMounted, ref, useId, watch } from 'vue'
import { injectFieldErrors } from '../composables/fieldErrors'

const props = withDefaults(
  defineProps<{
    // Plain text. Use the label slot when the heading carries markup -
    // twenty of them name a config key in <code>.
    label?: string
    // Ties the label to the control. Pass the same value as the
    // control's id.
    for?: string
    // Guidance under the control. Plain text - use the hint slot when it
    // carries markup, which three of them do.
    hint?: string
    // The json name the server refuses this field under.
    field?: string
    // A message the view decides itself.
    error?: string
    // Marks the plain-text label. A label given through the slot writes
    // its own, since it is markup by then.
    required?: boolean
    // Turn the browser's own check off, for a control whose constraints
    // are not what the reader is being asked about.
    native?: boolean
  }>(),
  { label: '', for: '', hint: '', field: '', error: '', required: false, native: true },
)

const root = ref<HTMLElement | null>(null)
const nativeError = ref('')

type Control = HTMLInputElement | HTMLSelectElement | HTMLTextAreaElement

function controls(): Control[] {
  if (!root.value) return []

  return Array.from(root.value.querySelectorAll<Control>('input, select, textarea'))
}

// The browser knows why it refused, and the reasons that matter here are
// the ones a person can act on. Its own `validationMessage` is used for
// anything else, because it is localized and we are not.
function messageFor(el: Control): string {
  const v = el.validity
  if (v.valid) return ''

  if (v.badInput) {
    return el.type === 'number' ? 'Enter a number' : 'This value is not valid'
  }

  if (v.rangeUnderflow) return `Must be ${(el as HTMLInputElement).min} or more`
  if (v.rangeOverflow) return `Must be ${(el as HTMLInputElement).max} or less`
  if (v.valueMissing) return 'Required'
  if (v.typeMismatch && el.type === 'email') return 'Enter an email address'
  if (v.typeMismatch && el.type === 'url') return 'Enter a URL'

  return el.validationMessage
}

function check() {
  if (!props.native) return
  for (const el of controls()) {
    const message = messageFor(el)
    if (message) {
      nativeError.value = message

      return
    }
  }
  nativeError.value = ''
}

// Blur reports, typing only ever clears. The second half is what keeps a
// corrected value from staying red until the field is left again.
function onBlur() {
  check()
}

function onInput() {
  if (nativeError.value) check()
}

// TYING THE LABEL TO ITS CONTROL, which is what `for` is for: clicking
// the label puts the caret in the field, and a screen reader reads the
// input as labelled.
//
// Done here rather than by an id in every view, because the id exists
// only to join two elements this component already owns both of.
//
// It never overrides. A `for` prop, a `for` already on the label, or an
// id already on the control all win - so the field that genuinely has
// two controls can still say which one it means.
const autoID = useId()

function tieLabelToControl() {
  if (props.for) return

  const label = root.value?.querySelector<HTMLLabelElement>('label.form-label')
  if (!label || label.htmlFor) return

  // A checkbox row writes its own <label> around the box, so there is
  // no form-label here and this does not run. The first control is the
  // right one everywhere else: a field with several is a field whose
  // heading names the group, and focusing its first entry is what
  // clicking that heading should do.
  const el = controls()[0]
  if (!el) return

  if (!el.id) el.id = autoID
  label.htmlFor = el.id
}

const form = injectFieldErrors()

// Registered only while mounted, so a field hidden behind v-if does not
// claim a message nobody can see.
watch(
  () => props.field,
  (next, prev) => {
    if (!form) return
    if (prev) form.unregister(prev)
    if (next) form.register(next)
  },
)

onMounted(() => {
  if (form && props.field) form.register(props.field)
  root.value?.addEventListener('focusout', onBlur)
  root.value?.addEventListener('input', onInput)
  tieLabelToControl()
})

onBeforeUnmount(() => {
  if (form && props.field) form.unregister(props.field)
  root.value?.removeEventListener('focusout', onBlur)
  root.value?.removeEventListener('input', onInput)
})

// The server's answer wins: it saw the whole request, and it is the one
// that refused to store anything.
const serverError = computed(() => (form && props.field ? form.errors.value[props.field] : ''))
const shown = computed(() => serverError.value || props.error || nativeError.value)
</script>

<template>
  <div ref="root" class="form-group">
    <!-- A trailing asterisk, because that is how the forms that mark a
         required field already do it. No new class for it: an unstyled
         one is a build failure here, and inventing a colour would make
         the same mark look like two different things across pages. -->
    <label v-if="label || $slots.label" class="form-label" :for="$props.for">
      <slot name="label">{{ label }}{{ required ? ' *' : '' }}</slot>
    </label>
    <slot />
    <!-- The error replaces the hint rather than stacking under it: two
         lines of guidance where one of them says the value is wrong
         reads as if both are still true.

         Which is why a hint belongs HERE and not in the default slot,
         where this component cannot reach it to replace it. The slot is
         for the hints that carry markup. -->
    <p v-if="shown" class="form-error">{{ shown }}</p>
    <p v-else-if="hint || $slots.hint" class="form-hint">
      <slot name="hint">{{ hint }}</slot>
    </p>
  </div>
</template>
