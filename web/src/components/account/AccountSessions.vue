<script setup lang="ts">
// Everywhere this account is signed in. Revoking takes effect on the
// next request that session makes, the control for a laptop left
// behind.
import { computed, ref } from 'vue'
import { revokeOtherSessions, revokeSession, sessions as listSessions } from '@/api/auth'
import { apiErrorMessage } from '@/api/client'
import type { UserSession } from '@/api/types'
import { formatDate } from '@/composables/formatDate'
import { useConfirm } from '@/composables/useConfirm'
import { signOut } from '@/stores/auth'
import { useNotificationStore } from '@/stores/notification'

const toast = useNotificationStore()
const { confirm } = useConfirm()
const sessions = ref<UserSession[]>([])
const busy = ref('')
const others = computed(() => sessions.value.filter((s) => !s.current).length)

async function load() {
  try {
    sessions.value = await listSessions()
  } catch (err) {
    toast.error(apiErrorMessage(err))
  }
}

// describe is a user agent as something a person recognises. Crude on
// purpose, it only has to answer "is this me?".
function describe(s: UserSession): string {
  const ua = s.user_agent || ''
  const browser =
    [
      ['Firefox/', 'Firefox'],
      ['Edg/', 'Edge'],
      ['Chrome/', 'Chrome'],
      ['Safari/', 'Safari'],
    ].find(([token]) => ua.includes(token))?.[1] ??
    ua.split(' ')[0] ??
    'Unknown client'
  const os =
    [
      ['Mac OS X', 'macOS'],
      ['Windows', 'Windows'],
      ['Android', 'Android'],
      ['iPhone', 'iOS'],
      ['iPad', 'iOS'],
      ['Linux', 'Linux'],
    ].find(([token]) => ua.includes(token))?.[1] ?? ''
  return os ? `${browser} on ${os}` : browser || 'Unknown client'
}

async function revoke(s: UserSession) {
  const ok = await confirm({
    title: s.current ? 'Sign out here' : 'Revoke this session',
    message: s.current
      ? 'This signs you out on this device, now.'
      : `Sign out ${describe(s)}? That device has to sign in again.`,
    confirmText: s.current ? 'Sign out' : 'Revoke',
    variant: 'danger',
  })
  if (!ok) return
  busy.value = s.id
  try {
    if (s.current) {
      await signOut()
      return
    }
    await revokeSession(s.id)
    toast.success('Session revoked')
    await load()
  } catch (err) {
    toast.error(apiErrorMessage(err))
  } finally {
    busy.value = ''
  }
}

async function revokeOthers() {
  const ok = await confirm({
    title: 'Sign out everywhere else',
    message: 'Every other signed-in device has to sign in again. This one stays.',
    confirmText: 'Sign out others',
    variant: 'danger',
  })
  if (!ok) return
  busy.value = 'others'
  try {
    const n = await revokeOtherSessions()
    toast.success(n === 1 ? 'One session signed out' : `${n} sessions signed out`)
    await load()
  } catch (err) {
    toast.error(apiErrorMessage(err))
  } finally {
    busy.value = ''
  }
}

defineExpose({ reload: load })
void load()
</script>

<template>
  <div class="card">
    <div class="card-header">
      <h2>Signed in</h2>
      <button
        v-if="others > 0"
        class="btn btn-danger btn-sm"
        :disabled="busy === 'others'"
        @click="revokeOthers"
      >
        Sign out everywhere else
      </button>
    </div>
    <div class="card-body">
      <table v-if="sessions.length">
        <thead>
          <tr>
            <th>Device</th>
            <th>Address</th>
            <th>Since</th>
            <th>Until</th>
            <th></th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="s in sessions" :key="s.id">
            <td>
              <strong>{{ describe(s) }}</strong>
              <span v-if="s.current" class="badge badge-success ml-2">This device</span>
            </td>
            <td>
              <code>{{ s.client_ip || '-' }}</code>
            </td>
            <td>{{ formatDate(s.created_at) }}</td>
            <td>{{ formatDate(s.expires_at) }}</td>
            <td class="text-right">
              <button class="btn btn-danger btn-sm" :disabled="busy === s.id" @click="revoke(s)">
                {{ s.current ? 'Sign out' : 'Revoke' }}
              </button>
            </td>
          </tr>
        </tbody>
      </table>
      <p v-else class="text-sm text-muted">No active sessions.</p>
    </div>
  </div>
</template>
