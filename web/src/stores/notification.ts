import { defineStore } from 'pinia'
import { ref } from 'vue'

/** How loud a toast is, which decides its colour and how long it sits. */
export type ToastKind = 'success' | 'error' | 'info'

/** One message on screen. `key` is ours, not anything from the server. */
export interface Toast {
  key: number
  kind: ToastKind
  text: string
}

// An error outlasts the others because it is the one somebody has to
// read and possibly act on - a success is a receipt for something they
// just did and already expected.
const LINGER: Record<ToastKind, number> = {
  success: 4000,
  error: 6000,
  info: 4000,
}

/**
 * The toast stack.
 *
 * A store rather than a composable because the messages are raised
 * from anywhere - views, api interceptors, the session watcher - and
 * rendered in exactly one place, ToastStack near the root. There is no
 * component tree relationship between those two ends to pass anything
 * through.
 */
export const useNotificationStore = defineStore('notification', () => {
  const toasts = ref<Toast[]>([])

  // Timers by toast key, so dismissing early cancels the pending
  // removal rather than leaving it to fire into an empty list.
  const timers = new Map<number, ReturnType<typeof setTimeout>>()
  let counter = 0

  /** Remove one message, whether it was clicked away or timed out. */
  function dismiss(key: number) {
    const timer = timers.get(key)
    if (timer !== undefined) {
      clearTimeout(timer)
      timers.delete(key)
    }

    toasts.value = toasts.value.filter((t) => t.key !== key)
  }

  /**
   * Raise a message. `linger` of 0 keeps it until it is clicked, which
   * nothing does today but is the honest meaning of "no timeout"
   * should something need it.
   */
  function show(text: string, kind: ToastKind = 'info', linger = LINGER[kind]) {
    const key = counter++
    toasts.value = [...toasts.value, { key, kind, text }]
    if (linger > 0) {
      timers.set(
        key,
        setTimeout(() => dismiss(key), linger),
      )
    }
  }

  return {
    toasts,
    show,
    dismiss,
    success: (text: string) => show(text, 'success'),
    error: (text: string) => show(text, 'error'),
    info: (text: string) => show(text, 'info'),
  }
})
