// Saving a file the browser way: a blob behind an object URL and a
// click on a link named after it. The desktop window asks where instead.
import { isDesktop, saveNative } from './desktop'

export function saveBlob(blob: Blob, name: string) {
  if (isDesktop()) {
    void saveNative(blob, name)
    return
  }
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = name
  a.click()
  URL.revokeObjectURL(url)
}

// blobOfDataURL decodes a base64 data URL, as a canvas exports, without
// a fetch the page's connect-src would refuse.
export function blobOfDataURL(data: string): Blob {
  const [head, body = ''] = data.split(',', 2)
  const type = head.match(/^data:([^;,]+)/)?.[1] ?? 'application/octet-stream'
  const bin = atob(body)
  const out = new Uint8Array(bin.length)
  for (let i = 0; i < bin.length; i++) out[i] = bin.charCodeAt(i)
  return new Blob([out], { type })
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
