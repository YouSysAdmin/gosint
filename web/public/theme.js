// Sets the theme before the first paint, read the same way the theme
// store reads it, so a dark console does not open light and fade. A
// file of its own since the shell's policy allows no inline script.
;(function () {
  try {
    var mode = localStorage.getItem('gosint_theme')
    var dark =
      mode === 'dark' || (mode !== 'light' && matchMedia('(prefers-color-scheme: dark)').matches)
    document.documentElement.setAttribute('data-theme', dark ? 'dark' : 'light')
  } catch (e) {
    // no storage, the store decides once it runs
  }
})()
