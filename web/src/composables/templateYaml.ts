// Keeps a template's YAML in step with the Input and Output picked in
// the editor: the type of each block, the input's key, and a comment
// listing the fields of the type, the ones an input offers as
// {{placeholders}} and the ones an output can be mapped into. Edits are
// line based so the rest of the document, comments included, stays as
// it was written.

export type Block = 'input' | 'output'

// FIELDS_MARK starts the generated comment, so a later change replaces
// it instead of stacking another.
const FIELDS_MARK = '# fields:'
const WIDTH = 72

interface Span {
  start: number // the "input:" line
  end: number // first line after the block
  indent: string
}

// inline is the type an inline block names, "input: {type: ip, key: x}",
// which this editor reads but leaves as written.
function inline(lines: string[], block: Block): string | null {
  const l = lines.find((l) => new RegExp(`^${block}:\\s*\\S`).test(l))
  if (!l) return null
  return l.match(/type:\s*["']?([\w-]+)/)?.[1] ?? ''
}

function span(lines: string[], block: Block): Span | null {
  const start = lines.findIndex((l) => new RegExp(`^${block}:\\s*$`).test(l))
  if (start < 0) return null
  let end = start + 1
  while (end < lines.length && (lines[end].trim() === '' || /^\s/.test(lines[end]))) end++
  // Trailing blank lines belong to the gap before the next key.
  while (end > start + 1 && lines[end - 1].trim() === '') end--
  const child = lines.slice(start + 1, end).find((l) => l.trim() !== '')
  const indent = child?.match(/^\s+/)?.[0] ?? '  '
  return { start, end, indent }
}

function scalar(lines: string[], s: Span, key: string): string {
  for (const l of lines.slice(s.start + 1, s.end)) {
    const m = l.match(new RegExp(`^${s.indent}${key}:\\s*(.*?)\\s*$`))
    if (m) return m[1].replace(/^["']|["']$/g, '')
  }
  return ''
}

// read gives the input and output types the document names, "" where
// it names none.
export function read(source: string): { input: string; output: string; key: string } {
  const lines = source.split('\n')
  const i = span(lines, 'input')
  const o = span(lines, 'output')
  return {
    input: i ? scalar(lines, i, 'type') : (inline(lines, 'input') ?? ''),
    output: o ? scalar(lines, o, 'type') : (inline(lines, 'output') ?? ''),
    key: i ? scalar(lines, i, 'key') : '',
  }
}

// fieldsComment wraps the field names into comment lines.
function fieldsComment(indent: string, fields: string[], what: string): string[] {
  const out: string[] = []
  let line = `${indent}${FIELDS_MARK} ${what}`
  for (const f of fields) {
    const piece = (line.endsWith(what) ? ' ' : ', ') + f
    if (line.length + piece.length > WIDTH) {
      out.push(line + ',')
      line = `${indent}#   ${f}`
    } else line += piece
  }
  out.push(line)
  return out
}

// set rewrites one block: its type, the input's key, and the fields
// comment. A block the document lacks is added at its end.
export function set(
  source: string,
  block: Block,
  type: string,
  key: string,
  fields: string[],
): string {
  const lines = source.split('\n')
  if (inline(lines, block) !== null) return source
  let s = span(lines, block)
  if (!s) {
    while (lines.length && lines[lines.length - 1].trim() === '') lines.pop()
    lines.push(`${block}:`, `  type: ${type}`)
    s = { start: lines.length - 2, end: lines.length, indent: '  ' }
  }

  const body = lines.slice(s.start + 1, s.end)
  // Drop the old generated comment, its continuation lines too.
  const kept: string[] = []
  let inMark = false
  for (const l of body) {
    const t = l.trim()
    if (t.startsWith(FIELDS_MARK)) {
      inMark = true
      continue
    }
    if (inMark && t.startsWith('#   ')) continue
    inMark = false
    kept.push(l)
  }

  const put = (k: string, v: string, after: number) => {
    const at = kept.findIndex((l) => new RegExp(`^${s!.indent}${k}:`).test(l))
    if (at >= 0) kept[at] = `${s!.indent}${k}: ${v}`
    else kept.splice(after, 0, `${s!.indent}${k}: ${v}`)
  }
  put('type', type, 0)
  if (block === 'input' && key) put('key', key, 1)

  const what =
    block === 'input' ? 'use any as {{name}} in the request,' : 'the targets of response.map,'
  if (fields.length) {
    const at =
      Math.max(
        ...['type', 'key'].map((k) =>
          kept.findIndex((l) => new RegExp(`^${s!.indent}${k}:`).test(l)),
        ),
      ) + 1
    kept.splice(at, 0, ...fieldsComment(s.indent, fields, what))
  }

  return [...lines.slice(0, s.start + 1), ...kept, ...lines.slice(s.end)].join('\n')
}

// renameKey points the {{old}} placeholders at the new input key.
export function renameKey(source: string, from: string, to: string): string {
  if (!from || !to || from === to) return source
  return source.split(`{{${from}}}`).join(`{{${to}}}`)
}
