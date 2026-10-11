// Saving a file the browser way: a blob behind an object URL and a
// click on a link named after it.

export function saveBlob(blob: Blob, name: string) {
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = name
  a.click()
  URL.revokeObjectURL(url)
}

export function saveJSON(data: unknown, name: string) {
  saveBlob(new Blob([JSON.stringify(data, null, 2)], { type: 'application/json' }), name)
}

// fileNameOf reads the name a Content-Disposition header carries.
export function fileNameOf(header: string | undefined, fallback: string): string {
  const m = header?.match(/filename\*?=(?:UTF-8'')?"?([^";]+)"?/i)
  if (!m) return fallback
  // A malformed percent sequence keeps the name as sent.
  try {
    return decodeURIComponent(m[1])
  } catch {
    return m[1]
  }
}
