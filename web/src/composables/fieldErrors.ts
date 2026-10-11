// The server's field errors, put back where they belong.
//
// `internal/core/validation` answers with
// `{"error": "...", "fields": [{"field", "rule", "message"}]}`, and a
// refused field belongs next to the input that caused it, not in one
// run-on toast at the top right of the screen.
//
// `field` is the JSON name the request actually sent - validation
// registers a tag-name function for exactly that - so a form keyed by
// the same names needs no mapping table.
//
// A FormField names its key with `field="..."` and registers it here
// while it is mounted. That registry is what lets capture() tell a
// message it put on screen from one that has nowhere to go: a key no
// mounted FormField renders is left for the toast.

import { type InjectionKey, type Ref, inject, provide, ref } from 'vue'

export interface FieldError {
  field: string
  rule: string
  message: string
}

interface ErrorBody {
  response?: { data?: { error?: string; fields?: FieldError[] } }
}

export interface FieldErrorContext {
  errors: Ref<Record<string, string>>
  register: (field: string) => void
  unregister: (field: string) => void
}

const contextKey: InjectionKey<FieldErrorContext> = Symbol('fieldErrors')

// The nearest form's errors, for a FormField given `field`.
export function injectFieldErrors(): FieldErrorContext | undefined {
  return inject(contextKey, undefined)
}

// Splits a refusal into what the mounted fields can show and what they
// cannot.
function placeFieldErrors(
  fields: FieldError[],
  rendered: (field: string) => boolean,
): { placed: Record<string, string>; rest: string[] } {
  const placed: Record<string, string> = {}
  const rest: string[] = []
  for (const fe of fields) {
    if (!fe?.message) continue
    if (fe.field && rendered(fe.field)) {
      placed[fe.field] = fe.message
    } else {
      rest.push(fe.message)
    }
  }

  return { placed, rest }
}

export function useFieldErrors() {
  const errors = ref<Record<string, string>>({})
  const mounted = new Map<string, number>()

  function register(field: string) {
    mounted.set(field, (mounted.get(field) ?? 0) + 1)
  }

  function unregister(field: string) {
    const n = (mounted.get(field) ?? 0) - 1
    if (n > 0) {
      mounted.set(field, n)
    } else {
      mounted.delete(field)
    }
  }

  provide(contextKey, { errors, register, unregister })

  // True when every refused field is on screen, which is the caller's
  // signal not to raise a toast as well - saying it twice reads as two
  // problems.
  //
  // False when anything is left: a field this form does not render, or
  // an entry with no `field` (a body that did not decode). The response's
  // summary is then narrowed to those leftovers, so the caller's toast
  // says only what is not already under an input.
  function capture(err: unknown): boolean {
    const data = (err as ErrorBody)?.response?.data
    const fields = data?.fields
    if (!Array.isArray(fields)) return false

    const { placed, rest } = placeFieldErrors(fields, (f) => mounted.has(f))
    errors.value = placed
    if (rest.length === 0) return Object.keys(placed).length > 0

    if (data && Object.keys(placed).length > 0) data.error = rest.join('. ')

    return false
  }

  // Clear one field as it is edited, or all of them when a request is
  // about to be made. Errors from the previous attempt outliving the
  // next one is how a form ends up refusing a value it already accepted.
  function clear(field?: string) {
    if (!field) {
      errors.value = {}

      return
    }
    if (errors.value[field]) delete errors.value[field]
  }

  return { capture, clear }
}
