import { defineStore } from 'pinia'
import { computed, ref, watchEffect } from 'vue'
import { storageGet, storageSet } from '@/composables/storage'

/** What the operator chose, which is not the same as what is on screen. */
export type ThemeMode = 'light' | 'dark' | 'system'

const STORAGE_KEY = 'gosint_theme'
const MODES: ThemeMode[] = ['light', 'dark', 'system']

/**
 * The colour mode: the operator's choice, what it resolves to, and the
 * `data-theme` attribute the stylesheet reads.
 *
 * Three modes rather than a boolean, because "follow the system" is a
 * standing instruction rather than a value - a person who picked it
 * expects the console to move when their laptop does at sunset, which
 * a stored true/false cannot express.
 */
export const useThemeStore = defineStore('theme', () => {
  const query = window.matchMedia('(prefers-color-scheme: dark)')

  // The OS preference held in a REF rather than read from the query
  // inside the computed below. `query.matches` is a plain property and
  // Vue cannot track it, so a computed reading it directly goes stale
  // the moment the OS flips - the page would keep whatever it resolved
  // to at boot, and every binding on isDark would disagree with what
  // is actually painted.
  const systemDark = ref(query.matches)
  query.addEventListener('change', (e) => {
    systemDark.value = e.matches
  })

  const saved = storageGet(STORAGE_KEY) as ThemeMode | null
  const mode = ref<ThemeMode>(saved && MODES.includes(saved) ? saved : 'system')

  /** What is actually on screen once `system` is resolved. */
  const isDark = computed(() =>
    mode.value === 'system' ? systemDark.value : mode.value === 'dark',
  )

  // Painting only: it runs on creation, so the attribute is correct
  // before first paint, and re-runs on either the choice or the OS
  // changing. Nothing is stored here, so a person who never chose
  // keeps following the system rather than having "system" written
  // down for them.
  watchEffect(() => {
    document.documentElement.setAttribute('data-theme', isDark.value ? 'dark' : 'light')
  })

  /** Pick a mode explicitly - the three-way control in the account menu. */
  function setMode(next: ThemeMode) {
    mode.value = next
    storageSet(STORAGE_KEY, next)
  }

  /**
   * Flip to the opposite of what is showing. Always lands on an
   * explicit light or dark: toggling is a statement about what the
   * person wants NOW, so leaving them in `system` would let the choice
   * be undone by their OS an hour later.
   */
  function toggle() {
    setMode(isDark.value ? 'light' : 'dark')
  }

  return { mode, isDark, setMode, toggle }
})
