// Where an entity or a relationship came from, as the server records
// it: the scan and transform that found it and the input it was found
// from, or the person who added it and how. One sentence for the panel.
import { formatDate } from '@/composables/formatDate'

export interface Origin {
  scan?: string
  transform?: string
  step?: string
  from?: string
  from_id?: string
  user?: string
  name?: string
  via?: string
  source?: string
  at?: string
}

const sources: Record<string, string> = {
  nuclei: 'a nuclei report',
  nmap: 'an nmap report',
  file: 'an export file',
  table: 'a table (CSV or XLSX)',
}

export function originOf(holder: Record<string, unknown> | null | undefined): Origin | null {
  const o = holder?.origin
  return o && typeof o === 'object' ? (o as Origin) : null
}

export function seenOf(holder: Record<string, unknown> | null | undefined): Origin | null {
  const o = holder?.seen
  return o && typeof o === 'object' ? (o as Origin) : null
}

// describeOrigin reads as "found by domain_to_subdomains from
// example.com", "added by Jane", "imported from a nuclei report by
// Jane", "added from the assistant by Jane", the date kept apart.
export function describeOrigin(o: Origin): string {
  const by = o.name ? ` by ${o.name}` : ''
  switch (o.via) {
    case 'transform':
    case 'flow':
      return `found by ${o.transform ?? 'a transform'}${o.from ? ` from ${o.from}` : ''}`
    case 'import':
      return `imported from ${sources[o.source ?? ''] ?? 'an import'}${by}`
    case 'chat':
      return `added from the assistant${by}`
    case 'hand':
      return `added${by}`
    default:
      return o.transform ? `found by ${o.transform}` : 'unknown'
  }
}

export function originDate(o: Origin): string {
  return formatDate(o.at, '')
}
