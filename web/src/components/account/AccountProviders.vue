<script setup lang="ts">
// The identity providers this account signs in with. Linking one is a
// round trip through the provider, started here and landing back on
// the profile with ?linked= or ?err=.
import { computed, ref } from 'vue'
import { identities as listIdentities, info, linkProvider, unlinkIdentity } from '@/api/auth'
import { apiErrorMessage } from '@/api/client'
import type { LinkedIdentity, LoginProvider } from '@/api/types'
import { formatDate } from '@/composables/formatDate'
import { useConfirm } from '@/composables/useConfirm'
import { useNotificationStore } from '@/stores/notification'

const toast = useNotificationStore()
const { confirm } = useConfirm()
const linked = ref<LinkedIdentity[]>([])
const providers = ref<LoginProvider[]>([])
const busy = ref('')

// A provider may be linked more than once (two people at the IdP), so
// every one stays on offer, those already linked last.
const offered = computed(() => {
  const have = new Set(linked.value.map((i) => i.provider_slug))
  return [...providers.value].sort(
    (a, b) => Number(have.has(a.slug)) - Number(have.has(b.slug)) || a.name.localeCompare(b.name),
  )
})

async function load() {
  try {
    const [ids, i] = await Promise.all([listIdentities(), info()])
    linked.value = ids
    providers.value = i.providers
  } catch (err) {
    toast.error(apiErrorMessage(err))
  }
}

async function link(p: LoginProvider) {
  busy.value = p.slug
  try {
    window.location.assign(await linkProvider(p.slug))
  } catch (err) {
    toast.error(apiErrorMessage(err))
    busy.value = ''
  }
}

async function unlink(i: LinkedIdentity) {
  const ok = await confirm({
    title: 'Unlink the provider',
    message: `Stop signing in with ${i.provider_name || 'this provider'} as ${i.email || 'this identity'}?`,
    confirmText: 'Unlink',
    variant: 'danger',
  })
  if (!ok) return
  busy.value = i.id
  try {
    await unlinkIdentity(i.id)
    toast.success('Provider unlinked')
    await load()
  } catch (err) {
    toast.error(apiErrorMessage(err))
  } finally {
    busy.value = ''
  }
}

void load()
</script>

<template>
  <div v-if="linked.length || providers.length" class="card">
    <div class="card-header">
      <h2>Sign-in providers</h2>
    </div>
    <div class="card-body">
      <table v-if="linked.length" class="mb-3">
        <thead>
          <tr>
            <th>Provider</th>
            <th>As</th>
            <th>Last used</th>
            <th></th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="i in linked" :key="i.id">
            <td>
              <strong>{{ i.provider_name || i.provider_slug || 'A removed provider' }}</strong>
            </td>
            <td>{{ i.email || '-' }}</td>
            <td>{{ formatDate(i.last_login_at) }}</td>
            <td class="text-right">
              <button class="btn btn-danger btn-sm" :disabled="busy === i.id" @click="unlink(i)">
                Unlink
              </button>
            </td>
          </tr>
        </tbody>
      </table>
      <p v-else class="text-sm text-muted mb-3">No provider is linked to this account.</p>
      <div v-if="offered.length" class="flex gap-2 flex-wrap">
        <button
          v-for="p in offered"
          :key="p.slug"
          type="button"
          class="btn btn-secondary btn-sm"
          :disabled="busy === p.slug"
          @click="link(p)"
        >
          Link {{ p.name }}
        </button>
      </div>
    </div>
  </div>
</template>
