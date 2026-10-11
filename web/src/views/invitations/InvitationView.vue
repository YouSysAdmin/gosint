<script setup lang="ts">
// The page an invitation link opens. Signed in as the invited address,
// one click joins. Not signed in, the person signs in or registers with
// the token kept, and comes back here. Signed in as somebody else, the
// page says so.
import { onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { apiErrorMessage } from '@/api/client'
import { acceptInvitation, lookupInvitation } from '@/api/invitations'
import type { InvitationLookup } from '@/api/types'
import AuthPage from '@/components/AuthPage.vue'
import { signOut } from '@/stores/auth'

const route = useRoute()
const router = useRouter()
const token = typeof route.params.token === 'string' ? route.params.token : ''
const invitation = ref<InvitationLookup | null>(null)
const error = ref('')
const busy = ref(false)

onMounted(async () => {
  try {
    invitation.value = await lookupInvitation(token)
  } catch (err) {
    error.value = apiErrorMessage(err)
  }
})

const mine = () =>
  invitation.value?.signed_in_as &&
  invitation.value.signed_in_as.toLowerCase() === invitation.value.email.toLowerCase()

async function accept() {
  busy.value = true
  try {
    const res = await acceptInvitation(token)
    await router.replace(`/investigations/${res.investigation_id}`)
  } catch (err) {
    error.value = apiErrorMessage(err)
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <AuthPage title="You are invited">
    <p v-if="error" class="form-error">{{ error }}</p>
    <template v-else-if="invitation">
      <p class="text-sm mb-3">
        <strong>{{ invitation.email }}</strong> is invited to the investigation
        <strong>{{ invitation.investigation }}</strong> as {{ invitation.role }}.
      </p>
      <template v-if="mine()">
        <button class="btn btn-primary btn-block" :disabled="busy" @click="accept">
          Join the investigation
        </button>
      </template>
      <template v-else-if="invitation.signed_in_as">
        <p class="text-sm text-muted mb-3">
          You are signed in as <strong>{{ invitation.signed_in_as }}</strong
          >. Sign out and come back as {{ invitation.email }}.
        </p>
        <button class="btn btn-secondary btn-block" @click="signOut({ invite: token })">
          Sign out
        </button>
      </template>
      <template v-else-if="invitation.has_account">
        <router-link
          class="btn btn-primary btn-block"
          :to="{ name: 'login', query: { invite: token } }"
        >
          Sign in to join
        </router-link>
      </template>
      <template v-else>
        <p class="text-sm text-muted mb-3">
          There is no account for this address yet. Create one and you join on the way.
        </p>
        <router-link
          class="btn btn-primary btn-block"
          :to="{ name: 'register', query: { invite: token } }"
        >
          Create an account
        </router-link>
      </template>
    </template>
    <p v-else class="text-sm text-muted">Loading...</p>
  </AuthPage>
</template>
