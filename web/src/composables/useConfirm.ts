// The console's one confirmation prompt.
//
// A destructive action asks before it runs, and it asks in the same
// shape everywhere - which is only true if there is one implementation
// and one dialog. `window.confirm` is the alternative and it is not
// one: it cannot carry a variant, it blocks the event loop, and
// browsers increasingly refuse it outright in cross-origin frames.
//
// One dialog serves the whole application: ConfirmDialog is mounted
// once, near the root, and every caller drives that single instance
// through this module's state. So the state lives at MODULE level, not
// inside the composable function - a per-caller copy would mean the
// prompt a view opened was watched by nobody.
import { computed, shallowRef } from 'vue'

/** How loud the prompt is, which decides its icon and button colour. */
export type ConfirmVariant = 'danger' | 'warning' | 'info'

/** What a caller asks. Only the question itself is required. */
export interface ConfirmRequest {
  title: string
  message: string
  confirmText?: string
  cancelText?: string
  variant?: ConfirmVariant
}

/**
 * An open prompt: what to show, and how to answer it.
 *
 * The two are ONE object rather than a visible flag beside a loose
 * resolver, so "the dialog is open" and "somebody is waiting for an
 * answer" cannot disagree. A dropped resolver is an await that never
 * returns, and the caller is usually mid-way through a delete handler
 * when it happens.
 */
interface OpenPrompt {
  shown: Required<ConfirmRequest>
  answer: (confirmed: boolean) => void
}

const prompt = shallowRef<OpenPrompt | null>(null)

const fallback = {
  confirmText: 'Confirm',
  cancelText: 'Cancel',
  variant: 'info' as ConfirmVariant,
}

/**
 * useConfirm gives callers `confirm`, and the dialog component the
 * three things it needs to render and answer.
 *
 * Views destructure `{ confirm }` and await it, ConfirmDialog takes
 * `open`, `shown`, `accept` and `dismiss`.
 */
export function useConfirm() {
  /**
   * Ask, and resolve true only if the person actually confirmed.
   * Dismissing - the Cancel button, Escape, a click outside - resolves
   * false rather than rejecting, because refusing is an ordinary
   * answer to a question and not an error the caller should have to
   * catch.
   */
  function confirm(request: ConfirmRequest): Promise<boolean> {
    // A second prompt while one is open answers the first as declined.
    // Nothing in the console opens two deliberately, so this is the
    // safety net for a stray double-click: the earlier await resolves
    // false and unwinds instead of hanging forever behind the dialog
    // that replaced it.
    prompt.value?.answer(false)

    return new Promise<boolean>((resolve) => {
      prompt.value = {
        shown: { ...fallback, ...request },
        answer: (confirmed) => {
          prompt.value = null
          resolve(confirmed)
        },
      }
    })
  }

  return {
    open: computed(() => prompt.value !== null),
    shown: computed(() => prompt.value?.shown ?? null),
    accept: () => prompt.value?.answer(true),
    dismiss: () => prompt.value?.answer(false),
    confirm,
  }
}
