// The relationship labels transforms draw, offered wherever one is typed.
export const KNOWN_LABELS = [
  'RELATED_TO',
  'HAS_DOMAIN',
  'HAS_SUBDOMAIN',
  'RESOLVES_TO',
  'HAS_IP',
  'HAS_EMAIL',
  'HAS_PHONE',
  'HAS_USERNAME',
  'HAS_SOCIAL_ACCOUNT',
  'HAS_CREDENTIAL',
  'HAS_BANK_ACCOUNT',
  'HAS_EXECUTIVE',
  'HELD_BY',
  'SUBSIDIARY_OF',
  'HAS_FINDING',
  'CONFIRMED_AS',
  'HAS_VULNERABILITY',
  'REFERENCES',
  'FOUND_IN_BREACH',
  'LEAKED_WITH',
  'HAS_WEBSITE',
  'WORKS_FOR',
  'OWNS',
  'LOCATED_AT',
  'LIVES_AT',
  'REGISTERED_BY',
  'BELONGS_TO',
  'MEMBER_OF',
  'KNOWS',
]

// labelChoices puts the labels a sketch already uses first.
export function labelChoices(used: Iterable<string>): string[] {
  const seen = new Set(used)
  return [...seen, ...KNOWN_LABELS.filter((k) => !seen.has(k))]
}

// normalizeLabel turns "works for" into WORKS_FOR.
export function normalizeLabel(raw: string): string {
  return raw.trim().toUpperCase().replace(/\s+/g, '_')
}
