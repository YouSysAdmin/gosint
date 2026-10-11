// The name of the command modifier on this machine, for the shortcuts
// the console prints: Cmd on a Mac, Ctrl elsewhere. The handlers take
// either key, this is only what the person reads.
export const modKey = /Mac|iPhone|iPad/.test(navigator.platform) ? 'Cmd' : 'Ctrl'
