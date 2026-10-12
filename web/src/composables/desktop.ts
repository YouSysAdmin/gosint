// The desktop app's window has no downloads and no tabs: a file is
// saved through a native dialog and a web address opens in the default
// browser. Both are functions the window binds before the page loads.
import { useNotificationStore } from '@/stores/notification'

declare global {
  interface Window {
    gosintDesktop?: boolean
    gosintSave?: (name: string, data: string) => Promise<boolean>
    gosintOpenExternal?: (url: string) => Promise<void>
  }
}

export function isDesktop(): boolean {
  return window.gosintDesktop === true
}

function base64Of(blob: Blob): Promise<string> {
  return new Promise((resolve, reject) => {
    const r = new FileReader()
    r.onload = () => resolve(String(r.result).replace(/^data:[^,]*,/, ''))
    r.onerror = () => reject(r.error)
    r.readAsDataURL(blob)
  })
}

// saveNative asks the window where to keep the file. A failure is a
// toast, a cancel says nothing.
export async function saveNative(blob: Blob, name: string) {
  try {
    if (!window.gosintSave) throw new Error('This window cannot save files')
    const saved = await window.gosintSave(name, await base64Of(blob))
    if (saved) useNotificationStore().success(`Saved ${name}`)
  } catch (err) {
    useNotificationStore().error(err instanceof Error ? err.message : String(err))
  }
}

export function openExternal(url: string) {
  window.gosintOpenExternal?.(url).catch((err: unknown) => {
    useNotificationStore().error(err instanceof Error ? err.message : String(err))
  })
}
