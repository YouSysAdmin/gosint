// humanSize renders a byte count the way a person reads one.
export function humanSize(n?: number): string {
  if (!n || n <= 0) return '0 B'
  const units = ['B', 'KB', 'MB', 'GB']
  let v = n
  let i = 0
  while (v >= 1024 && i < units.length - 1) {
    v /= 1024
    i++
  }

  return `${i === 0 ? Math.round(v) : v.toFixed(1)} ${units[i]}`
}
