// Rules that hide entities from the graph: a property, how to compare
// it and a value. "label", "type" and "flag" are offered beside the
// properties, custom fields included.
import type { GraphNode } from '@/api/types'

export type Operator =
  'is' | 'not' | 'contains' | 'lacks' | 'starts' | 'ends' | 'more' | 'less' | 'set' | 'empty'

export interface Rule {
  field: string
  op: Operator
  value: string
}

export const OPERATORS: { value: Operator; label: string }[] = [
  { value: 'is', label: 'is' },
  { value: 'not', label: 'is not' },
  { value: 'contains', label: 'contains' },
  { value: 'lacks', label: 'does not contain' },
  { value: 'starts', label: 'starts with' },
  { value: 'ends', label: 'ends with' },
  { value: 'more', label: 'is more than' },
  { value: 'less', label: 'is less than' },
  { value: 'set', label: 'is set' },
  { value: 'empty', label: 'is empty' },
]

// needsValue is false for the operators that only ask whether there is
// a value at all.
export function needsValue(op: Operator): boolean {
  return op !== 'set' && op !== 'empty'
}

function read(n: GraphNode, field: string): string {
  if (field === 'label') return n.label
  if (field === 'type') return n.type
  if (field === 'flag') return n.flag ?? ''
  const v = n.properties?.[field]
  if (v === null || v === undefined) return ''
  return Array.isArray(v) ? v.map(String).join(', ') : String(v)
}

export function matches(n: GraphNode, r: Rule): boolean {
  const have = read(n, r.field).toLowerCase()
  const want = r.value.trim().toLowerCase()
  if (r.op === 'set') return have !== ''
  if (r.op === 'empty') return have === ''
  if (!want) return true
  switch (r.op) {
    case 'is':
      return have === want
    case 'not':
      return have !== want
    case 'contains':
      return have.includes(want)
    case 'lacks':
      return !have.includes(want)
    case 'starts':
      return have.startsWith(want)
    case 'ends':
      return have.endsWith(want)
    case 'more':
    case 'less': {
      // Numbers compare as numbers, anything else as text, which orders
      // YYYY-MM-DD dates too. An entity without the value does not match.
      if (have === '') return false
      const a = Number(have)
      const b = Number(want)
      const c = Number.isFinite(a) && Number.isFinite(b) ? a - b : have.localeCompare(want)
      return r.op === 'more' ? c > 0 : c < 0
    }
  }
}

// hiddenBy lists the entities the rules leave out.
export function hiddenBy(nodes: GraphNode[], rules: Rule[]): Set<string> {
  const active = rules.filter((r) => r.value.trim() || !needsValue(r.op))
  const out = new Set<string>()
  if (!active.length) return out
  for (const n of nodes) if (!active.every((r) => matches(n, r))) out.add(n.id)
  return out
}

// fields lists what the rules can compare: label, type and every
// scalar property the sketch uses.
export function fields(nodes: GraphNode[]): string[] {
  const keys = new Set<string>()
  for (const n of nodes)
    for (const [k, v] of Object.entries(n.properties ?? {}))
      if (v !== null && typeof v !== 'object') keys.add(k)
  return ['label', 'type', 'flag', ...[...keys].sort()]
}
