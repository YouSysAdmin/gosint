// A form over an entity type's fields: which fields a person can fill
// in, what control each gets, and the conversion both ways between the
// form's strings and the properties the type expects.
import type { EntityType, TypeField } from '@/api/types'

export type FormValues = Record<string, string | boolean>

// editable leaves out what has no sensible input, transforms fill those.
// A ref is typed in as the referenced entity's value, which the type
// turns into the nested object. "any" is typed in as text.
export function editable(f: TypeField): boolean {
  return f.kind !== 'object' && !(f.kind === 'list' && f.elem === 'ref')
}

// formFields is the type's editable fields, the primary one first.
export function formFields(t: EntityType | undefined): TypeField[] {
  return (t?.fields ?? [])
    .filter(editable)
    .sort(
      (a, b) => Number(b.primary) - Number(a.primary) || Number(b.required) - Number(a.required),
    )
}

export function control(
  f: TypeField,
): 'select' | 'vaultkey' | 'checkbox' | 'number' | 'textarea' | 'text' {
  if (f.vault_key) return 'vaultkey'
  if (f.options?.length) return 'select'
  if (f.kind === 'bool') return 'checkbox'
  if (f.kind === 'int' || f.kind === 'float') return 'number'
  if (f.long || f.kind === 'list') return 'textarea'
  return 'text'
}

// A list is typed one value per line, so a value may hold a comma and
// a long one is read whole.
export function hint(f: TypeField): string {
  if (f.kind === 'list') return `${f.description}. One value per line`
  return f.description
}

// rows sizes a text area to what it holds, within bounds.
export function rows(v: string | boolean | undefined, min: number): number {
  const lines = String(v ?? '').split('\n').length
  return Math.min(10, Math.max(min, lines + 1))
}

function convert(f: TypeField, v: string | boolean): unknown {
  if (f.kind === 'bool') return Boolean(v)
  if (f.kind === 'int' || f.kind === 'float') return Number(v)
  if (f.kind === 'list')
    return String(v)
      .split('\n')
      .map((s) => s.trim())
      .filter(Boolean)
  return String(v).trim()
}

// refValue is the value a nested entity is keyed on: its first string
// property, which for every built-in type is the primary field.
function refValue(v: Record<string, unknown>): string {
  for (const key of ['value', 'domain', 'email', 'address', 'name', 'url', 'number']) {
    if (typeof v[key] === 'string' && v[key]) return v[key] as string
  }
  const first = Object.values(v).find((x) => typeof x === 'string' && x)
  return (first as string | undefined) ?? ''
}

// toProperties reads a form back into properties, leaving out what is
// empty.
export function toProperties(fields: TypeField[], values: FormValues): Record<string, unknown> {
  const out: Record<string, unknown> = {}
  for (const f of fields) {
    const v = values[f.name]
    if (v === undefined || v === '') continue
    out[f.name] = convert(f, v)
  }

  return out
}

// fromProperties fills a form from an entity's properties.
export function fromProperties(fields: TypeField[], props: Record<string, unknown>): FormValues {
  const out: FormValues = {}
  for (const f of fields) {
    const v = props[f.name]
    if (v === null || v === undefined) continue
    if (f.kind === 'bool') out[f.name] = Boolean(v)
    else if (Array.isArray(v)) out[f.name] = v.map(String).join('\n')
    else if (typeof v === 'object') {
      // A nested entity shows as its own value, the field it is keyed on.
      if (f.kind === 'ref') {
        const inner = refValue(v as Record<string, unknown>)
        if (inner) out[f.name] = inner
      }
    } else out[f.name] = String(v)
  }

  return out
}

// patch is what changed between two fillings of the form. A cleared
// field becomes null, which the server's merge stores as cleared. The
// two sides are compared as text, so a number typed back as it was is
// no change.
export function patch(
  fields: TypeField[],
  before: FormValues,
  after: FormValues,
): Record<string, unknown> {
  const out: Record<string, unknown> = {}
  for (const f of fields) {
    const was = before[f.name] ?? ''
    const now = after[f.name] ?? ''
    if (String(was) === String(now)) continue
    out[f.name] = now === '' ? null : convert(f, now)
  }

  return out
}
