import { storageRemovePrefix } from '@/composables/storage'
import { defineStore } from 'pinia'
import { ref } from 'vue'
import * as authApi from '@/api/auth'
import type { Profile } from '@/api/types'

// The signed-in account. loaded tells "not asked yet" from "nobody".
export const useAuthStore = defineStore('auth', () => {
  const user = ref<Profile | null>(null)
  const loaded = ref(false)

  async function load(): Promise<Profile | null> {
    if (loaded.value) return user.value
    try {
      user.value = await authApi.me()
    } catch {
      user.value = null
    }
    loaded.value = true
    return user.value
  }

  function set(p: Profile) {
    user.value = p
    loaded.value = true
  }

  // refresh re-reads the profile, after a change to how the account
  // signs in.
  async function refresh(): Promise<Profile | null> {
    loaded.value = false
    return load()
  }

  return { user, loaded, load, set, refresh }
})

// Signing out crosses a document boundary on purpose, so no store keeps
// the previous account's state.
export async function signOut(query: Record<string, string> = {}) {
  const search = new URLSearchParams(query).toString()
  // Filters name what was searched for in a sketch, the next person on
  // this browser has no business reading them.
  storageRemovePrefix('gosint.filters.')
  try {
    await authApi.logout()
  } finally {
    location.assign(`${import.meta.env.BASE_URL}login${search ? '?' + search : ''}`)
  }
}
