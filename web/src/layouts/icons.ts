// The console's icon set: 36 glyphs in one map.
//
// The glyphs are FEATHER ICONS, refitted to an 18px grid.
//
//   Feather - https://feathericons.com
//   https://github.com/feathericons/feather
//   MIT License, Copyright (c) 2013-2023 Cole Bemis
//
// Refitted rather than imported: Feather is drawn on a 24 viewBox at
// stroke 2, and this set is 18 at stroke 1.5. Some are the same shape
// scaled by exactly 0.75 (send, users). Others need their geometry
// adjusted, because a straight scale loses a shape at this size.
//
// Pasted rather than depended on: an icon font ships megabytes into
// the binary, where inline SVG for the glyphs actually used costs a few
// kilobytes.
//
// Adding one means matching the grid - 18x18 viewBox, 1.5 stroke,
// currentColor, round caps - or it reads as a different weight beside
// its neighbours.

// One glyph per line, keys quoted. Two guards parse this map by
// matching quoted keys, and prettier both reflows the SVG strings -
// leaving them unreadable as shapes - and strips quotes it considers
// unnecessary.
// prettier-ignore
export const ICONS: Record<string, string> = {
    'grid': '<svg width="18" height="18" viewBox="0 0 18 18" fill="none"><rect x="1.5" y="1.5" width="6" height="6" rx="1.5" stroke="currentColor" stroke-width="1.5"/><rect x="10.5" y="1.5" width="6" height="6" rx="1.5" stroke="currentColor" stroke-width="1.5"/><rect x="1.5" y="10.5" width="6" height="6" rx="1.5" stroke="currentColor" stroke-width="1.5"/><rect x="10.5" y="10.5" width="6" height="6" rx="1.5" stroke="currentColor" stroke-width="1.5"/></svg>',
    'mail': '<svg width="18" height="18" viewBox="0 0 18 18" fill="none"><rect x="2" y="3.5" width="14" height="11" rx="2" stroke="currentColor" stroke-width="1.5"/><path d="M2 5.5l7 5 7-5" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"/></svg>',
    'inbox': '<svg width="18" height="18" viewBox="0 0 18 18" fill="none"><path d="M16.5 10.5H13.5l-1.5 2.25h-6L4.5 10.5H1.5" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"/><path d="M4.1 3.2L1.5 10.5v4.5A1.5 1.5 0 003 16.5h12a1.5 1.5 0 001.5-1.5v-4.5l-2.6-7.3A1.5 1.5 0 0012.48 2.25H5.52a1.5 1.5 0 00-1.42 1.05z" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"/></svg>',
    'book': '<svg width="18" height="18" viewBox="0 0 18 18" fill="none"><path d="M3 3.5h4a2 2 0 012 2v9a1.5 1.5 0 00-1.5-1.5H3v-9zM15 3.5h-4a2 2 0 00-2 2v9a1.5 1.5 0 011.5-1.5H15v-9z" stroke="currentColor" stroke-width="1.5" stroke-linejoin="round"/></svg>',
    'key': '<svg width="18" height="18" viewBox="0 0 18 18" fill="none"><circle cx="6.4" cy="11.6" r="3.4" stroke="currentColor" stroke-width="1.5"/><path d="M8.8 9.2L15.5 2.5" stroke="currentColor" stroke-width="1.5" stroke-linecap="round"/><path d="M11.7 6.3l1.5 1.5M13.6 4.4l1.5 1.5" stroke="currentColor" stroke-width="1.5" stroke-linecap="round"/></svg>',
    'file-text': '<svg width="18" height="18" viewBox="0 0 18 18" fill="none"><path d="M10.5 1.5H4.5a1.5 1.5 0 00-1.5 1.5v12a1.5 1.5 0 001.5 1.5h9a1.5 1.5 0 001.5-1.5V6l-4.5-4.5z" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"/><path d="M10.5 1.5V6H15M12 9.75H6M12 12.75H6M7.5 6.75H6" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"/></svg>',
    'server': '<svg width="18" height="18" viewBox="0 0 18 18" fill="none"><rect x="2" y="2" width="14" height="5" rx="1.5" stroke="currentColor" stroke-width="1.5"/><rect x="2" y="11" width="14" height="5" rx="1.5" stroke="currentColor" stroke-width="1.5"/><circle cx="5" cy="4.5" r="0.75" fill="currentColor"/><circle cx="5" cy="13.5" r="0.75" fill="currentColor"/></svg>',
    'globe': '<svg width="18" height="18" viewBox="0 0 18 18" fill="none"><circle cx="9" cy="9" r="7" stroke="currentColor" stroke-width="1.5"/><path d="M2 9h14M9 2a11.05 11.05 0 013 7 11.05 11.05 0 01-3 7 11.05 11.05 0 01-3-7 11.05 11.05 0 013-7z" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"/></svg>',
    'link': '<svg width="18" height="18" viewBox="0 0 18 18" fill="none"><path d="M7.5 10.5a3.75 3.75 0 005.3.45l2.25-2.25a3.75 3.75 0 00-5.3-5.3l-1.29 1.28" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"/><path d="M10.5 7.5a3.75 3.75 0 00-5.3-.45L2.96 9.3a3.75 3.75 0 005.3 5.3l1.28-1.28" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"/></svg>',
    'alert-triangle': '<svg width="18" height="18" viewBox="0 0 18 18" fill="none"><path d="M7.86 2.87L1.21 14.25a1.31 1.31 0 001.14 1.97h13.3a1.31 1.31 0 001.14-1.97L10.14 2.87a1.31 1.31 0 00-2.28 0z" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"/><path d="M9 6.75v3M9 12.75h.007" stroke="currentColor" stroke-width="1.5" stroke-linecap="round"/></svg>',
    'x-circle': '<svg width="18" height="18" viewBox="0 0 18 18" fill="none"><circle cx="9" cy="9" r="7" stroke="currentColor" stroke-width="1.5"/><path d="M11.25 6.75l-4.5 4.5M6.75 6.75l4.5 4.5" stroke="currentColor" stroke-width="1.5" stroke-linecap="round"/></svg>',
    'info': '<svg width="18" height="18" viewBox="0 0 18 18" fill="none"><circle cx="9" cy="9" r="7" stroke="currentColor" stroke-width="1.5"/><path d="M9 12v-3M9 6h.007" stroke="currentColor" stroke-width="1.5" stroke-linecap="round"/></svg>',
    'x': '<svg width="18" height="18" viewBox="0 0 18 18" fill="none"><path d="M13.5 4.5l-9 9M4.5 4.5l9 9" stroke="currentColor" stroke-width="1.5" stroke-linecap="round"/></svg>',
    'menu': '<svg width="18" height="18" viewBox="0 0 18 18" fill="none"><path d="M2.25 4.5h13.5M2.25 9h13.5M2.25 13.5h13.5" stroke="currentColor" stroke-width="1.5" stroke-linecap="round"/></svg>',
    'chevron-down': '<svg width="18" height="18" viewBox="0 0 18 18" fill="none"><path d="M4.5 6.75L9 11.25l4.5-4.5" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"/></svg>',
    'panel-open': '<svg width="18" height="18" viewBox="0 0 18 18" fill="none"><rect x="2.25" y="2.25" width="13.5" height="13.5" rx="1.5" stroke="currentColor" stroke-width="1.5"/><path d="M6.75 2.25v13.5" stroke="currentColor" stroke-width="1.5"/><path d="M10.5 6.75L12.75 9l-2.25 2.25" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"/></svg>',
    'panel-shut': '<svg width="18" height="18" viewBox="0 0 18 18" fill="none"><rect x="2.25" y="2.25" width="13.5" height="13.5" rx="1.5" stroke="currentColor" stroke-width="1.5"/><path d="M6.75 2.25v13.5" stroke="currentColor" stroke-width="1.5"/><path d="M12 11.25L9.75 9 12 6.75" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"/></svg>',
    'users': '<svg width="18" height="18" viewBox="0 0 18 18" fill="none"><path d="M12.75 15.75v-1.5a3 3 0 00-3-3h-6a3 3 0 00-3 3v1.5" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"/><circle cx="6.75" cy="5.25" r="3" stroke="currentColor" stroke-width="1.5"/><path d="M17.25 15.75v-1.5a3 3 0 00-2.25-2.9M12 2.33a3 3 0 010 5.84" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"/></svg>',
    'type': '<svg width="18" height="18" viewBox="0 0 18 18" fill="none"><path d="M3 3h12M9 3v12M5.25 15h7.5" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"/></svg>',
    'list': '<svg width="18" height="18" viewBox="0 0 18 18" fill="none"><path d="M6.75 4.5h9M6.75 9h9M6.75 13.5h9M2.25 4.5h.007M2.25 9h.007M2.25 13.5h.007" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"/></svg>',
    'send': '<svg width="18" height="18" viewBox="0 0 18 18" fill="none"><path d="M16.5 1.5L8.25 9.75M16.5 1.5l-5.25 15-3-6.75L1.5 6.75l15-5.25z" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"/></svg>',
    'briefcase': '<svg width="18" height="18" viewBox="0 0 18 18" fill="none"><rect x="2" y="6" width="14" height="10" rx="1.5" stroke="currentColor" stroke-width="1.5"/><path d="M12 6V4.5A1.5 1.5 0 0010.5 3h-3A1.5 1.5 0 006 4.5V6" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"/></svg>',
    'settings': '<svg width="18" height="18" viewBox="0 0 18 18" fill="none"><circle cx="9" cy="9" r="2.25" stroke="currentColor" stroke-width="1.5"/><path d="M14.7 11.1a1.2 1.2 0 00.24 1.32l.04.04a1.46 1.46 0 11-2.06 2.06l-.04-.04a1.2 1.2 0 00-1.32-.24 1.2 1.2 0 00-.73 1.1v.12a1.46 1.46 0 01-2.91 0v-.06a1.2 1.2 0 00-.79-1.1 1.2 1.2 0 00-1.32.24l-.04.04a1.46 1.46 0 11-2.06-2.06l.04-.04a1.2 1.2 0 00.24-1.32 1.2 1.2 0 00-1.1-.73h-.12a1.46 1.46 0 010-2.91h.06a1.2 1.2 0 001.1-.79 1.2 1.2 0 00-.24-1.32l-.04-.04a1.46 1.46 0 112.06-2.06l.04.04a1.2 1.2 0 001.32.24h.06a1.2 1.2 0 00.73-1.1v-.12a1.46 1.46 0 012.91 0v.06a1.2 1.2 0 00.73 1.1 1.2 1.2 0 001.32-.24l.04-.04a1.46 1.46 0 112.06 2.06l-.04.04a1.2 1.2 0 00-.24 1.32v.06a1.2 1.2 0 001.1.73h.12a1.46 1.46 0 010 2.91h-.06a1.2 1.2 0 00-1.1.73z" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"/></svg>',
    'layers': '<svg width="18" height="18" viewBox="0 0 18 18" fill="none"><path d="M9 1.5L1.5 6 9 10.5 16.5 6 9 1.5z" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"/><path d="M1.5 12L9 16.5 16.5 12" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"/><path d="M1.5 9L9 13.5 16.5 9" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"/></svg>',
    'mail-x': '<svg width="18" height="18" viewBox="0 0 18 18" fill="none"><path d="M16 8.5V4a2 2 0 00-2-2H4a2 2 0 00-2 2v8a2 2 0 002 2h6" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"/><path d="M2 4.5l7 5 7-5" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"/><path d="M12.5 12.5l3.5 3.5M16 12.5l-3.5 3.5" stroke="currentColor" stroke-width="1.5" stroke-linecap="round"/></svg>',
    'shield': '<svg width="18" height="18" viewBox="0 0 18 18" fill="none"><path d="M9 1.5l6 2.25v4.5c0 3.5-2.4 6.8-6 8.25-3.6-1.45-6-4.75-6-8.25v-4.5L9 1.5z" stroke="currentColor" stroke-width="1.5" stroke-linejoin="round"/><path d="M6.75 9l1.5 1.5 3-3" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"/></svg>',
    'user-check': '<svg width="18" height="18" viewBox="0 0 18 18" fill="none"><path d="M11.25 15.75v-1.5a3 3 0 00-3-3h-4.5a3 3 0 00-3 3v1.5" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"/><circle cx="6" cy="5.25" r="3" stroke="currentColor" stroke-width="1.5"/><path d="M12.75 9l1.5 1.5 3-3" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"/></svg>',
    'beaker': '<svg width="18" height="18" viewBox="0 0 18 18" fill="none"><path d="M6 2.25h6" stroke="currentColor" stroke-width="1.5" stroke-linecap="round"/><path d="M7 2.25v4.6L3.4 14.6h11.2L11 6.85V2.25" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"/><path d="M5.1 11h7.8" stroke="currentColor" stroke-width="1.5" stroke-linecap="round"/></svg>',
    'bell-off': '<svg width="18" height="18" viewBox="0 0 18 18" fill="none"><path d="M13.5 6a4.5 4.5 0 10-9 0c0 5.25-2.25 6.75-2.25 6.75h13.5S13.5 11.25 13.5 6z" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"/><path d="M10.3 15.75a1.5 1.5 0 01-2.6 0" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"/><path d="M2.6 2.6l12.8 12.8" stroke="currentColor" stroke-width="1.5" stroke-linecap="round"/></svg>',
    'plug': '<svg width="18" height="18" viewBox="0 0 18 18" fill="none"><path d="M13.5 6v3.75a3 3 0 01-3 3h-3a3 3 0 01-3-3V6z" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"/><path d="M6.75 6V1.75M11.25 6V1.75M9 12.75V16.5" stroke="currentColor" stroke-width="1.5" stroke-linecap="round"/></svg>',
    'history': '<svg width="18" height="18" viewBox="0 0 18 18" fill="none"><path d="M2.25 2.25v3.75h3.75" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"/><path d="M2.29 9.75A6.75 6.75 0 104.5 3.98L2.25 6" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"/><path d="M9 5.25v3.9l3.15 1.85" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"/></svg>',
    'radio-tower': '<svg width="18" height="18" viewBox="0 0 18 18" fill="none"><path d="M5.5 1.9a5 5 0 000 4.2" stroke="currentColor" stroke-width="1.5" stroke-linecap="round"/><path d="M12.5 1.9a5 5 0 010 4.2" stroke="currentColor" stroke-width="1.5" stroke-linecap="round"/><circle cx="9" cy="4" r="1.25" stroke="currentColor" stroke-width="1.5"/><path d="M9 6.5L5.5 16M9 6.5L12.5 16M6.7 12.2h4.6" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"/></svg>',
    'lock': '<svg width="18" height="18" viewBox="0 0 18 18" fill="none"><rect x="3.5" y="7.5" width="11" height="8" rx="1.5" stroke="currentColor" stroke-width="1.5"/><path d="M6 7.5V5.25a3 3 0 016 0V7.5" stroke="currentColor" stroke-width="1.5" stroke-linecap="round"/><path d="M9 10.75v1.75" stroke="currentColor" stroke-width="1.5" stroke-linecap="round"/></svg>',
    'package': '<svg width="18" height="18" viewBox="0 0 18 18" fill="none"><path d="M15.75 6a1.5 1.5 0 00-.75-1.3l-5.25-3a1.5 1.5 0 00-1.5 0l-5.25 3A1.5 1.5 0 002.25 6v6a1.5 1.5 0 00.75 1.3l5.25 3a1.5 1.5 0 001.5 0l5.25-3A1.5 1.5 0 0015.75 12z" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"/><path d="M2.48 5.25L9 9l6.52-3.75M9 16.5V9" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"/></svg>',
    'id-card': '<svg width="18" height="18" viewBox="0 0 18 18" fill="none"><rect x="2" y="4" width="14" height="10" rx="1.5" stroke="currentColor" stroke-width="1.5"/><circle cx="6.5" cy="8" r="1.6" stroke="currentColor" stroke-width="1.5"/><path d="M4.4 11.9a2.4 2.4 0 014.2 0" stroke="currentColor" stroke-width="1.5" stroke-linecap="round"/><path d="M11 7.75h3M11 10.25h3" stroke="currentColor" stroke-width="1.5" stroke-linecap="round"/></svg>',
    'languages': '<svg width="18" height="18" viewBox="0 0 18 18" fill="none"><path d="M1.5 3.75h9M5.6 1.5h0.8M3.75 6l4.5 4.5M3 10.5l4.5-4.5 1.5-2.25" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"/><path d="M16.5 16.5l-3.75-7.5L9 16.5M10.5 13.5h4.5" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"/></svg>',
}

/**
 * getIcon returns the markup for a glyph, or an empty string when the
 * name is unknown.
 *
 * Empty rather than a fallback glyph: a wrong-but-present icon reads
 * as a deliberate choice, where a 17px gap is visibly missing. Neither
 * is good, which is why TestEveryNavIconExists checks the names in the
 * markup against this map instead of relying on either.
 */
export function getIcon(name: string): string {
  return ICONS[name] ?? ''
}
