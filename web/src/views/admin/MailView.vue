<script setup lang="ts">
// The relay the server sends its own mail through: password reset and
// invitation links. Without a host nothing is sent and the links are
// handed over by an administrator. THE PASSWORD IS NEVER READ BACK.
import { onMounted, ref } from 'vue'
import { getMail, saveMail, testMail } from '@/api/admin'
import { apiErrorMessage } from '@/api/client'
import type { MailSettings, MailSettingsInput } from '@/api/types'
import EmptyState from '@/components/EmptyState.vue'
import FormField from '@/components/FormField.vue'
import LoadingBlock from '@/components/LoadingBlock.vue'
import PageHeader from '@/components/PageHeader.vue'
import AdminNav from '@/views/admin/AdminNav.vue'
import { useNotificationStore } from '@/stores/notification'

const toast = useNotificationStore()
const saved = ref<MailSettings | null>(null)
const form = ref({
  host: '',
  port: 587,
  username: '',
  password: '',
  from: '',
  starttls: true,
})
const busy = ref('')
// The first load shows the spinner, or a retry when it failed, and the
// form only once the settings are in it.
const loaded = ref(false)
const loadError = ref('')

function fill(m: MailSettings) {
  saved.value = m
  form.value = {
    host: m.host,
    port: m.port || 587,
    username: m.username,
    password: '',
    from: m.from,
    starttls: m.starttls,
  }
}

async function load() {
  try {
    fill(await getMail())
    loaded.value = true
    loadError.value = ''
  } catch (err) {
    toast.error(apiErrorMessage(err))
    if (!loaded.value) loadError.value = apiErrorMessage(err)
  }
}

async function save() {
  const f = form.value
  const body: MailSettingsInput = {
    host: f.host,
    port: Number(f.port),
    username: f.username,
    from: f.from,
    starttls: f.starttls,
  }
  // Only a typed password travels, so a save keeps the stored one.
  if (f.password) body.password = f.password
  busy.value = 'save'
  try {
    fill(await saveMail(body))
    toast.success(f.host ? 'Mail relay saved' : 'Mail turned off')
  } catch (err) {
    toast.error(apiErrorMessage(err))
  } finally {
    busy.value = ''
  }
}

async function test() {
  busy.value = 'test'
  try {
    toast.success(`Test message sent to ${await testMail()}`)
  } catch (err) {
    toast.error(apiErrorMessage(err))
  } finally {
    busy.value = ''
  }
}

// Implicit TLS lives on 465, STARTTLS on 587.
function tlsChanged() {
  const f = form.value
  if (!f.starttls && f.port === 587) f.port = 465
  else if (f.starttls && f.port === 465) f.port = 587
}

onMounted(load)
</script>

<template>
  <PageHeader title="Mail">
    <button
      class="btn btn-secondary"
      :disabled="!saved?.enabled || busy !== ''"
      title="Send a message to your own address through the saved relay"
      @click="test"
    >
      {{ busy === 'test' ? 'Sending...' : 'Send a test message' }}
    </button>
  </PageHeader>
  <AdminNav />

  <LoadingBlock v-if="!loaded && !loadError" />
  <div v-else-if="!loaded" class="card">
    <EmptyState title="The mail settings could not be loaded">
      <p>{{ loadError }}</p>
      <button class="btn btn-secondary" @click="load">Retry</button>
    </EmptyState>
  </div>
  <form v-else class="card" @submit.prevent="save">
    <div class="card-body">
      <p class="text-sm text-muted mb-3">
        The relay for password reset and invitation links, built from the public URL. Left empty,
        nothing is sent and an administrator hands the links over instead.
        <span v-if="saved && !saved.enabled" class="badge badge-neutral ml-2">off</span>
        <span v-else-if="saved" class="badge badge-success ml-2">on</span>
      </p>
      <div class="two">
        <FormField label="Host" hint="Empty turns mail off">
          <input
            v-model="form.host"
            class="form-input"
            maxlength="255"
            placeholder="smtp.example.com"
          />
        </FormField>
        <FormField label="Port">
          <input v-model.number="form.port" class="form-input" type="number" min="1" max="65535" />
        </FormField>
      </div>
      <FormField label="Sender" hint="An address, a display name allowed">
        <input
          v-model="form.from"
          class="form-input"
          maxlength="320"
          placeholder="Gosint <gosint@example.com>"
        />
      </FormField>
      <div class="two">
        <FormField label="Username">
          <input v-model="form.username" class="form-input" maxlength="255" autocomplete="off" />
        </FormField>
        <FormField
          label="Password"
          :hint="saved?.has_password ? 'Stored, leave empty to keep it' : ''"
        >
          <input
            v-model="form.password"
            class="form-input"
            type="password"
            autocomplete="new-password"
          />
        </FormField>
      </div>
      <label class="checkbox-label mb-3">
        <input v-model="form.starttls" type="checkbox" @change="tlsChanged" />
        STARTTLS (off connects with implicit TLS, port 465)
      </label>
      <div class="flex justify-end">
        <button class="btn btn-primary" :disabled="busy !== ''">Save</button>
      </div>
    </div>
  </form>
</template>

<style scoped>
.two {
  display: grid;
  grid-template-columns: 2fr 1fr;
  gap: 12px;
}

@media (max-width: 600px) {
  .two {
    grid-template-columns: 1fr;
  }
}
</style>
