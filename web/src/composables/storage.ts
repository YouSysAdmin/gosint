// The browser's storage behind try/catch: a private window, a policy
// or a blocked site can make every call throw, and a preference that
// cannot be kept must not take the console down with it.
function area(session: boolean): Storage | null {
  try {
    return session ? window.sessionStorage : window.localStorage
  } catch {
    return null
  }
}

export function storageGet(key: string, session = false): string | null {
  try {
    return area(session)?.getItem(key) ?? null
  } catch {
    return null
  }
}

export function storageSet(key: string, value: string, session = false) {
  try {
    area(session)?.setItem(key, value)
  } catch {
    // nothing to do, the preference is just not remembered
  }
}

export function storageRemove(key: string, session = false) {
  try {
    area(session)?.removeItem(key)
  } catch {
    // same as above
  }
}

// storageRemovePrefix drops every key starting with prefix.
export function storageRemovePrefix(prefix: string, session = false) {
  try {
    const store = area(session)
    if (!store) return
    const keys: string[] = []
    for (let i = 0; i < store.length; i++) {
      const key = store.key(i)
      if (key?.startsWith(prefix)) keys.push(key)
    }
    keys.forEach((key) => store.removeItem(key))
  } catch {
    // same as above
  }
}

export function storageGetJSON<T>(key: string, fallback: T): T {
  const raw = storageGet(key)
  if (raw === null) return fallback
  try {
    return JSON.parse(raw) as T
  } catch {
    return fallback
  }
}

export function storageSetJSON(key: string, value: unknown) {
  storageSet(key, JSON.stringify(value))
}
