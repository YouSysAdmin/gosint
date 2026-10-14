import axios from 'axios'

// The session travels as an HttpOnly cookie on every same-origin
// request, so there is no token handling here.
const api = axios.create({
  baseURL: '/api',
  headers: { 'Content-Type': 'application/json' },
  withCredentials: true,
})

// A 401 outside the sign-in calls means the session ended: go to the
// login page once, keeping where the person was. A 503 "sealed" means
// the server restarted and waits for its unseal key.
let redirecting = false
api.interceptors.response.use(
  (r) => r,
  (err) => {
    const url: string = err.config?.url ?? ''
    const here = location.pathname.replace(import.meta.env.BASE_URL, '/')
    const next = encodeURIComponent(here)
    const sealed = err.response?.status === 503 && err.response?.data?.error === 'sealed'
    if (sealed && !redirecting && here !== '/unseal') {
      redirecting = true
      location.assign(`${import.meta.env.BASE_URL}unseal?next=${next}`)
    } else if (err.response?.status === 401 && !url.startsWith('/auth/') && !redirecting) {
      redirecting = true
      location.assign(`${import.meta.env.BASE_URL}login?next=${next}`)
    }
    return Promise.reject(err)
  },
)

// Errors are {"error": "msg", "fields": [...]}. Falls back to the
// transport message.
export function apiErrorMessage(err: unknown): string {
  if (axios.isAxiosError(err)) {
    const msg = (err.response?.data as { error?: string } | undefined)?.error
    if (msg) return msg
    return err.message
  }
  return String(err)
}

// isStatus tells whether an error is the server answering that code.
export function isStatus(err: unknown, status: number): boolean {
  return axios.isAxiosError(err) && err.response?.status === status
}

// readBlobError reads the server's message back out of an error answered
// to a blob request, where the body arrives as a Blob. The error is
// returned with its data replaced, to be thrown again.
export async function readBlobError(err: unknown): Promise<unknown> {
  const data = (err as { response?: { data?: unknown } }).response?.data
  if (data instanceof Blob) {
    try {
      ;(err as { response: { data: unknown } }).response.data = JSON.parse(await data.text())
    } catch {
      // Not JSON, the status alone tells the story.
    }
  }
  return err
}

export default api
