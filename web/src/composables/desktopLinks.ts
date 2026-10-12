// The clicks a desktop window cannot follow: a link to another site
// opens in the browser, a download of this server's is fetched and
// saved through the native dialog.
import { fileNameOf } from './download'
import { isDesktop, openExternal, saveNative } from './desktop'
import { useNotificationStore } from '@/stores/notification'

async function saveFromServer(url: URL) {
  try {
    const res = await fetch(url, { credentials: 'same-origin' })
    if (!res.ok) throw new Error(`The download failed (${res.status})`)
    const fallback = decodeURIComponent(url.pathname.split('/').pop() || 'download')
    const name = fileNameOf(res.headers.get('Content-Disposition') ?? undefined, fallback)
    await saveNative(await res.blob(), name)
  } catch (err) {
    useNotificationStore().error(err instanceof Error ? err.message : String(err))
  }
}

export function installDesktopLinks() {
  if (!isDesktop()) return
  document.addEventListener(
    'click',
    (e) => {
      if (e.defaultPrevented || e.button !== 0) return
      const a = (e.target as Element | null)?.closest?.('a[href]') as HTMLAnchorElement | null
      if (!a) return
      const url = new URL(a.href, location.href)
      if (url.protocol !== 'http:' && url.protocol !== 'https:') return
      if (url.origin !== location.origin) {
        e.preventDefault()
        openExternal(url.href)
        return
      }
      if (a.hasAttribute('download') || url.searchParams.get('download') === '1') {
        e.preventDefault()
        void saveFromServer(url)
      }
    },
    true,
  )
}
