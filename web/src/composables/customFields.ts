// Custom fields are an entity's own properties its type does not
// declare: any name the investigator wants, a text, number, yes/no or
// date value. The store keeps unknown keys, the filters offer them.
import { kindLabel } from '@/composables/fieldKinds'

export type CustomKind = 'string' | 'number' | 'bool' | 'date'

export interface CustomRow {
  key: string
  value: string
  // was is the stored value, absent for a row added in this edit.
  was?: string
  kind: CustomKind
  error?: string
}

export const CUSTOM_KINDS = (['string', 'number', 'bool', 'date'] as CustomKind[]).map((value) => ({
  value,
  label: kindLabel(value),
}))

export const CUSTOM_KEY = /^[A-Za-z][A-Za-z0-9_ -]{0,63}$/

const ISO_DATE = /^\d{4}-\d{2}-\d{2}$/

// kindOf reads the kind a stored value has. A date is a text that reads
// as YYYY-MM-DD, the store keeps it as a string.
export function kindOf(v: unknown): CustomKind {
  if (typeof v === 'number') return 'number'
  if (typeof v === 'boolean') return 'bool'
  return typeof v === 'string' && ISO_DATE.test(v) ? 'date' : 'string'
}

// customOf is the custom rows of a set of properties.
export function customOf(p: Record<string, unknown>, declared: Set<string>): CustomRow[] {
  return Object.entries(p)
    .filter(
      ([k, v]) =>
        !declared.has(k) && v !== null && ['string', 'number', 'boolean'].includes(typeof v),
    )
    .map(([k, v]) => ({ key: k, value: String(v), was: String(v), kind: kindOf(v) }))
}

// setKind switches a new row's kind and drops a value the new input
// could not show.
export function setKind(r: CustomRow, kind: CustomKind) {
  r.kind = kind
  r.error = undefined
  if (kind === 'bool') r.value = r.value === 'false' ? 'false' : 'true'
  else if (kind === 'date' && !ISO_DATE.test(r.value)) r.value = ''
}

// typed turns a row's text into its kind, or a message saying why it
// cannot be one.
export function typed(r: CustomRow): { value: unknown } | { error: string } {
  const v = r.value.trim()
  if (r.kind === 'number') {
    const n = Number(v)
    if (v === '' || !Number.isFinite(n)) return { error: 'not a number' }
    return { value: n }
  }
  if (r.kind === 'bool') return { value: v === 'true' }
  if (r.kind === 'date' && !ISO_DATE.test(v)) return { error: 'not a date' }
  return { value: v }
}
