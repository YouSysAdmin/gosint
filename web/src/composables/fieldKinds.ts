// The names a person reads for a field's kind, the same in the type
// editor and in an entity's custom fields.
const labels: Record<string, string> = {
  string: 'Text',
  int: 'Whole number',
  float: 'Number',
  number: 'Number',
  bool: 'Yes/no',
  datetime: 'Date',
  date: 'Date',
  url: 'URL',
  email: 'Email',
  list: 'List',
}

export function kindLabel(kind: string): string {
  return labels[kind] ?? kind
}
