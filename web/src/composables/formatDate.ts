// One date formatter for the whole console, so a missing date and a
// timestamp read the same on every page.
//
// The placeholder is a parameter because the two readings are both
// right - a column that may simply have no value wants "-", one that
// says whether something has ever happened wants "Never".
export function formatDate(value?: string | null, empty = '-'): string {
  if (!value) return empty
  return new Date(value).toLocaleString(undefined, clock24)
}

// 24-hour time, everywhere in the console.
//
// hourCycle, not hour12: false. They agree on every engine tested here,
// but only one of them is defined to produce 00-23. `hour12: false` is
// specified as selecting a 24-hour cycle without saying which, and for
// some locales engines have resolved it to h24 - the cycle that numbers
// hours 1-24 and renders five past midnight as "24:05". h23 cannot.
//
// The LOCALE stays the browser's. This changes the clock, not the date
// order: an operator in Kyiv keeps 11.08.2026 and one in New York keeps
// 8/11/2026, which is theirs to decide and not ours.
const clock24: Intl.DateTimeFormatOptions = { hourCycle: 'h23' }

// formatTimeParts is the same clock for a caller that wants its own
// components - a short "11 Aug, 14:05" rather than the full stamp. A
// view never calls toLocaleString itself, so the clock cannot drift
// between pages.
export function formatTimeParts(
  value: string | null | undefined,
  parts: Intl.DateTimeFormatOptions,
  empty = '-',
): string {
  if (!value) return empty
  return new Date(value).toLocaleString(undefined, { ...parts, ...clock24 })
}

/**
 * How long ago, in words. '' or undefined gives `empty`.
 *
 * PAST A WEEK it stops counting and gives the date. "just now" through
 * "6d ago" is an interval a reader holds in their head. Beyond that the
 * date is the more useful answer and the shorter one to read, where
 * "42d ago" is a number nobody converts.
 */
export function timeAgo(value?: string | null, empty = 'never'): string {
  if (!value) return empty

  const seconds = Math.max(0, (Date.now() - new Date(value).getTime()) / 1000)
  if (seconds < 90) return 'just now'
  if (seconds < 3600) return `${Math.round(seconds / 60)}m ago`
  if (seconds < 86400) return `${Math.round(seconds / 3600)}h ago`
  if (seconds < 604800) return `${Math.round(seconds / 86400)}d ago`

  return formatTimeParts(value, { day: 'numeric', month: 'short', year: 'numeric' })
}

/**
 * Whether a moment has passed. Missing is NOT past - an invitation with
 * no expiry never expires, and reading it as expired would hide a live
 * one.
 *
 * Here rather than in the one view that asks, because reading the clock
 * is what this file is for: a view that computes its own time is how the
 * console ended up with three different wordings of "5 minutes ago".
 */
export function isPast(value?: string | null): boolean {
  if (!value) return false

  return new Date(value).getTime() < Date.now()
}
